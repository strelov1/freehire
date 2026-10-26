package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// oorwin adapts Oorwin (oorwin.com), an applicant-tracking and CRM suite used mostly by IT
// staffing firms in the US and India. Each customer's career portal is an Angular app at
// https://<board>.oorwin.com/careers/ that reads one POST-only JSON API shared by every tenant
// and keyed by the portal's label in the body:
//
//	api.oorwin.ai/api/v2/careers/getJobList  {"sub_domain": <board>, "limit", "page", ...}
//	api.oorwin.ai/api/v2/careers/job_view    {"sub_domain": <board>, "job_id": <sha1>}
//
// The listing carries a posting's title, location, work arrangement, job type and experience
// range but no body, so this is a HydratingSource. It honours a limit far above any board's
// size (5000 answered asiapower's 2560 postings in one page, measured 2026-10-26), and states
// its own total, which is what the walk is held to. A label the platform does not know answers
// 200 with {"status":404,"message":"Invalid request! ..."} — the board is gone — while a known
// portal with nothing open answers success with total 0.
//
// The detail is heavy: besides the posting it carries the whole application form (~600 KB
// uncompressed, ~60 KB gzipped) and takes 3-6 s to answer, so the first crawl of a large board
// is bounded per run (oorwinMaxNewPerRun) and the fan-out shares a small in-flight cap across
// the whole run (oorwinMaxInFlight). A posting taken down answers success 0 "The job was no
// longer active or expired".
//
// Tenants on Oorwin's US instance (<board>.oorwin.us, api.oorwinai.us) are not read: no board
// in the catalogue lives there yet. The seam is the API host.
type oorwin struct {
	http         JSONPoster
	maxNewPerRun int
}

// NewOorwin builds the Oorwin adapter over the given poster.
func NewOorwin(c JSONPoster) Source { return oorwin{http: c, maxNewPerRun: oorwinMaxNewPerRun} }

func (oorwin) Provider() string { return "oorwin" }

// fullBoardListing: the walk ends only once it holds the listing's own stated total, and any
// page failing fails the crawl.
func (oorwin) fullBoardListing() {}

const (
	oorwinAPI      = "https://api.oorwin.ai/api/v2/careers/"
	oorwinPageSize = 5000
	// oorwinMaxPages is a safety ceiling: at oorwinPageSize no board measured needs a second
	// page, so reaching it without the stated total means the paging is not doing what it
	// says, and the crawl fails rather than reporting a truncated board.
	oorwinMaxPages = 20
	// oorwinDetailWorkers is the per-board detail fan-out; oorwinMaxInFlight bounds the whole
	// run, since every tenant's detail is answered by the same API host.
	oorwinDetailWorkers = 4
	oorwinMaxInFlight   = 4
	// oorwinMaxNewPerRun bounds how many unseen postings one board hydrates per run. A detail
	// takes ~4 s and the run keeps four in flight, so the provider hydrates ~1 posting a second
	// in total. A buffered board persists only when its Fetch returns, so the bound is sized for
	// the WHOLE run to fit the 3000 s ingest timeout: seven portals hold more than 150 postings,
	// so a first run hydrates ~1,650 (~28 minutes) and the 7.6k backlog drains over about five
	// runs. A posting past the bound is not yielded (nothing is stored for it yet) and is
	// hydrated by a later run.
	oorwinMaxNewPerRun = 150
)

// oorwinBoard is a career-portal label.
var oorwinBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// oorwinListRequest is the listing body. The portal also sends filter and template fields; the
// API answers the same listing without them.
type oorwinListRequest struct {
	SubDomain string `json:"sub_domain"`
	Limit     int    `json:"limit"`
	Page      int    `json:"page"`
	ListType  int    `json:"list_type"`
	Order     string `json:"order"`
	Sort      string `json:"sort"`
}

type oorwinDetailRequest struct {
	SubDomain string `json:"sub_domain"`
	JobID     string `json:"job_id"`
}

// oorwinListResponse is the listing answer. Total is a pointer so a reshaped answer is told
// apart from a portal with nothing open.
type oorwinListResponse struct {
	Success int    `json:"success"`
	Message string `json:"message"`
	Data    struct {
		ListDetails struct {
			Total     *int            `json:"total"`
			Data      []oorwinListing `json:"data"`
			OtherInfo struct {
				CompanyDetails struct {
					DisplayName string `json:"display_name"`
				} `json:"company_details"`
			} `json:"other_info"`
		} `json:"list_details"`
	} `json:"data"`
}

