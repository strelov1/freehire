package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// adzuna adapts the Adzuna Job Search API v1 (api.adzuna.com), an independent job aggregator
// operating its own employer/recruiter feeds across many countries. It is the one KEYED source
// besides usajobs/reed/whatjobs: every request needs an app_id/app_key pair, so the adapter is
// registered only when both ADZUNA_APP_ID and ADZUNA_APP_KEY are set (see All).
//
// Unlike the boardless aggregators (getmanfred, echojobs), Adzuna has no "list everything" call —
// every request selects a country and, within it, a category (e.g. "it-jobs"), the same shape
// whatjobs' keyword board and trudvsem's OKATO-region board already use for a source whose real
// catalogue is far deeper than any one crawl can walk. The country goes in CompanyEntry.Region
// (it is literally which regional API path segment to hit — /jobs/{country}/search/{page} — the
// same job Region already does for Lever's EU host), and the category in CompanyEntry.Board.
// Category, not free-text keyword: a live check confirmed it is a hard filter (every result on
// page 50 of "it-jobs" still tagged it-jobs), unlike whatjobs' relevance-ranked keyword search,
// which pads a thin result set with unrelated inventory past the first page or two — so this
// adapter does not need whatjobs' corroboration-and-relevance-collapse logic.
type adzuna struct {
	http   JSONGetter
	appID  string
	appKey string
}

const (
	adzunaBaseURL = "https://api.adzuna.com/v1/api/jobs"
	// adzunaPageSize is this API's page-size ceiling, confirmed live rather than assumed:
	// results_per_page=100 answers HTTP 400.
	adzunaPageSize = 50
	// adzunaSortOrder makes the crawl read the NEWEST postings first. Without it Adzuna
	// answers in relevance order, which is stable between runs, so a bounded crawl re-reads
	// one fixed slice for ever: measured live on 2026-09-06, the first result of the request
	// this adapter used to send was published 2026-02-10, seven months earlier. The budget was
	// buying 23.5k posting-slots a day and writing ~6.3k new postings — a 27% yield. Ordering
	// by date is what aims the budget at postings the catalogue does not already hold.
	adzunaSortOrder = "date"
	// adzunaMaxDaysOld bounds a request to recently published postings. It is NOT redundant
	// with adzunaSortOrder, and that is the whole reason it exists: Fetch's loop ends when a
	// page comes back empty, and against a feed tens of thousands deep that never happens
	// inside adzunaMaxPages. A quiet board (au publishes ~176 postings between runs) would
	// otherwise spend its full page budget on postings the pipeline's seen-set discards, and
	// those requests are drawn from the same daily ceiling a busy board needs. Two days rather
	// than one so a missed run does not open a gap the next run cannot close.
	adzunaMaxDaysOld = 2
	// adzunaMaxPages bounds one board's crawl. Unlike a plain "how deep is sensible" figure,
	// this is one leg of a REQUEST BUDGET the platform's terms bound: Adzuna's free API states
	// 250 requests/day and 2500/month, and the crawl spends
	//
	//	boards (4) x adzunaMaxPages (15) x runs/day (4, see the systemd timer) = 240/day
	//
	// so raising any one of the three without lowering another puts the catalogue's largest
	// single source outside the terms it is served under. TestAdzunaPageBudgetStaysWithinThe-
	// DailyCeiling holds the arithmetic; the timer's cadence is the leg that lives outside
	// this repository's build, in deploy/systemd/freehire-ingest@adzuna.timer.
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
func (adzuna) aggregator() {}

func (adzuna) sweepGrace() time.Duration { return adzunaSweepGrace }

// adzunaID reads a posting's id, which the API emits as a JSON string for most postings but as a
// bare JSON number for some (observed live 2026-08-08, gb/it-jobs page 7) — an upstream
// inconsistency, not a documented alternate shape. A page hitting it must decode instead of
// failing the whole page, mirroring habrJobLocations/habrAddress's shape-tolerant UnmarshalJSON.
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

// adzunaPosting is one search result. Several fields the API sends are deliberately unread —
// salary_min/salary_max/salary_is_predicted and category — because none has a home in the Job
// shape today, mirroring echojobs' treatment of its own unread vendor fields.
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
		if err := a.http.GetJSON(ctx, adzunaPageURL(country, category, page, a.appID, a.appKey), &resp); err != nil {
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

func adzunaPageURL(country, category string, page int, appID, appKey string) string {
	q := url.Values{
		"app_id":           {appID},
		"app_key":          {appKey},
		"results_per_page": {fmt.Sprint(adzunaPageSize)},
		"category":         {category},
		"content-type":     {"application/json"},
		"sort_by":          {adzunaSortOrder},
		"max_days_old":     {fmt.Sprint(adzunaMaxDaysOld)},
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
