package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
// Coverage caveat: Algolia caps offset pagination at 1000 hits per query (paginationLimitedTo),
// while the index holds somewhat more; a single sweep therefore returns the first 1000 and
// logs how many it left behind rather than silently truncating.
type workatastartup struct {
	http HeaderJSONPoster
}

const (
	waasKeyEnv      = "WAAS_ALGOLIA_KEY"
	waasAlgoliaApp  = "45BWZJ1SGC"
	waasAlgoliaIdx  = "WaaSPublicCompanyJob_production"
	waasHitsPerPage = 1000
	// waasMaxPages bounds pagination; Algolia's 1000-hit cap means this is rarely > 1.
	waasMaxPages = 5
)

// NewWorkAtAStartup builds the Work at a Startup adapter over the given HTTP client.
func NewWorkAtAStartup(c HeaderJSONPoster) Source { return workatastartup{http: c} }

func (workatastartup) Provider() string { return "workatastartup" }

func (workatastartup) boardless() {}

func (workatastartup) aggregator() {}

// waasHit is one job from the Algolia index, body inline (no detail call). remote is
// "no" | "yes" | "only"; search_path is the canonical job URL.
type waasHit struct {
	ID               json.Number `json:"id"`
	Title            string      `json:"title"`
	Description      string      `json:"description"`
	Remote           string      `json:"remote"`
	CreatedAt        string      `json:"created_at"`
	CompanyName      string      `json:"company_name"`
	LocationsForSrch []string    `json:"locations_for_search"`
	SearchPath       string      `json:"search_path"`
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

	var jobs []Job
	for page := 0; page < waasMaxPages; page++ {
		body := map[string]any{"params": fmt.Sprintf("hitsPerPage=%d&page=%d&query=", waasHitsPerPage, page)}
		var resp struct {
			Hits    []waasHit `json:"hits"`
			NbHits  int       `json:"nbHits"`
			NbPages int       `json:"nbPages"`
		}
		if err := s.http.PostJSONWithHeaders(ctx, url, headers, body, &resp); err != nil {
			return nil, fmt.Errorf("workatastartup: page %d: %w", page, err)
		}
		for _, h := range resp.Hits {
			if job, ok := h.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if page == 0 && resp.NbHits > len(resp.Hits) {
			// Algolia's 1000-hit pagination cap: report what the sweep cannot reach.
			log.Printf("workatastartup: index has %d jobs, retrievable cap is %d — %d not fetched this run",
				resp.NbHits, len(resp.Hits)*max(resp.NbPages, 1), resp.NbHits-len(resp.Hits)*max(resp.NbPages, 1))
		}
		if page+1 >= resp.NbPages || len(resp.Hits) == 0 {
			break
		}
	}
	return jobs, nil
}

// toJob maps an Algolia hit to a Job, returning ok=false for an unusable hit (no id, which
// would collide on the dedup key, or no company which would break the slug). The markdown
// description is rendered to sanitized HTML; remote "only"/"yes" set the work mode.
func (h waasHit) toJob() (Job, bool) {
	id := h.ID.String()
	if id == "" || id == "0" || h.CompanyName == "" {
		return Job{}, false
	}
	location := ""
	if len(h.LocationsForSrch) > 0 {
		location = h.LocationsForSrch[0]
	}
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
