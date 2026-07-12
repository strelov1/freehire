package sources

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// reed adapts the Reed Jobseeker API (reed.co.uk), the UK's largest job board. Like the
// other aggregators it is boardless (one public API, no per-tenant board) yet lists many
// employers, so it stays in the source facet and takes each posting's employer as the
// company.
//
// It is keyed (besides usajobs): the API is gated behind an API key used as HTTP Basic
// credentials (key as username, blank password), so the adapter is registered only when
// REED_API_KEY is configured (see All). The key is a secret and lives in the environment,
// never in a board file.
//
// The API has no sector filter, only free-text keywords, and freehire is an IT board — so
// the adapter enumerates a topical IT slice by searching a curated keyword set, unioning the
// hits and deduping by Reed job id. The search list omits the employer's real apply URL and
// truncates the description, so a job is fetched in detail for the full body and the employer's
// externalUrl (with the Reed listing URL, jobUrl, as the fallback).
//
// Because the Reed API enforces a per-hour request quota, reed is a HydratingSource: it fetches
// detail ONLY for jobs the catalogue does not already have (FetchNew, driven by the pipeline's
// seen-set), emitting an already-ingested job as a liveness refresh with no detail request. This
// bounds a run's request volume to the day's new postings instead of re-hydrating every live
// posting each crawl, which would exhaust the quota (a 403 "exceeded your per-hour request
// limit"). Fetch is the list-only fallback used when the pipeline cannot supply a seen-set.
type reed struct {
	http   HeaderJSONGetter
	apiKey string
}

const (
	reedSearchURL     = "https://www.reed.co.uk/api/1.0/search"
	reedJobURL        = "https://www.reed.co.uk/api/1.0/jobs/"
	reedPageSize      = 100  // the API's maximum page size
	reedMaxSkip       = 5000 // per-keyword pagination cap; bounds a runaway feed (volumes are < this)
	reedDetailWorkers = 6    // modest detail fan-out; the shared client retries 429/5xx
	// reedDateLayout is the API's day-granular posting date, e.g. "17/06/2026".
	reedDateLayout = "02/01/2006"
)

// reedKeywords is the curated IT/technology slice. The bare "IT" keyword is deliberately
// omitted (it matches "IT support" and stray mentions, ~33k noisy hits); these terms keep
// the crawl to software/data/devops/cloud roles. Overlap across terms is deduped by job id,
// and our classify/skilltag dictionaries drop anything non-IT that slips through.
var reedKeywords = []string{
	"software developer", "software engineer", "web developer", "frontend developer",
	"backend developer", "full stack developer", "mobile developer", "ios developer",
	"android developer", "devops engineer", "site reliability engineer", "platform engineer",
	"cloud engineer", "data engineer", "data scientist", "machine learning engineer",
	"qa engineer", "test engineer", "security engineer", "python developer", "java developer",
	"golang developer", "javascript developer", "typescript developer", "react developer",
	"node developer", ".net developer", "php developer", "ruby developer", "solutions architect",
}

// NewReed builds the Reed adapter over the given HTTP client and API key.
func NewReed(c HeaderJSONGetter, apiKey string) Source { return reed{http: c, apiKey: apiKey} }

func (reed) Provider() string { return "reed" }

// reed needs no board id (one API), so its config carries no board.
func (reed) boardless() {}

// reed aggregates postings from many employers, so it stays in the source facet.
func (reed) aggregator() {}

// reedSearchResponse is one search page; only the ids are read from the list (it truncates
// the description and omits externalUrl — both come from the per-job detail).
type reedSearchResponse struct {
	TotalResults int       `json:"totalResults"`
	Results      []reedJob `json:"results"`
}

// reedJob decodes both the search item and the job detail (the detail is the superset). The
// list populates only JobID; the detail fills the rest.
type reedJob struct {
	JobID          int64  `json:"jobId"`
	EmployerName   string `json:"employerName"`
	JobTitle       string `json:"jobTitle"`
	LocationName   string `json:"locationName"`
	DatePosted     string `json:"datePosted"`
	ExternalURL    string `json:"externalUrl"`
	JobURL         string `json:"jobUrl"`
	JobDescription string `json:"jobDescription"`
}

