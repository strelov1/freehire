package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// realtek adapts Realtek Semiconductor's own recruitment site (recruit.realtek.com). Single-
// company and boardless. The site is a Kendo grid over a small keyless JSON API, all Traditional
// Chinese.
//
// The listing is CAPPED at 200 rows: the unfiltered dump and an unfiltered search both stop
// there while the board holds more (279 on 2026-10-25). The search's experience filter splits
// the board into two disjoint halves — xp=0 returns the postings asking no experience (Exp 0 or
// -1), xp=1 those asking at least a year — so the crawl reads both, and a half that comes back
// at the cap fails the run rather than silently dropping its overflow.
//
// The listing has no body; a posting the catalogue does not hold yet is hydrated from the
// vacancy-detail endpoint (HydratingSource), whose Requirement text carries both the duties and
// the requirements. A seen posting costs no request.
type realtek struct {
	http realtekHTTP
}

// realtekHTTP is the transport realtek needs: a form POST for the search and a GET per posting.
type realtekHTTP interface {
	FormJSONPoster
	JSONGetter
}

const (
	realtekSearchURL = "https://recruit.realtek.com/Job/GetFilterList"
	realtekDetailURL = "https://recruit.realtek.com/Job/GetVacancyDetail?JobOppId="
	realtekJobURL    = "https://recruit.realtek.com/Job/JobDetail?jobid="
	// realtekRowCap is the most rows the API returns for one query.
	realtekRowCap = 200
)

// realtekExperienceBuckets are the xp values that split the board into disjoint halves.
var realtekExperienceBuckets = []string{"0", "1"}

// realtekCategories maps the site's job type onto vocab.CategoryValues where it names one
// discipline. Titles are mostly Chinese, which our title dictionary cannot read, so this is
// what files them; realtekTechTypes marks the R&D types that span several disciplines.
var realtekCategories = map[string]string{
	"數位設計":   "hardware", // digital IC design
	"類比設計":   "hardware", // analog IC design
	"SoC驗證":  "hardware", // SoC verification
	"設計技術服務": "hardware", // design services: layout, CAD flows
	"元件工程":   "hardware", // component engineering
	"人力資源管理": "hr",
	"智權、法務":  "legal",
	"行政管理":   "administration",
}

var realtekTechTypes = map[string]bool{
	"系統設計":   true, // system design: firmware, drivers, platform software
	"設計技術研發": true, // design-technology R&D
	"資訊技術":   true, // information technology
	"研發替代役 & 預聘_應屆畢業人才": true, // R&D alternative service and graduate pre-hire
}

var realtekIDRE = regexp.MustCompile(`^[0-9]+$`)

// NewRealtek builds the Realtek recruitment-site adapter over the given HTTP client.
func NewRealtek(c realtekHTTP) Source { return realtek{http: c} }

func (realtek) Provider() string { return "realtek" }

// realtek is single-company, so its config entry carries no board.
func (realtek) boardless() {}

type realtekListResponse struct {
	Pass bool          `json:"Pass"`
	Data []realtekPost `json:"Data"`
}

// realtekPost is one listing row. Every value is a JSON string, ids and Exp included.
type realtekPost struct {
	OppID    string `json:"JobOppId"`
	Title    string `json:"JobTitle"`
	JobType  string `json:"JobType"`
	Exp      string `json:"Exp"`
	Location string `json:"Location"`
}

type realtekDetailResponse struct {
	Pass bool `json:"Pass"`
	Data *struct {
		Title       *string `json:"JobTitle"`
		Requirement *string `json:"Requirement"`
	} `json:"Data"`
}

func (s realtek) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s realtek) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	asked := 0
	for _, p := range list {
		if !seen(p.OppID) {
			asked++
		}
	}
	jobs := fetchDetails(list, defaultDetailWorkers, func(p realtekPost) (Job, bool) {
		if seen(p.OppID) {
			return Job{ExternalID: p.OppID, URL: realtekJobURL + p.OppID, Title: realtekTitle(p.Title), Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, p)
	})
	if asked > 0 && len(jobs) == len(list)-asked {
		return nil, fmt.Errorf("realtek: none of %d new postings could be read", asked)
	}
	return jobs, nil
}

// list reads both experience halves of the board and fails when either reaches the row cap.
func (s realtek) list(ctx context.Context) ([]realtekPost, error) {
	var out []realtekPost
	ids := map[string]bool{}
	for _, xp := range realtekExperienceBuckets {
		var r realtekListResponse
		if err := s.http.PostFormJSON(ctx, realtekSearchURL, url.Values{"xp": {xp}}, &r); err != nil {
			return nil, fmt.Errorf("realtek: search xp=%s: %w", xp, err)
		}
		if !r.Pass {
			return nil, fmt.Errorf("realtek: search xp=%s answered Pass=false", xp)
		}
		if len(r.Data) >= realtekRowCap {
			return nil, fmt.Errorf("realtek: search xp=%s returned %d rows, the API's cap; the board needs a finer split", xp, len(r.Data))
		}
		for _, p := range r.Data {
			if !realtekIDRE.MatchString(p.OppID) || ids[p.OppID] {
				return nil, fmt.Errorf("realtek: posting id %q is malformed or listed twice", p.OppID)
			}
			ids[p.OppID] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		// The board always carries vacancies; none is a changed API, and must not read as every
		// posting closed.
		return nil, fmt.Errorf("realtek: the search lists no postings")
	}
	return out, nil
}

// detail hydrates a new posting. The endpoint answers an unknown id with 200 and a null title,
// the site's own not-found signal, and that posting is dropped as gone; a failed request defers
// it to the next crawl.
func (s realtek) detail(ctx context.Context, e CompanyEntry, p realtekPost) (Job, bool) {
	var r realtekDetailResponse
	if err := s.http.GetJSON(ctx, realtekDetailURL+p.OppID, &r); err != nil {
		log.Printf("realtek: detail %s failed; deferring: %v", p.OppID, err)
		return Job{}, false
	}
	if !r.Pass || r.Data == nil || r.Data.Title == nil || r.Data.Requirement == nil {
		return Job{}, false
	}
	title := realtekTitle(firstNonEmpty(*r.Data.Title, p.Title))
	body := plainTextToHTML(*r.Data.Requirement)
	if title == "" || body == "" {
		return Job{}, false
	}
	jobType := strings.TrimSpace(p.JobType)
	location, countries := twLocation(p.Location)
	job := Job{
		ExternalID:  p.OppID,
		URL:         realtekJobURL + p.OppID,
		Title:       title,
		Company:     e.Company,
		Description: sanitizeHTML(body),
		Location:    location,
		Countries:   countries,
		Category:    realtekCategories[jobType],
		IsTechHint:  realtekTechTypes[jobType],
	}
	// Exp is the minimum years asked; 0 and -1 both mean none is.
	if n, err := strconv.Atoi(strings.TrimSpace(p.Exp)); err == nil && n > 0 {
		job.ExperienceYearsMin = &n
	}
	return job, true
}

func realtekTitle(s string) string { return strings.Join(strings.Fields(s), " ") }
