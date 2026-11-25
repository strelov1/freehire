package aggregators

import (
	"github.com/strelov1/freehire/internal/ingest/sources/kit"

	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// careerjet adapts the Careerjet Partner API v4 (search.api.careerjet.net), a multi-country job
// aggregator that pays publishers per click on its tracking links. KEYED: the API key is the HTTP
// Basic username with an empty password, so the adapter is registered only when CAREERJET_API_KEY
// and CAREERJET_USER_IP are set (see registry_special.go).
//
// There is no "list everything" call: every request is a keyword search in one locale. The locale
// (en_GB, en_US, ...) goes in CompanyEntry.Region, the keywords in CompanyEntry.Board, and the
// location stays empty (country-wide). The keywords are a relevance-ranked search, but sorting by
// date makes a bounded crawl read what was published last.
type careerjet struct {
	http   kit.HeaderJSONGetter
	apiKey string
	userIP string
}

const (
	careerjetBaseURL = "https://search.api.careerjet.net/v4/query"
	// careerjetUserAgent is the user_agent the API requires of every request. Fixed and honest:
	// we crawl server-side, there is no end user's browser to forward.
	//
	// shortcut: crawled server-side with our own IP/UA, Careerjet may not pay for these clicks;
	// upgrade to a live user-triggered search if they flag it.
	careerjetUserAgent = "freehire-ingest/1.0 (+https://freehire.me)"
	// careerjetPageSize is the API's page_size ceiling.
	careerjetPageSize = 100
	// careerjetMaxPages is the API's page ceiling (page 1..10): a search is never readable past
	// 1000 hits however many it reports, so the page budget is the API's own depth.
	// Careerjet publishes no request quota; TestCareerjetPageBudget holds the arithmetic.
	careerjetMaxPages = 10
	// careerjetSweepGrace widens the unseen sweep as adzuna's does: every url routes through
	// Careerjet's tracking domain, so liveness cannot be probed, and a crawl reaches only the
	// newest 1000 hits of a deeper feed.
	careerjetSweepGrace = 14 * 24 * time.Hour
)

// NewCareerjet builds the Careerjet adapter over the given HTTP client, API key and the server's
// public egress IP (sent as user_ip).
func NewCareerjet(c kit.HeaderJSONGetter, apiKey, userIP string) kit.Source {
	return careerjet{http: c, apiKey: apiKey, userIP: userIP}
}

func (careerjet) Provider() string { return "careerjet" }

// careerjet aggregates postings from many employers, so it stays in the source facet.
func (careerjet) Aggregator() {}

func (careerjet) SweepGrace() time.Duration { return careerjetSweepGrace }

// careerjetPosting is one search hit. The API has no job id: see externalID.
type careerjetPosting struct {
	Title        string   `json:"title"`
	Company      string   `json:"company"`
	Date         string   `json:"date"`
	Description  string   `json:"description"`
	Locations    string   `json:"locations"`
	SalaryMin    *float64 `json:"salary_min"`
	SalaryMax    *float64 `json:"salary_max"`
	SalaryType   string   `json:"salary_type"`
	SalaryCurr   string   `json:"salary_currency_code"`
	TrackingLink string   `json:"url"`
}

type careerjetResponse struct {
	Type    string             `json:"type"`
	Message string             `json:"message"`
	Pages   int                `json:"pages"`
	Jobs    []careerjetPosting `json:"jobs"`
}

// Fetch reads one locale+keywords slice. A failure on the first page, or an answer that is not a
// job list, is a board-level error; a failure on a later page ends the walk with what was
// gathered. Pagination stops at the API's reported page count, on an empty page, or at
// careerjetMaxPages.
func (c careerjet) Fetch(ctx context.Context, e kit.CompanyEntry) ([]kit.Job, error) {
	locale, keywords := strings.TrimSpace(e.Region), strings.TrimSpace(e.Board)
	if locale == "" {
		return nil, fmt.Errorf("careerjet: company %q has a blank locale (Region)", e.Company)
	}
	if keywords == "" {
		return nil, fmt.Errorf("careerjet: company %q has blank keywords (Board)", e.Company)
	}

	var jobs []kit.Job
	for page := 1; page <= careerjetMaxPages; page++ {
		var resp careerjetResponse
		err := c.http.GetJSONWithHeaders(ctx, careerjetPageURL(locale, keywords, page, c.userIP), c.authHeaders(), &resp)
		// The API answers 200 with type LOCATIONS when it read the query as a place name and ran
		// no search. That is a config error, not an empty feed.
		if err == nil && resp.Type != "JOBS" {
			err = fmt.Errorf("answer type %q, not JOBS: %s", resp.Type, resp.Message)
		}
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("careerjet: locale %q keywords %q page %d: %w", locale, keywords, page, err)
			}
			log.Printf("careerjet: locale %q keywords %q page %d failed, stopping with %d jobs gathered: %v",
				locale, keywords, page, len(jobs), err)
			return jobs, nil
		}
		for _, p := range resp.Jobs {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if len(resp.Jobs) == 0 || page >= resp.Pages {
			break
		}
	}
	return jobs, nil
}

func (c careerjet) authHeaders() map[string]string {
	return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(c.apiKey+":"))}
}

func careerjetPageURL(locale, keywords string, page int, userIP string) string {
	q := url.Values{
		"locale_code": {locale},
		"keywords":    {keywords},
		"sort":        {"date"},
		"page":        {fmt.Sprint(page)},
		"page_size":   {fmt.Sprint(careerjetPageSize)},
		"user_ip":     {userIP},
		"user_agent":  {careerjetUserAgent},
	}
	return careerjetBaseURL + "?" + q.Encode()
}

func (p careerjetPosting) toJob() (kit.Job, bool) {
	if p.TrackingLink == "" || p.Title == "" {
		return kit.Job{}, false
	}
	lo, hi, curr, period := p.salary()
	return kit.Job{
		ExternalID:     p.externalID(),
		URL:            p.TrackingLink,
		Title:          p.Title,
		Company:        p.Company,
		Location:       p.Locations,
		Description:    p.Description,
		PostedAt:       kit.ParsePubDate(p.Date),
		SalaryMin:      lo,
		SalaryMax:      hi,
		SalaryCurrency: curr,
		SalaryPeriod:   period,
	}, true
}

// externalID hashes title, company and locations: the payload has no job id, and the tracking
// url is a per-click link whose stability across requests is not established, so it cannot key a
// posting. The date is left out on purpose: a re-dated repost is the same posting.
func (p careerjetPosting) externalID() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{p.Title, p.Company, p.Locations}, "\x00")))
	return hex.EncodeToString(sum[:])[:24]
}

// salary maps the structured salary onto freehire's fields. Careerjet's salary_type is
// Y/M/W/D/H; weekly has no freehire period, so a weekly figure is dropped whole rather than
// restated under another period (enrichment reads it from the text).
func (p careerjetPosting) salary() (lo, hi *int, currency, period string) {
	period = map[string]string{"Y": "year", "M": "month", "D": "day", "H": "hour"}[p.SalaryType]
	if p.SalaryMin != nil {
		lo = kit.RoundSalaryPart(*p.SalaryMin)
	}
	if p.SalaryMax != nil {
		hi = kit.RoundSalaryPart(*p.SalaryMax)
	}
	if period == "" || (lo == nil && hi == nil) {
		return nil, nil, "", ""
	}
	return lo, hi, p.SalaryCurr, period
}