// oorwinListing is one listing row; a field the portal sends as null decodes as "". The numeric id is not read: it is per-tenant and its JSON
// type varies (a number on one portal, a string on another); the SHA-1 is what the portal
// addresses a posting by.
type oorwinListing struct {
	SHA1            string `json:"computed_sha1_job_id"`
	Title           string `json:"title"`
	JobType         string `json:"job_type"`
	ExperienceRange string `json:"experience_range"`
	RemoteStatus    string `json:"remote_status"`
	City            string `json:"city"`
	State           string `json:"state_format_name"`
	Country         string `json:"country_format_name"`
	PublishedOn     string `json:"cp_published_on"`
}

type oorwinDetailResponse struct {
	Success int    `json:"success"`
	Message string `json:"message"`
	Data    struct {
		JobDetails struct {
			SHA1        string `json:"computed_sha1_job_id"`
			Title       string `json:"title"`
			Description string `json:"job_description"`
		} `json:"job_details"`
	} `json:"data"`
}

func oorwinPageURL(board, sha1 string) string {
	return "https://" + board + ".oorwin.com/careers/index.html#/job/details/" + sha1
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s oorwin) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole portal, refreshes the postings already stored and hydrates up to
// maxNewPerRun of the rest.
func (s oorwin) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, company, rows, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	// The portal's own display name is the employer's; the catalogue's is the fallback, since a
	// board seeded from a bare label carries the label as its company.
	company = firstNonEmpty(company, e.Company)
	var unseen []oorwinListing
	var refreshed []Job
	for _, r := range rows {
		if seen(r.SHA1) {
			refreshed = append(refreshed, Job{ExternalID: r.SHA1, URL: oorwinPageURL(board, r.SHA1),
				Title: strings.TrimSpace(r.Title), Company: company, SeenRefresh: true})
			continue
		}
		if len(unseen) < s.maxNewPerRun {
			unseen = append(unseen, r)
		}
	}
	jobs := fetchDetails(unseen, oorwinDetailWorkers, func(r oorwinListing) (Job, bool) {
		return s.detail(ctx, board, company, r)
	})
	return append(jobs, refreshed...), nil
}

// list walks the portal's listing until it holds the stated total, each posting once. Any
// page failing, or the ceiling reached first, fails the crawl: a short listing would read to
// the sweep as postings gone.
func (s oorwin) list(ctx context.Context, rawBoard string) (board, company string, out []oorwinListing, err error) {
	board = strings.ToLower(strings.TrimSpace(rawBoard))
	if !oorwinBoard.MatchString(board) {
		return "", "", nil, fmt.Errorf("oorwin: board %q is not a career-portal label", rawBoard)
	}
	seenIDs := make(map[string]bool)
	for page := 1; page <= oorwinMaxPages; page++ {
		var resp oorwinListResponse
		req := oorwinListRequest{SubDomain: board, Limit: oorwinPageSize, Page: page, ListType: 1,
			Order: "cp_published_on", Sort: "desc"}
		if err := s.http.PostJSON(ctx, oorwinAPI+"getJobList", req, &resp); err != nil {
			return "", "", nil, fmt.Errorf("oorwin: board %s page %d: %w", board, page, err)
		}
		if resp.Success != 1 {
			if page == 1 && strings.Contains(resp.Message, "Invalid request") {
				return "", "", nil, fmt.Errorf("oorwin: board %q: %w", board, ErrBoardGone)
			}
			return "", "", nil, fmt.Errorf("oorwin: board %s page %d: %q", board, page, resp.Message)
		}
		ld := resp.Data.ListDetails
		if ld.Total == nil {
			return "", "", nil, fmt.Errorf("oorwin: board %s page %d answer carries no total", board, page)
		}
		company = firstNonEmpty(company, strings.TrimSpace(ld.OtherInfo.CompanyDetails.DisplayName))
		for _, r := range ld.Data {
			r.SHA1 = strings.TrimSpace(r.SHA1)
			if r.SHA1 == "" || seenIDs[r.SHA1] {
				continue
			}
			seenIDs[r.SHA1] = true
			out = append(out, r)
		}
		if len(out) >= *ld.Total {
			return board, company, out, nil
		}
		// An empty page short of the total is the listing ending early, not the board.
		if len(ld.Data) == 0 {
			return "", "", nil, fmt.Errorf("oorwin: board %s listed %d of its stated %d postings", board, len(out), *ld.Total)
		}
	}
	return "", "", nil, fmt.Errorf("oorwin: board %s listed %d postings in %d pages without reaching its total", board, len(out), oorwinMaxPages)
}

