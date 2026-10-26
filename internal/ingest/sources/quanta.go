package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// quanta adapts Quanta Computer's own recruitment site (hr.quantatw.com), the Taiwan catalogue.
// Single-company and boardless. Quanta's US manufacturing arm hires through its UKG tenant,
// which the ukgready adapter already crawls; this site is the parent's.
//
// The whole board is ONE anonymous GET that returns every open vacancy with its duties and
// requirements inline — the site's own page fetches it once and searches it in the browser — so
// there is no pagination and no detail request. The query string is ignored by the server.
//
// Everything is Traditional Chinese, which our title dictionary cannot read, so the site's own
// job category is what states the discipline (quantaCategories).
//
// Not fullBoardListing: the dump states no total to check it against, and a live run on
// 2026-10-25 once received 8 of its 244 vacancies under a 200 with well-formed JSON. Granting the
// board-scoped close on that evidence would have closed the other 236.
type quanta struct {
	http JSONGetter
}

const (
	quantaListURL = "https://hr.quantatw.com/QuantaRecruit/Home/QueryJob"
	// quantaJobURL is the site's own share link: the job page opens with ?serial= expanded.
	quantaJobURL = "https://hr.quantatw.com/QuantaRecruit/Home/Job?serial="
)

// quantaCategories maps the site's job category (caponm) onto vocab.CategoryValues where it
// names one discipline. The rest — sales-and-marketing (FAE beside account sales), validation
// (EMC and safety labs beside system test), the fresh-graduate, overseas, "other" and the
// alternative-military-service R&D zones (mechanical beside software) — span several and are
// left to the dictionaries.
var quantaCategories = map[string]string{
	"硬體研發": "hardware",             // hardware R&D
	"電源研發": "hardware",             // power-supply R&D
	"軟體研發": "software_engineering", // software R&D
	"機構研發": "engineering_design",   // mechanical design
	"專案管理": "project_management",
	"生產製造": "manufacturing",
	"行政管理": "administration",
}

var quantaSerialRE = regexp.MustCompile(`^[0-9]+$`)

// NewQuanta builds the Quanta Computer adapter over the given HTTP client.
func NewQuanta(c JSONGetter) Source { return quanta{http: c} }

func (quanta) Provider() string { return "quanta" }

// quanta is single-company, so its config entry carries no board.
func (quanta) boardless() {}

type quantaResponse struct {
	Jobs *[]quantaJob `json:"jobResult"`
}

// quantaJob is one vacancy. Every value is a JSON string, ids included.
type quantaJob struct {
	Serial       string  `json:"serial"`
	Title        string  `json:"jobdnm"`
	Category     string  `json:"caponm"`
	Location     string  `json:"locatm"`
	Duties       string  `json:"descnm"`
	Requirements *string `json:"requnm"` // null on a few disability-quota rows
	Cancelled    string  `json:"cancel"`
	External     string  `json:"posext"`
}

func (s quanta) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var r quantaResponse
	if err := s.http.GetJSON(ctx, quantaListURL, &r); err != nil {
		return nil, fmt.Errorf("quanta: listing: %w", err)
	}
	if r.Jobs == nil || len(*r.Jobs) == 0 {
		// The board always carries vacancies; none is a changed response, and must not read
		// as every posting closed.
		return nil, fmt.Errorf("quanta: the listing carries no vacancies")
	}
	seen := map[string]bool{}
	var out []Job
	for _, j := range *r.Jobs {
		if !quantaSerialRE.MatchString(j.Serial) || seen[j.Serial] {
			return nil, fmt.Errorf("quanta: vacancy serial %q is malformed or listed twice", j.Serial)
		}
		seen[j.Serial] = true
		// Only cancel=N, posext=Y rows have ever been observed; anything else is not on the
		// public board.
		if j.Cancelled == "Y" || j.External == "N" {
			continue
		}
		if job, ok := s.job(e, j); ok {
			out = append(out, job)
		}
	}
	return out, nil
}

func (quanta) job(e CompanyEntry, j quantaJob) (Job, bool) {
	title := strings.Join(strings.Fields(j.Title), " ")
	body := plainTextToHTML(j.Duties)
	if j.Requirements != nil {
		body += plainTextToHTML(*j.Requirements)
	}
	if title == "" || body == "" {
		return Job{}, false
	}
	location, countries := twLocation(j.Location)
	return Job{
		ExternalID:  j.Serial,
		URL:         quantaJobURL + j.Serial,
		Title:       title,
		Company:     e.Company,
		Description: sanitizeHTML(body),
		Location:    location,
		Countries:   countries,
		Category:    quantaCategories[strings.TrimSpace(j.Category)],
	}, true
}
