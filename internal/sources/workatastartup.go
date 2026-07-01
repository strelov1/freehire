package sources

import (
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
)

// workatastartup adapts Work at a Startup (workatastartup.com), Y Combinator's job board.
// Boardless (one Algolia-backed index, no per-tenant board) and multi-company, so it stays
// in the source facet and takes each posting's company from the hit. The board is gated:
// the public page ships an Algolia key neutered to return nothing, and a working key is only
// embedded in a logged-in session. So this adapter needs that session's Algolia search key,
// supplied out-of-band via WAAS_ALGOLIA_KEY (a long-lived secured search key, not a login).
// With the key, the index is queried directly — every hit carries the full posting (company,
// title, markdown description, location, remote, date), so there is no per-job detail call.
//
// Two Algolia quirks shape the crawl. (1) The index defaults to distinct-by-company, collapsing
// its ~5k postings to one-per-company (~1.3k); every query therefore sets distinct=false so all
// postings are visible. (2) Offset pagination is capped at 1000 hits (paginationLimitedTo), well
// under the full count, so a single sweep would silently truncate. Instead the crawl bisects the
// id space: each query filters a half-open [lo, hi) id window and, whenever a window still fills
// a page, splits it in half and recurses. Because id is unique per posting, a width-1 window
// holds at most one hit, so the recursion always terminates and reaches every posting — the
// standard workaround for the offset cap, with id (unique, numeric, filterable) as the key.
type workatastartup struct {
	http HeaderJSONPoster
}

const (
	waasKeyEnv     = "WAAS_ALGOLIA_KEY"
	waasAlgoliaApp = "45BWZJ1SGC"
	waasAlgoliaIdx = "WaaSPublicCompanyJob_production"
	// waasHitsPerPage is Algolia's max page size and its offset-pagination cap (paginationLimitedTo):
	// a window returning a full page is assumed to hold more and gets bisected.
	waasHitsPerPage = 1000
	// waasIDProbeBase seeds the exponential search for the id upper bound; current ids are ~1e5.
	waasIDProbeBase = 1 << 16
)

// NewWorkAtAStartup builds the Work at a Startup adapter over the given HTTP client.
func NewWorkAtAStartup(c HeaderJSONPoster) Source { return workatastartup{http: c} }

func (workatastartup) Provider() string { return "workatastartup" }

func (workatastartup) boardless() {}

func (workatastartup) aggregator() {}

