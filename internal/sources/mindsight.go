package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// mindsightBase is the single host Mindsight serves every tenant's public career page
// from; a board is the tenant's path segment (oportunidades.mindsight.com.br/<slug>).
const mindsightBase = "https://oportunidades.mindsight.com.br"

// mindsightOpen is the status marking a live posting; the listing only returns published
// postings, but the guard keeps a non-open one out should the shape ever widen.
const mindsightOpen = "IN_PROGRESS"

// mindsight adapts Mindsight's public career pages (oportunidades.mindsight.com.br), a
// Brazilian ATS. Each page is a Next.js app embedding its data in __NEXT_DATA__: the
// board listing carries every posting's structured fields (id/title/location/work model/
// dates) but not the body, so each posting's description comes from its own detail page,
// fanned out like the other detail-fetching adapters.
type mindsight struct {
	http TextGetter
}

// NewMindsight builds the Mindsight adapter over the given HTTP client.
func NewMindsight(c TextGetter) Source { return mindsight{http: c} }

func (mindsight) Provider() string { return "mindsight" }

// mindsightPost is one posting from the listing's publicJobPostings array. work_model is a
// structured IN_PERSON/REMOTE/HYBRID enum; country is an ISO alpha-2 code, state an ISO
// subdivision code; the description is absent here and fetched per posting.
type mindsightPost struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	WorkModel string `json:"work_model"`
	Country   string `json:"country"`
	State     string `json:"state"`
	City      string `json:"city"`
	StartAt   string `json:"external_publication_start_at"`
	CreatedAt string `json:"created_at"`
}

func (m mindsight) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	body, err := m.http.GetText(ctx, fmt.Sprintf("%s/%s", mindsightBase, e.Board))
	if err != nil {
		return nil, fmt.Errorf("mindsight: listing %q: %w", e.Board, err)
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return nil, fmt.Errorf("mindsight: board %q: no __NEXT_DATA__ in listing", e.Board)
	}
	var data struct {
		Props struct {
			PageProps struct {
				PublicJobPostings []mindsightPost `json:"publicJobPostings"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("mindsight: board %q: decode listing: %w", e.Board, err)
	}

	posts := data.Props.PageProps.PublicJobPostings
	return fetchDetails(posts, defaultDetailWorkers, func(p mindsightPost) (Job, bool) {
		return m.detail(ctx, e, p)
	}), nil
}

// detail builds a Job from a listing post, enriching it with the description from the
// posting's own detail page. The listing post is authoritative for the posting's existence
// and structured fields, so a failed detail fetch only costs the description. A non-open
// post (or one with no id, which would collide on the dedup key) is dropped.
func (m mindsight) detail(ctx context.Context, e CompanyEntry, p mindsightPost) (Job, bool) {
	if p.ID == 0 || (p.Status != "" && p.Status != mindsightOpen) {
		return Job{}, false
	}
	url := fmt.Sprintf("%s/%s/%d", mindsightBase, e.Board, p.ID)

	description := ""
	if body, err := m.http.GetText(ctx, url); err == nil {
		if raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}'); ok {
			var d struct {
				Props struct {
					PageProps struct {
						JobPosting struct {
							Description string `json:"description"`
						} `json:"jobPosting"`
					} `json:"pageProps"`
				} `json:"props"`
			}
			if json.Unmarshal([]byte(raw), &d) == nil {
				description = sanitizeHTML(d.Props.PageProps.JobPosting.Description)
			}
		}
	}

	// Prefer the publication date, falling back to created_at — but only when the
	// former yields no usable date. firstNonEmpty on the raw strings would let a
	// present-but-unparseable/future StartAt (which parseRFC3339/NotFuture drops to nil)
	// shadow a valid CreatedAt, leaving the job undated; parse both and keep the first hit.
	posted := parseRFC3339(p.StartAt)
	if posted == nil {
		posted = parseRFC3339(p.CreatedAt)
	}

	mode := mindsightWorkMode(p.WorkModel)
	return Job{
		ExternalID:  strconv.Itoa(p.ID),
		URL:         url,
		Title:       strings.TrimSpace(p.Name),
		Company:     e.Company,
		Location:    joinNonEmpty(p.City, p.State, p.Country),
		Description: description,
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    posted,
	}, true
}

// mindsightWorkMode maps Mindsight's work_model enum to our work-mode vocabulary; an
// empty or unrecognized value yields "".
func mindsightWorkMode(model string) string {
	switch strings.ToUpper(strings.TrimSpace(model)) {
	case "REMOTE":
		return "remote"
	case "HYBRID":
		return "hybrid"
	case "IN_PERSON":
		return "onsite"
	default:
		return ""
	}
}
