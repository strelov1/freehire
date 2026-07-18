package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// bullhorn adapts Bullhorn ATS career boards through the vendor's public REST API. Bullhorn
// is the largest staffing ATS, so a board is a staffing agency rather than a single employer;
// the company name comes from config (the API exposes only the opaque corpToken).
//
// The board is "<cls>:<corpToken>": cls is the numeric cluster (a.k.a. swimlane) that selects
// the API host public-rest<cls>.bullhornstaffing.com, and corpToken is the public alphanumeric
// company token embedded in every Bullhorn career portal. Both are public — the public API is
// keyless, gated only by the customer publishing a job as "Published - Approved". One
// search/JobOrder call returns every open posting fully populated (structured location, an HTML
// publicDescription, dateAdded), so no per-posting detail fetch is needed.
//
// The API exposes no human-facing job page (only this REST endpoint and POST /apply), so the
// job URL is the REST entity reference: it uniquely and stably identifies the posting on
// Bullhorn's own domain. Board sources are not liveness-probed, so a URL that is not GET-able
// without OAuth never triggers a false close.
type bullhorn struct {
	http JSONGetter
}

const (
	// bullhornPageSize is the JobOrder page size. Bullhorn's own career portal requests 500.
	bullhornPageSize = 500
	// bullhornMaxPages bounds pagination as a runaway guard; a large staffing agency can list
	// many thousands of jobs, and 500·40 = 20k caps a single run without truncating typical boards.
	bullhornMaxPages = 40
	// bullhornFields is the JobOrder field projection. address's subfields are requested
	// explicitly — a bare "address" returns an empty object, while address(city,state,countryName)
	// populates the location. Salary and categories are omitted: the Job contract carries no
	// salary (enrichment derives it from the body) and Category expects freehire's controlled
	// vocabulary, not Bullhorn's free-text category names.
	bullhornFields = "id,title,publicDescription,address(city,state,countryName),dateAdded,employmentType"
)

// NewBullhorn builds the Bullhorn adapter over the given HTTP client.
func NewBullhorn(c JSONGetter) Source { return bullhorn{http: c} }

func (bullhorn) Provider() string { return "bullhorn" }

// bullhornSearchResponse is the search/JobOrder envelope. total is the full match count; the
// response returns a start-offset window of it in data.
type bullhornSearchResponse struct {
	Total int                `json:"total"`
	Start int                `json:"start"`
	Count int                `json:"count"`
	Data  []bullhornJobOrder `json:"data"`
}

type bullhornJobOrder struct {
	ID                int             `json:"id"`
	Title             string          `json:"title"`
	PublicDescription string          `json:"publicDescription"`
	Address           bullhornAddress `json:"address"`
	// DateAdded is epoch milliseconds when the posting was created.
	DateAdded      int64  `json:"dateAdded"`
	EmploymentType string `json:"employmentType"`
}

// bullhornAddress is a JobOrder's structured location. The public field set reliably carries
// city/state; country is included when the customer exposes countryName.
type bullhornAddress struct {
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"countryName"`
}

// String renders the location as "City, State", falling back to the country name, then "".
func (a bullhornAddress) String() string {
	var parts []string
	for _, p := range []string{a.City, a.State} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return strings.TrimSpace(a.Country)
}

func (s bullhorn) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	colon := strings.Index(e.Board, ":")
	if colon < 0 {
		return nil, fmt.Errorf("bullhorn: board %q must be %q", e.Board, "<cls>:<corpToken>")
	}
	cls, corpToken := e.Board[:colon], e.Board[colon+1:]
	if cls == "" || corpToken == "" {
		return nil, fmt.Errorf("bullhorn: board %q must be %q with both parts set", e.Board, "<cls>:<corpToken>")
	}
	base := fmt.Sprintf("https://public-rest%s.bullhornstaffing.com/rest-services/%s/search/JobOrder", cls, corpToken)

	var jobs []Job
	for page := 0; page < bullhornMaxPages; page++ {
		start := page * bullhornPageSize
		q := url.Values{}
		q.Set("query", "(isOpen:1)")
		q.Set("fields", bullhornFields)
		q.Set("sort", "-dateAdded")
		q.Set("count", strconv.Itoa(bullhornPageSize))
		q.Set("start", strconv.Itoa(start))

		var resp bullhornSearchResponse
		if err := s.http.GetJSON(ctx, base+"?"+q.Encode(), &resp); err != nil {
			return nil, fmt.Errorf("bullhorn: board %s start %d: %w", e.Board, start, err)
		}
		for _, o := range resp.Data {
			if j, ok := s.toJob(o, cls, corpToken, e); ok {
				jobs = append(jobs, j)
			}
		}
		if len(resp.Data) == 0 || start+len(resp.Data) >= resp.Total {
			break
		}
	}
	return jobs, nil
}

// toJob maps a JobOrder to a Job, returning ok=false for a posting missing an id (which would
// collide on the dedup key). The company is the configured staffing agency; the API has no
// employer name to fall back to.
func (bullhorn) toJob(o bullhornJobOrder, cls, corpToken string, e CompanyEntry) (Job, bool) {
	if o.ID == 0 {
		return Job{}, false
	}
	return Job{
		ExternalID:     strconv.Itoa(o.ID),
		URL:            fmt.Sprintf("https://public-rest%s.bullhornstaffing.com/rest-services/%s/entity/JobOrder/%d", cls, corpToken, o.ID),
		Title:          o.Title,
		Company:        e.Company,
		Location:       o.Address.String(),
		Description:    sanitizeHTML(o.PublicDescription),
		PostedAt:       parseEpochMillis(o.DateAdded),
		EmploymentType: bullhornEmploymentType(o.EmploymentType),
	}, true
}

// bullhornEmploymentType maps Bullhorn's free-text employmentType (per-customer labels like
// "Permanent", "Contract To Hire", "Direct Hire", "Temporary") onto the freehire vocabulary via
// keyword containment, in priority order. An unrecognized value maps to "" so the description
// parser decides — structured signal only, never a guess.
func bullhornEmploymentType(t string) string {
	s := strings.ToLower(t)
	switch {
	case strings.Contains(s, "intern"):
		return "internship"
	case strings.Contains(s, "part-time") || strings.Contains(s, "part time"):
		return "part_time"
	case strings.Contains(s, "contract") || strings.Contains(s, "temp") || strings.Contains(s, "seasonal"):
		return "contract"
	case strings.Contains(s, "full-time") || strings.Contains(s, "full time") ||
		strings.Contains(s, "permanent") || strings.Contains(s, "direct hire"):
		return "full_time"
	default:
		return ""
	}
}