// waasHit is one job from the Algolia index, body inline (no detail call). remote is
// "no" | "yes" | "only"; search_path is the canonical job URL. locations_for_search is
// heterogeneous — usually []string, but some records carry nested-array garbage (e.g.
// [[[["Remote"]]], "Remote"]) — so it decodes as raw elements and firstLocation picks the
// first scalar (see toJob).
type waasHit struct {
	ID               json.Number       `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	Remote           string            `json:"remote"`
	CreatedAt        string            `json:"created_at"`
	CompanyName      string            `json:"company_name"`
	LocationsForSrch []json.RawMessage `json:"locations_for_search"`
	SearchPath       string            `json:"search_path"`
}

func (s workatastartup) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	key := os.Getenv(waasKeyEnv)
	if key == "" {
		return nil, fmt.Errorf("workatastartup: %s is not set (needs a logged-in session's Algolia search key)", waasKeyEnv)
	}
	headers := map[string]string{
		"X-Algolia-Application-Id": waasAlgoliaApp,
		"X-Algolia-API-Key":        key,
	}
	url := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", waasAlgoliaApp, waasAlgoliaIdx)

	// Find an id upper bound: the smallest power-of-two past every posting's id. Exponential so it
	// self-adjusts as ids grow, with no hard-coded ceiling that could one day silently truncate.
	hi := int64(waasIDProbeBase)
	for {
		hits, err := s.query(ctx, headers, url, hi, -1, 1)
		if err != nil {
			return nil, err
		}
		if len(hits) == 0 {
			break
		}
		hi *= 2
	}

	var jobs []Job
	if err := s.collect(ctx, headers, url, 0, hi, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// collect gathers every posting whose id is in the half-open window [lo, hi), recursively
// bisecting a window that still fills a page (so more remain past the offset cap). id uniqueness
// guarantees termination: a width-1 window holds at most one hit.
func (s workatastartup) collect(ctx context.Context, headers map[string]string, url string, lo, hi int64, out *[]Job) error {
	hits, err := s.query(ctx, headers, url, lo, hi, waasHitsPerPage)
	if err != nil {
		return err
	}
	if len(hits) < waasHitsPerPage || hi-lo <= 1 {
		for _, h := range hits {
			if job, ok := h.toJob(); ok {
				*out = append(*out, job)
			}
		}
		return nil
	}
	mid := lo + (hi-lo)/2
	if err := s.collect(ctx, headers, url, lo, mid, out); err != nil {
		return err
	}
	return s.collect(ctx, headers, url, mid, hi, out)
}

// query runs one Algolia search over the id window with distinct off (the index defaults to
// distinct-by-company, which would hide most postings). hi < 0 means an open upper bound (used to
// probe for postings at or above lo); otherwise the window is half-open [lo, hi).
func (s workatastartup) query(ctx context.Context, headers map[string]string, url string, lo, hi int64, hitsPerPage int) ([]waasHit, error) {
	filters := fmt.Sprintf(`["id>=%d","id<%d"]`, lo, hi)
	if hi < 0 {
		filters = fmt.Sprintf(`["id>=%d"]`, lo)
	}
	p := neturl.Values{}
	p.Set("query", "")
	p.Set("distinct", "false")
	p.Set("hitsPerPage", strconv.Itoa(hitsPerPage))
	p.Set("numericFilters", filters)

	body := map[string]any{"params": p.Encode()}
	var resp struct {
		Hits []waasHit `json:"hits"`
	}
	if err := s.http.PostJSONWithHeaders(ctx, url, headers, body, &resp); err != nil {
		return nil, fmt.Errorf("workatastartup: id window [%d,%d): %w", lo, hi, err)
	}
	return resp.Hits, nil
}

// toJob maps an Algolia hit to a Job, returning ok=false for an unusable hit (no id, which
// would collide on the dedup key, or no company which would break the slug). The markdown
// description is rendered to sanitized HTML; remote "only"/"yes" set the work mode.
func (h waasHit) toJob() (Job, bool) {
	id := h.ID.String()
	if id == "" || id == "0" || h.CompanyName == "" {
		return Job{}, false
	}
	location := firstLocation(h.LocationsForSrch)
	url := h.SearchPath
	if url == "" {
		url = fmt.Sprintf("https://www.workatastartup.com/jobs/%s", id)
	}
	workMode := waasWorkMode(h.Remote)
	return Job{
		ExternalID:  id,
		URL:         url,
		Title:       h.Title,
		Company:     h.CompanyName,
		Location:    location,
		Description: sanitizeHTML(markdownToHTML(h.Description)),
		Remote:      workMode == "remote" || workMode == "hybrid",
		WorkMode:    workMode,
		PostedAt:    parseRFC3339(h.CreatedAt),
	}, true
}

// firstLocation returns the first locations_for_search element that is a plain JSON string,
// skipping the nested-array garbage some records carry there. Empty when none is a scalar.
func firstLocation(raw []json.RawMessage) string {
	for _, e := range raw {
		var s string
		if json.Unmarshal(e, &s) == nil {
			return s
		}
	}
	return ""
}

// waasWorkMode maps WaaS's remote flag to the controlled work-mode vocabulary:
// "only" = remote-only, "yes" = remote allowed, "no" = in-office.
func waasWorkMode(remote string) string {
	switch strings.ToLower(remote) {
	case "only", "yes":
		return "remote"
	case "no":
		return "onsite"
	default:
		return ""
	}
}
