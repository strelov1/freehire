package sources

import (
	"context"
	"fmt"
	"strings"
)

// phenom adapts Phenom People career sites (e.g. careers.dhl.com). The board is the
// site host; both the listing and the per-job detail are POSTs to
// "https://<board>/widgets" distinguished by a "ddoKey" — refineSearch for the paged
// job list, jobDetail for one posting's full HTML description. The list carries only a
// teaser, so each posting needs a detail fetch (bounded-concurrency), like the other
// detail adapters.

// phenomPageSize is the refineSearch page size; the list pages by "from" and stops on a
// short/empty page (the API's totalHits is not relied upon).
const phenomPageSize = 100

// phenomDateLayout is Phenom's postedDate format: RFC3339 with milliseconds and a
// numeric, colon-less zone offset ("2026-05-17T22:00:00.000+0000").
const phenomDateLayout = "2006-01-02T15:04:05.000-0700"

type phenom struct {
	http JSONPoster
}

// NewPhenom builds the Phenom adapter over the given HTTP client.
func NewPhenom(c JSONPoster) Source { return phenom{http: c} }

func (phenom) Provider() string { return "phenom" }

// phenomPosting is one job from the refineSearch list (no full description here).
type phenomPosting struct {
	JobSeqNo   string `json:"jobSeqNo"`
	Title      string `json:"title"`
	CityState  string `json:"cityState"`
	Locale     string `json:"locale"`
	PostedDate string `json:"postedDate"`
}

func (s phenom) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	// Each posting's full description comes from its own jobDetail request, fanned out
	// under a bounded worker pool.
	return fetchDetails(postings, defaultDetailWorkers, func(p phenomPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// list pages the refineSearch widget. lang/country are deliberately omitted from the
// body: they do not change the result set (verified), so the board host is the only
// per-tenant input.
func (s phenom) list(ctx context.Context, board string) ([]phenomPosting, error) {
	url := fmt.Sprintf("https://%s/widgets", board)
	var postings []phenomPosting
	for from := 0; ; from += phenomPageSize {
		body := map[string]any{
			"deviceType": "desktop",
			"ddoKey":     "refineSearch",
			"jobs":       true,
			"from":       from,
			"size":       phenomPageSize,
		}
		var resp struct {
			RefineSearch struct {
				Data struct {
					Jobs []phenomPosting `json:"jobs"`
				} `json:"data"`
			} `json:"refineSearch"`
		}
		if err := s.http.PostJSON(ctx, url, body, &resp); err != nil {
			return nil, fmt.Errorf("phenom: list board %s: %w", board, err)
		}
		page := resp.RefineSearch.Data.Jobs
		if len(page) == 0 {
			break
		}
		postings = append(postings, page...)
		if len(page) < phenomPageSize {
			break
		}
	}
	return postings, nil
}

// detail fetches one posting's jobDetail and maps it to a Job, returning ok=false when
// the request fails or carries no description so the caller skips just that posting.
func (s phenom) detail(ctx context.Context, e CompanyEntry, p phenomPosting) (Job, bool) {
	url := fmt.Sprintf("https://%s/widgets", e.Board)
	body := map[string]any{
		"deviceType": "desktop",
		"ddoKey":     "jobDetail",
		"pageName":   "job-details",
		"jobSeqNo":   p.JobSeqNo,
	}
	var resp struct {
		JobDetail struct {
			Data struct {
				Job struct {
					Description string `json:"description"`
					Title       string `json:"title"`
				} `json:"job"`
			} `json:"data"`
		} `json:"jobDetail"`
	}
	if err := s.http.PostJSON(ctx, url, body, &resp); err != nil {
		return Job{}, false
	}
	job := resp.JobDetail.Data.Job
	if job.Description == "" {
		return Job{}, false
	}

	return Job{
		ExternalID:  p.JobSeqNo,
		URL:         phenomJobURL(e.Board, p.Locale, p.JobSeqNo),
		Title:       firstNonEmpty(p.Title, job.Title),
		Company:     e.Company,
		Location:    p.CityState,
		Description: sanitizeHTML(job.Description),
		Remote:      isRemote(p.CityState),
		PostedAt:    parseLayout(phenomDateLayout, p.PostedDate),
	}, true
}

// phenomJobURL builds a posting's public page from its locale and sequence id. Phenom
// paths are "/<country>/<lang>/job/<jobSeqNo>" and the list locale is "<lang>_<COUNTRY>"
// (e.g. "en_GLOBAL" -> "/global/en/job/..."). A locale without an underscore falls back
// to the unlocalized "/job/<jobSeqNo>".
func phenomJobURL(board, locale, seq string) string {
	if lang, country, ok := strings.Cut(locale, "_"); ok && lang != "" && country != "" {
		return fmt.Sprintf("https://%s/%s/%s/job/%s", board, strings.ToLower(country), strings.ToLower(lang), seq)
	}
	return fmt.Sprintf("https://%s/job/%s", board, seq)
}
