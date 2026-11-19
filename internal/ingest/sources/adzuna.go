package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// adzuna adapts the Adzuna Job Search API v1 (api.adzuna.com), an independent aggregator across
// many countries. KEYED: every request needs an app_id/app_key pair, so the adapter is registered
// only when both ADZUNA_APP_ID and ADZUNA_APP_KEY are set (see All).
//
// There is no "list everything" call: every request selects a country and, within it, a category
// (e.g. "it-jobs"). The country goes in CompanyEntry.Region (the /jobs/{country}/search/{page}
// path segment), the category in CompanyEntry.Board. Category is a hard filter, unlike whatjobs'
// relevance-ranked keyword search, so no corroboration logic is needed.
type adzuna struct {
	http   JSONGetter
	appID  string
	appKey string
}

const (
	adzunaBaseURL = "https://api.adzuna.com/v1/api/jobs"
	// adzunaPageSize is this API's page-size ceiling: results_per_page=100 answers HTTP 400.
	adzunaPageSize = 50
	// adzunaSortOrder reads the NEWEST postings first. Without it Adzuna answers in a stable
	// relevance order, so a bounded crawl re-reads one fixed slice forever.
	adzunaSortOrder = "date"
	// adzunaMaxDaysOld bounds a request to recently published postings. Not redundant with
	// adzunaSortOrder: Fetch ends on an empty page, which never happens inside adzunaMaxPages
	// against a feed tens of thousands deep, so a quiet board would spend its page budget on
	// postings the seen-set discards, from the same daily ceiling a busy board needs. Two days
	// rather than one so a missed run leaves no gap.
	adzunaMaxDaysOld = 2
	// adzunaMaxPages bounds one board's crawl. It is one leg of a REQUEST BUDGET set by Adzuna's
	// free-API terms (250 requests/day, 2500/month):
	//
	//	boards (4) x adzunaMaxPages (15) x runs/day (4, see the systemd timer) = 240/day
	//
	// Raising one leg without lowering another breaks the terms.
	// TestAdzunaPageBudgetStaysWithinTheDailyCeiling holds the arithmetic; the timer cadence lives
	// outside this repo, in freehire-ops' provision/host2/systemd/freehire-ingest@adzuna.timer.
	adzunaMaxPages = 15
	// adzunaSweepGrace widens the unseen sweep for the same reason whatjobs' does: redirect_url
	// routes through Adzuna's own domain rather than the employer's site, so a posting's liveness
	// cannot be probed directly, and the crawl reaches only a bounded slice of a much deeper feed.
	adzunaSweepGrace = 14 * 24 * time.Hour
)

// NewAdzuna builds the Adzuna adapter over the given HTTP client and credential pair.
func NewAdzuna(c JSONGetter, appID, appKey string) Source {
	return adzuna{http: c, appID: appID, appKey: appKey}
}

func (adzuna) Provider() string { return "adzuna" }

// adzuna aggregates postings from many employers, so it stays in the source facet.
func (adzuna) Aggregator() {}

func (adzuna) SweepGrace() time.Duration { return adzunaSweepGrace }

// Unseen is not evidence a posting closed: a crawl reaches only a small slice of a much deeper
// feed, most of which is older than 21 days. cmd/adzuna-liveness reads Adzuna's own /details page
// and closes on a 404/410.
func (adzuna) LivenessProbed() {}

// adzunaID reads a posting's id, which the API emits as a JSON string for most postings but as a
// bare number for some. A page hitting it must decode rather than fail whole, mirroring
// habrJobLocations/habrAddress's shape-tolerant UnmarshalJSON.
type adzunaID string

func (id *adzunaID) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*id = adzunaID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*id = adzunaID(n.String())
	return nil
}

// adzunaPosting is one search result. salary_min/salary_max/salary_is_predicted and category are
// deliberately unread: none has a home in the Job shape.
type adzunaPosting struct {
	ID          adzunaID `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Created     string   `json:"created"`
	RedirectURL string   `json:"redirect_url"`
	Company     struct {
		DisplayName string `json:"display_name"`
	} `json:"company"`
	Location struct {
		DisplayName string `json:"display_name"`
	} `json:"location"`
}

// Fetch reads one country+category slice. Per the house pagination rule, a failure on the first
// page is a board-level error; a failure on a later page ends the walk with what was already
// gathered. Pagination stops on an empty page or at adzunaMaxPages.
func (a adzuna) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return a.fetch(ctx, e, adzunaMaxDaysOld)
}

// FetchRecent reads the same slice as Fetch but bounds it to the run's window. The window goes
// out as max_days_old, which Adzuna's search API documents as "the age of the oldest advertisement
// in days that will be returned" (https://developer.adzuna.com/docs/search). A window under one
// day still asks for one, the finest grain the parameter has.
func (a adzuna) FetchRecent(ctx context.Context, e CompanyEntry, window time.Duration) ([]Job, error) {
	return a.fetch(ctx, e, max(1, int(window/(24*time.Hour))))
}

func (a adzuna) fetch(ctx context.Context, e CompanyEntry, maxDaysOld int) ([]Job, error) {
	country := strings.TrimSpace(e.Region)
	if country == "" {
		return nil, fmt.Errorf("adzuna: company %q has a blank country (Region)", e.Company)
	}
	category := strings.TrimSpace(e.Board)
	if category == "" {
		return nil, fmt.Errorf("adzuna: company %q has a blank category board", e.Company)
	}

	var jobs []Job
	for page := 1; page <= adzunaMaxPages; page++ {
		var resp struct {
			Results []adzunaPosting `json:"results"`
		}
		if err := a.http.GetJSON(ctx, adzunaPageURL(country, category, page, maxDaysOld, a.appID, a.appKey), &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("adzuna: country %q category %q page %d: %w", country, category, page, err)
			}
			log.Printf("adzuna: country %q category %q page %d failed, stopping with %d jobs gathered: %v",
				country, category, page, len(jobs), err)
			return jobs, nil
		}
		if len(resp.Results) == 0 {
			break
		}
		for _, p := range resp.Results {
			jobs = append(jobs, p.toJob())
		}
	}
	return jobs, nil
}

func adzunaPageURL(country, category string, page, maxDaysOld int, appID, appKey string) string {
	q := url.Values{
		"app_id":           {appID},
		"app_key":          {appKey},
		"results_per_page": {fmt.Sprint(adzunaPageSize)},
		"category":         {category},
		"content-type":     {"application/json"},
		"sort_by":          {adzunaSortOrder},
		"max_days_old":     {strconv.Itoa(maxDaysOld)},
	}
	return fmt.Sprintf("%s/%s/search/%d?%s", adzunaBaseURL, country, page, q.Encode())
}

func (p adzunaPosting) toJob() Job {
	return Job{
		ExternalID:  string(p.ID),
		URL:         p.RedirectURL,
		Title:       p.Title,
		Company:     p.Company.DisplayName,
		Location:    p.Location.DisplayName,
		Description: p.Description,
		PostedAt:    parseRFC3339(p.Created),
	}
}