// toJob maps a job detail to a Job, returning ok=false for an unusable posting (no native id,
// which would collide on the dedup key, or no employer, which would break the company slug).
// The URL is the employer's own externalUrl when present, else the Reed listing page.
func (j reedJob) toJob() (Job, bool) {
	if j.JobID == 0 || j.EmployerName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  strconv.FormatInt(j.JobID, 10),
		URL:         firstNonEmpty(j.ExternalURL, j.JobURL),
		Title:       j.JobTitle,
		Company:     j.EmployerName,
		Location:    j.LocationName,
		Description: sanitizeHTML(j.JobDescription),
		Remote:      isRemote(j.LocationName),
		PostedAt:    parseLayout(reedDateLayout, j.DatePosted),
	}, true
}

func (r reed) authHeaders() map[string]string {
	token := base64.StdEncoding.EncodeToString([]byte(r.apiKey + ":"))
	return map[string]string{"Authorization": "Basic " + token}
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen-set (a non-DB
// caller or a test): it hydrates every unique job's detail. The pipeline prefers FetchNew.
func (r reed) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return r.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew enumerates the IT slice across the curated keywords (deduping by job id), then fetches
// detail ONLY for jobs not already ingested — an already-ingested job (seen) is emitted as a
// liveness refresh (SeenRefresh) carrying just its identity, with no detail request — so a run's
// request volume stays under the Reed API's per-hour quota. Detail fetches run concurrently; a
// failed detail is dropped; only a total search (listing) failure is a board-level error.
func (r reed) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	if r.apiKey == "" {
		return nil, errors.New("reed: missing API key (set REED_API_KEY)")
	}
	ids, err := r.searchIDs(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(ids, reedDetailWorkers, func(id int64) (Job, bool) {
		extID := strconv.FormatInt(id, 10)
		if seen(extID) {
			// Already ingested: refresh liveness only, no detail request. Carry just the
			// identity — a content-less re-upsert would wipe the description/facets hydrated
			// when this job was new (an empty description re-derives to empty facets).
			return Job{ExternalID: extID, SeenRefresh: true}, true
		}
		return r.detail(ctx, id)
	}), nil
}

// searchIDs pages every curated keyword and returns the union of unique job ids, deduped so a
// posting matched by several keywords is fetched once. A single keyword's failure is tolerated
// (skipped, the union of the rest is still worth ingesting — like fetchDetailsStream dropping
// one bad detail); but if EVERY keyword fails it returns an error, so a total outage (e.g. a
// bad key 401ing every search) surfaces instead of masquerading as "Reed has no jobs" — which
// would otherwise persist an empty crawl.
func (r reed) searchIDs(ctx context.Context) ([]int64, error) {
	headers := r.authHeaders()
	seen := map[int64]struct{}{}
	var ids []int64
	var firstErr error
	succeeded := 0
	for _, kw := range reedKeywords {
		if err := r.searchKeyword(ctx, kw, headers, seen, &ids); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		succeeded++
	}
	if succeeded == 0 {
		return nil, fmt.Errorf("reed: all %d keyword searches failed: %w", len(reedKeywords), firstErr)
	}
	return ids, nil
}

// searchKeyword pages one keyword, appending each new (deduped via seen) job id to ids.
func (r reed) searchKeyword(ctx context.Context, kw string, headers map[string]string, seen map[int64]struct{}, ids *[]int64) error {
	for skip := 0; skip < reedMaxSkip; skip += reedPageSize {
		u := fmt.Sprintf("%s?keywords=%s&resultsToTake=%d&resultsToSkip=%d",
			reedSearchURL, url.QueryEscape(kw), reedPageSize, skip)
		var resp reedSearchResponse
		if err := r.http.GetJSONWithHeaders(ctx, u, headers, &resp); err != nil {
			return fmt.Errorf("reed: search %q skip %d: %w", kw, skip, err)
		}
		if len(resp.Results) == 0 {
			break
		}
		for _, it := range resp.Results {
			if it.JobID == 0 {
				continue
			}
			if _, dup := seen[it.JobID]; dup {
				continue
			}
			seen[it.JobID] = struct{}{}
			*ids = append(*ids, it.JobID)
		}
		if skip+reedPageSize >= resp.TotalResults {
			break
		}
	}
	return nil
}

// detail fetches one job's full record. A failed detail returns ok=false so the streaming
// fan-out drops it without aborting the crawl.
func (r reed) detail(ctx context.Context, id int64) (Job, bool) {
	var d reedJob
	u := reedJobURL + strconv.FormatInt(id, 10)
	if err := r.http.GetJSONWithHeaders(ctx, u, r.authHeaders(), &d); err != nil {
		return Job{}, false
	}
	return d.toJob()
}