// detail fetches one posting's body and maps the posting. The platform's "no longer active"
// answer drops it; any other failure, or an answer that is not this posting, is an
// unreadableDetail marker — the detail is the only source of the body.
func (s oorwin) detail(ctx context.Context, board, company string, r oorwinListing) (Job, bool) {
	pageURL := oorwinPageURL(board, r.SHA1)
	var d oorwinDetailResponse
	if err := s.http.PostJSON(ctx, oorwinAPI+"job_view", oorwinDetailRequest{SubDomain: board, JobID: r.SHA1}, &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(r.SHA1, pageURL, company), true
		}
		return Job{}, false
	}
	if d.Success != 1 {
		if strings.Contains(d.Message, "no longer active") {
			return Job{}, false
		}
		return unreadableDetail(r.SHA1, pageURL, company), true
	}
	jd := d.Data.JobDetails
	if strings.TrimSpace(jd.SHA1) != r.SHA1 {
		return unreadableDetail(r.SHA1, pageURL, company), true
	}
	country := strings.TrimSpace(r.Country)
	workMode := oorwinWorkMode(r.RemoteStatus)
	j := Job{
		ExternalID:         r.SHA1,
		URL:                pageURL,
		Title:              firstNonEmpty(strings.TrimSpace(jd.Title), strings.TrimSpace(r.Title)),
		Company:            company,
		Location:           oorwinLocation(r.City, r.State, country),
		Description:        sanitizeHTML(jd.Description),
		WorkMode:           workMode,
		Remote:             workMode == "remote",
		EmploymentType:     oorwinEmploymentType(r.JobType),
		ExperienceYearsMin: oorwinExperienceMin(r.ExperienceRange),
		PostedAt:           parseLayout("2006-01-02 15:04:05.000", strings.TrimSpace(r.PublishedOn)),
	}
	if c := location.NormalizeCountry(country); c != "" {
		j.Countries = []string{c}
	}
	return j, true
}

// oorwinLocation joins the posting's cities (a comma-joined list when it names several: "Jersey
// City, NJ,Dallas, TX,Tampa, FL"), or its states when it names no city, with its country.
func oorwinLocation(city, state, country string) string {
	place := firstNonEmpty(strings.TrimSpace(city), strings.TrimSpace(state))
	if place == "" {
		return country
	}
	if country == "" {
		return place
	}
	return place + ", " + country
}

// oorwinWorkMode maps the portal's remote status — OnSite, Hybrid or Remote, measured across
// 1,665 listed postings, the first hundred of each of 53 portals (147 state none).
func oorwinWorkMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "onsite":
		return "onsite"
	case "hybrid":
		return "hybrid"
	case "remote":
		return "remote"
	}
	return ""
}

// oorwinEmploymentType maps the job type. Measured on the same sample: Contractual 902, Permanent 276, Full Time
// 205, C2H (contract-to-hire) 8, none 273. "Permanent" says how long, not how many hours, so it
// stays "" like none; C2H starts as a contract.
func oorwinEmploymentType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "contractual", "contract", "c2h":
		return "contract"
	case "full time", "full-time":
		return "full_time"
	case "part time", "part-time":
		return "part_time"
	case "internship", "intern":
		return "internship"
	}
	return ""
}

// oorwinExperienceMin reads the lower bound of the experience range ("8-10", or a bare "5").
// A lower bound of 0 ("0-30" is common) states no requirement, so it is left unset.
func oorwinExperienceMin(v string) *int {
	lo, _, _ := strings.Cut(strings.TrimSpace(v), "-")
	n, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil || n <= 0 || n > 50 {
		return nil
	}
	return &n
}
