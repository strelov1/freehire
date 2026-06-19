package sources

import (
	"context"
	"fmt"
	"strings"
)

// apple adapts Apple's public careers search API (jobs.apple.com/api/v1), a single-company
// source with no per-tenant board id (boardless). The listing is a POST to /api/v1/search,
// paged 20 at a time (the page size is server-fixed; a larger request is ignored). Its
// results carry only a generic team summary, so each posting's role-specific description
// comes from a /api/v1/jobDetails/<positionId> GET, fanned out under a bounded worker pool
// like the other detail adapters.
//
// Two non-obvious API facts, both learned from the live endpoint:
//   - The request body's filters object is REQUIRED even when empty. Omitting it makes the
//     API reject the body with HTTP 436 ("jobsite.general.serviceError"), so an empty
//     filters:{} is always sent. No CSRF token or session cookie is needed.
//   - The listing returns one row per (position, location), so a multi-location role recurs
//     under the same positionId across the result set; it is deduped to one job before the
//     detail fan-out so its detail is fetched once.
type apple struct {
	http appleHTTP
}

// appleHTTP is the transport apple needs: a JSON POST for the paged listing and a JSON GET
// for each posting's detail.
type appleHTTP interface {
	JSONPoster
	JSONGetter
}

const (
	appleSearchURL = "https://jobs.apple.com/api/v1/search"
	appleDetailURL = "https://jobs.apple.com/api/v1/jobDetails/%s?locale=" + appleLocale
	appleJobURL    = "https://jobs.apple.com/" + appleLocale + "/details/%s/%s"
	// appleLocale is the single locale crawled; the en-us catalogue is the global superset.
	appleLocale = "en-us"
)

// NewApple builds the Apple careers adapter over the given HTTP client.
func NewApple(c appleHTTP) Source { return apple{http: c} }

func (apple) Provider() string { return "apple" }

// apple is single-company, so its config entries carry no board.
func (apple) boardless() {}

// appleLocation is one location on a posting; name is the human display string.
type appleLocation struct {
	Name string `json:"name"`
}

// applePosting is one result from the search listing. positionId is the stable id used for
// the detail fetch, the public URL, and the dedup key; the rich, role-specific description
// is NOT here — it comes from the detail fetch.
type applePosting struct {
	PositionID    string          `json:"positionId"`
	PostingTitle  string          `json:"postingTitle"`
	Slug          string          `json:"transformedPostingTitle"`
	Locations     []appleLocation `json:"locations"`
	PostDateInGMT string          `json:"postDateInGMT"`
	HomeOffice    bool            `json:"homeOffice"`
}

// appleSearchResponse wraps the search listing payload; every response nests its body under
// "res".
type appleSearchResponse struct {
	Res struct {
		SearchResults []applePosting `json:"searchResults"`
		TotalRecords  int            `json:"totalRecords"`
	} `json:"res"`
}

func (s apple) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	postings = dedupApplePostings(postings)
	return fetchDetails(postings, defaultDetailWorkers, func(p applePosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// list pages /api/v1/search until a page comes back empty or the catalogue total is reached.
func (s apple) list(ctx context.Context) ([]applePosting, error) {
	var postings []applePosting
	for page := 1; ; page++ {
		var resp appleSearchResponse
		if err := s.http.PostJSON(ctx, appleSearchURL, appleSearchBody(page), &resp); err != nil {
			return nil, fmt.Errorf("apple: list page %d: %w", page, err)
		}
		results := resp.Res.SearchResults
		if len(results) == 0 {
			break
		}
		postings = append(postings, results...)
		if resp.Res.TotalRecords > 0 && len(postings) >= resp.Res.TotalRecords {
			break
		}
	}
	return postings, nil
}

// appleSearchBody is the search request for one page. filters is sent as an empty object
// because the API rejects a body without it (see the type comment); format mirrors what the
// site sends and governs the human-readable date strings (which the adapter ignores).
func appleSearchBody(page int) map[string]any {
	return map[string]any{
		"query":   "",
		"filters": map[string]any{},
		"page":    page,
		"locale":  appleLocale,
		"sort":    "newest",
		"format": map[string]string{
			"longDate":   "MMMM D, YYYY",
			"mediumDate": "MMM D, YYYY",
		},
	}
}

// detail fetches one posting's jobDetails and maps it to a Job, returning ok=false when the
// request fails or the posting has no id/description so the caller skips just that posting.
func (s apple) detail(ctx context.Context, e CompanyEntry, p applePosting) (Job, bool) {
	if p.PositionID == "" {
		return Job{}, false
	}
	var resp struct {
		Res struct {
			Description             string `json:"description"`
			MinimumQualifications   string `json:"minimumQualifications"`
			PreferredQualifications string `json:"preferredQualifications"`
		} `json:"res"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(appleDetailURL, p.PositionID), &resp); err != nil {
		return Job{}, false
	}
	desc := appleDescription(resp.Res.Description, resp.Res.MinimumQualifications, resp.Res.PreferredQualifications)
	if desc == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  p.PositionID,
		URL:         fmt.Sprintf(appleJobURL, p.PositionID, p.Slug),
		Title:       strings.TrimSpace(p.PostingTitle),
		Company:     e.Company,
		Location:    appleLocationString(p.Locations),
		Description: desc,
		Remote:      p.HomeOffice,
		WorkMode:    workModeFromRemote(p.HomeOffice),
		PostedAt:    parseRFC3339(p.PostDateInGMT),
	}, true
}

// dedupApplePostings keeps the first posting per positionId, preserving listing order, so a
// multi-location role (which recurs once per location) is fetched and stored once.
func dedupApplePostings(in []applePosting) []applePosting {
	seen := make(map[string]bool, len(in))
	out := make([]applePosting, 0, len(in))
	for _, p := range in {
		if p.PositionID == "" || seen[p.PositionID] {
			continue
		}
		seen[p.PositionID] = true
		out = append(out, p)
	}
	return out
}

// appleLocationString joins a posting's location display names with "; ", skipping blanks.
func appleLocationString(locs []appleLocation) string {
	names := make([]string, 0, len(locs))
	for _, l := range locs {
		if n := strings.TrimSpace(l.Name); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, "; ")
}

// appleDescription assembles the role's full description from the detail fields. Apple serves
// these as plain text (no markup), so they are joined with blank lines under their section
// headers rather than HTML-sanitized; an empty section is omitted.
func appleDescription(description, minQual, prefQual string) string {
	parts := make([]string, 0, 3)
	if d := strings.TrimSpace(description); d != "" {
		parts = append(parts, d)
	}
	if m := strings.TrimSpace(minQual); m != "" {
		parts = append(parts, "Minimum Qualifications\n"+m)
	}
	if p := strings.TrimSpace(prefQual); p != "" {
		parts = append(parts, "Preferred Qualifications\n"+p)
	}
	return strings.Join(parts, "\n\n")
}
