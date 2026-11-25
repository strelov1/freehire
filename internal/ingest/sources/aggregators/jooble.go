package aggregators

import (
	"github.com/strelov1/freehire/internal/ingest/sources/kit"

	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// jooble adapts the Jooble REST API (https://jooble.org/api/about), an independent aggregator.
// KEYED: the key is a PATH segment of the request URL, so it is registered only when
// JOOBLE_API_KEY is set (see All) and must never reach a log line or an error string
// (redactKey). Jooble pays publishers per click on a posting's `link`, a jooble.org tracking
// URL, which is therefore the apply link.
//
// There is no "list everything" call: every request is a keyword search within a location. The
// keywords go in CompanyEntry.Board, the location in CompanyEntry.Region. Jooble runs one site
// per country and a key works only on the site it was issued on (a jooble.org key sees US
// listings only), so the country is the KEY plus the location string, not a request field; see
// providers/jooble.md.
type jooble struct {
	http kit.JSONPoster
	key  string
	now  func() time.Time
}

const (
	joobleBaseURL = "https://jooble.org/api/"
	// joobleResultOnPage is the page size asked for. Jooble documents the field but no ceiling,
	// so this is a guess, harmless if wrong: the walk stops on totalCount or an empty page
	// whatever size the server actually returns.
	joobleResultOnPage = 50
	// joobleMaxDaysOld bounds a plain Fetch to recently created postings, for the reason
	// adzunaMaxDaysOld does: a quiet board must not spend its page budget on postings the
	// seen-set discards. Two days so a missed run leaves no gap.
	joobleMaxDaysOld = 2
	// joobleMaxPages bounds one board's crawl. It is one leg of a REQUEST BUDGET set by
	// Jooble's free plan (100 requests/day per key, reset at midnight UTC):
	//
	//	boards (4) x joobleMaxPages (5) x runs/day (4, see the systemd timer) = 80/day
	//
	// Raising one leg without lowering another breaks the limit; a 429 answers the rest of the
	// day. TestJooblePageBudgetStaysWithinTheDailyCeiling holds the arithmetic; the timer
	// cadence lives outside this repo, in freehire-ops' provision/host2/systemd.
	joobleMaxPages = 5
	// joobleSweepGrace widens the unseen sweep for the reason adzunaSweepGrace does: `link`
	// routes through Jooble's own domain, so a posting's liveness cannot be probed directly, and
	// the crawl reaches only a bounded slice of a much deeper feed.
	joobleSweepGrace = 14 * 24 * time.Hour
)

// NewJooble builds the Jooble adapter over the given HTTP client and API key.
func NewJooble(c kit.JSONPoster, key string) kit.Source {
	return jooble{http: c, key: key, now: time.Now}
}

func (jooble) Provider() string { return "jooble" }

// jooble aggregates postings from many employers, so it stays in the source facet.
func (jooble) Aggregator() {}

func (jooble) SweepGrace() time.Duration { return joobleSweepGrace }

// joobleRequest is the documented JSON body. DateCreatedFrom is NOT in the public docs page
// (which lists keywords, location, radius, salary, page, ResultOnPage, SearchMode,
// companysearch); it is sent on the strength of the task brief, and an unknown field is ignored
// by a JSON API, so the worst case is a crawl that is not incremental.
type joobleRequest struct {
	Keywords        string `json:"keywords"`
	Location        string `json:"location"`
	Page            int    `json:"page"`
	ResultOnPage    int    `json:"ResultOnPage"`
	DateCreatedFrom string `json:"datecreatedfrom"`
}

// joobleResponse is one result page. Unread: `salary` (free text such as "17,600 UAH", and no
// existing helper parses that), `source` and `type` (free text, not freehire's vocabularies).
type joobleResponse struct {
	TotalCount int         `json:"totalCount"`
	Jobs       []joobleJob `json:"jobs"`
}

type joobleJob struct {
	ID       looseID `json:"id"`
	Title    string  `json:"title"`
	Location string  `json:"location"`
	Snippet  string  `json:"snippet"`
	Link     string  `json:"link"`
	Company  string  `json:"company"`
	Updated  string  `json:"updated"`
}

// redactedError is err with the API key masked in its text. The shared HTTP client puts the
// request URL, key included, in every error it returns. The chain stays reachable through Unwrap.
type redactedError struct {
	msg string
	err error
}

func (e redactedError) Error() string { return e.msg }
func (e redactedError) Unwrap() error { return e.err }

func (j jooble) redactKey(err error) error {
	return redactedError{msg: strings.ReplaceAll(err.Error(), j.key, "***"), err: err}
}

// Fetch reads one keyword+location slice. Per the house pagination rule, a failure on the first
// page is a board-level error; a failure on a later page ends the walk with what was already
// gathered. Pagination stops once totalCount postings are read, on an empty page, or at
// joobleMaxPages.
func (j jooble) Fetch(ctx context.Context, e kit.CompanyEntry) ([]kit.Job, error) {
	return j.fetch(ctx, e, joobleMaxDaysOld)
}

// FetchRecent reads the same slice as Fetch but bounds it to the run's window, as
// datecreatedfrom. A window under one day still asks for one, the finest grain the field has.
func (j jooble) FetchRecent(ctx context.Context, e kit.CompanyEntry, window time.Duration) ([]kit.Job, error) {
	return j.fetch(ctx, e, max(1, int(window/(24*time.Hour))))
}

func (j jooble) fetch(ctx context.Context, e kit.CompanyEntry, maxDaysOld int) ([]kit.Job, error) {
	location := strings.TrimSpace(e.Region)
	if location == "" {
		return nil, fmt.Errorf("jooble: company %q has a blank location (Region)", e.Company)
	}
	keywords := strings.TrimSpace(e.Board)
	if keywords == "" {
		return nil, fmt.Errorf("jooble: company %q has a blank keywords board", e.Company)
	}

	body := joobleRequest{
		Keywords:        keywords,
		Location:        location,
		ResultOnPage:    joobleResultOnPage,
		DateCreatedFrom: j.now().UTC().AddDate(0, 0, -maxDaysOld).Format("2006-01-02"),
	}
	var jobs []kit.Job
	for page := 1; page <= joobleMaxPages; page++ {
		body.Page = page
		var resp joobleResponse
		if err := j.http.PostJSON(ctx, joobleBaseURL+j.key, body, &resp); err != nil {
			err = j.redactKey(err)
			if page == 1 {
				return nil, fmt.Errorf("jooble: keywords %q location %q page %d: %w", keywords, location, page, err)
			}
			log.Printf("jooble: keywords %q location %q page %d failed, stopping with %d jobs gathered: %v",
				keywords, location, page, len(jobs), err)
			return jobs, nil
		}
		for _, p := range resp.Jobs {
			jobs = append(jobs, p.toJob())
		}
		if len(resp.Jobs) == 0 || len(jobs) >= resp.TotalCount {
			break
		}
	}
	return jobs, nil
}

func (p joobleJob) toJob() kit.Job {
	return kit.Job{
		ExternalID:  string(p.ID),
		URL:         p.Link,
		Title:       p.Title,
		Company:     p.Company,
		Location:    p.Location,
		Description: p.Snippet,
		// `updated` carries no zone ("2023-09-15T12:55:35.3870000"), read as UTC.
		PostedAt: kit.ParseLayout("2006-01-02T15:04:05", p.Updated),
	}
}
