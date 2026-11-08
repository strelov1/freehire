// Package sources: firefish adapts Firefish (firefishsoftware.com), the UK recruitment CRM
// behind employers such as Arnold Clark. A tenant's career site lives on its own domain
// (careers.arnoldclark.com) or as a subdomain of the vendor's (arnoldclark.firefishsoftware.com,
// which 301s to the tenant's own branding) — the board is that whole host, exactly the address
// the tenant's postings carry, and one the tenant cannot be told apart from on the shared
// hosting alone.
//
// Every Firefish site publishes an RSS listing at /rss/adverts/latest.aspx — one unpaginated
// document carrying every open vacancy with its full HTML body and structured facets
// (reference number, job type, location area, posted/closing dates, structured pay bounds with
// currency and rate). One GET per board, no detail request; that completeness is what earns
// fullBoardListing. Verified live 2026-11-07 on careers.arnoldclark.com (342 vacancies, mixed
// years of history) and on the vendor's own jobs.firefishsoftware.com.
//
// An unknown tenant label does not resolve (NXDOMAIN), and a site with no openings answers a
// well-formed channel with no items.
package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// firefish adapts one tenant's career site (see the package comment above).
type firefish struct {
	http XMLGetter
}

// NewFirefish builds the Firefish adapter over the given HTTP client.
func NewFirefish(c XMLGetter) Source { return firefish{http: c} }

func (firefish) Provider() string { return "firefish" }

// fullBoardListing: the feed is one unpaginated document carrying every open vacancy.
func (firefish) fullBoardListing() {}

// firefishBoard is the shape of a careers host (careers.arnoldclark.com,
// jobs.firefishsoftware.com, or any host the Recognize table lets through).
var firefishBoard = regexp.MustCompile(`^[a-z0-9.-]+\.[a-z]{2,}$`)

// firefishFeed is the RSS document a tenant publishes: a single channel of items. XMLName is
// checked so an answer that is not the feed is told apart from a site with no openings, which
// the platform serves as a well-formed, item-less channel.
type firefishFeed struct {
	XMLName xml.Name       `xml:"rss"`
	Items   []firefishItem `xml:"channel>item"`
}

// firefishItem is one open vacancy. The description is HTML inside CDATA, so it arrives
// unescaped; the facets are the vendor's ffAdvert namespace, read as direct children.
type firefishItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Reference   string `xml:"http://www.firefishsoftware.com/ ReferenceNumber"`
	JobType     string `xml:"http://www.firefishsoftware.com/ JobType"`
	Location    string `xml:"http://www.firefishsoftware.com/ LocationArea"`
	PostedDate  string `xml:"http://www.firefishsoftware.com/ PostedDate"`
	ClosingDate string `xml:"http://www.firefishsoftware.com/ ClosingDate"`
	Country     string `xml:"http://www.firefishsoftware.com/ Country"`
	MinPayment  string `xml:"http://www.firefishsoftware.com/ MinimumPayment"`
	MaxPayment  string `xml:"http://www.firefishsoftware.com/ MaximumPayment"`
	PaymentRate string `xml:"http://www.firefishsoftware.com/ PaymentRate"`
	Currency    string `xml:"http://www.firefishsoftware.com/ Currency"`
}

// firefishFeedURL is the one RSS document a Firefish career site publishes.
func firefishFeedURL(board string) string {
	return "https://" + board + "/rss/adverts/latest.aspx"
}

// Fetch reads the site's RSS listing and maps every vacancy.
func (s firefish) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board, items, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(items))
	for _, it := range items {
		if job, ok := it.toJob(e.Company, board); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is Fetch that marks every vacancy seen — the feed carries the whole posting, so a
// re-upsert of a seen vacancy costs nothing and the SeenRefresh shortcut is not needed.
func (s firefish) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.Fetch(ctx, e)
}

// list reads the tenant's RSS document, dropping any item without the reference number a close
// needs or the link a visitor would open.
func (s firefish) list(ctx context.Context, rawBoard string) (string, []firefishItem, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !firefishBoard.MatchString(board) {
		return "", nil, fmt.Errorf("firefish: board %q is not a careers host", rawBoard)
	}
	var feed firefishFeed
	if err := s.http.GetXML(ctx, firefishFeedURL(board), &feed); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return "", nil, fmt.Errorf("firefish: board %q: %w", board, ErrBoardGone)
		}
		return "", nil, fmt.Errorf("firefish: board %s feed: %w", board, err)
	}
	var out []firefishItem
	seen := map[string]bool{}
	for _, it := range feed.Items {
		it.Reference = strings.TrimSpace(it.Reference)
		if it.Reference == "" || strings.TrimSpace(it.Link) == "" || seen[it.Reference] {
			continue
		}
		seen[it.Reference] = true
		out = append(out, it)
	}
	return board, out, nil
}

// toJob maps a feed item to a Job. The identity is the vacancy's own reference number, which
// the vendor states globally per site; the URL is the item's link, the tenant's canonical
// posting address. The structured pay fields carry both bounds, a currency and a rate only
// when the employer stated them — a 0.00 bound means not stated, and a salary without its
// period would read as annual downstream.
func (it firefishItem) toJob(company, board string) (Job, bool) {
	title := strings.TrimSpace(it.Title)
	if title == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:  it.Reference,
		URL:         strings.TrimSpace(it.Link),
		Title:       title,
		Company:     company,
		Location:    strings.TrimSpace(it.Location),
		Description: sanitizeHTML(it.Description),
		PostedAt:    parsePubDate(strings.TrimSpace(it.PostedDate)),
	}
	if code := location.NormalizeCountry(strings.TrimSpace(it.Country)); code != "" {
		job.Countries = []string{code}
	}
	if et := firefishEmploymentType(it.JobType); et != "" {
		job.EmploymentType = et
	}
	if d := firefishDeadline(it.ClosingDate); d != nil {
		job.ApplicationDeadline = d
	}
	if period := firefishSalaryPeriod(it.PaymentRate); period != "" {
		if min, ok := firefishAmount(it.MinPayment); ok {
			job.SalaryMin = &min
			if max, ok := firefishAmount(it.MaxPayment); ok && max >= min {
				job.SalaryMax = &max
			}
			job.SalaryCurrency = strings.TrimSpace(it.Currency)
			job.SalaryPeriod = period
		}
	}
	return job, true
}

// firefishDeadline parses the feed's ClosingDate (an RFC1123 GMT timestamp, as every RSS
// pubDate). Unlike a posted_at a deadline may legitimately sit in the future, so it bypasses
// NotFuture: a stale one past today closes the vacancy on the write.
func firefishDeadline(s string) *time.Time {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		t = t.UTC()
		return &t
	}
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		t = t.UTC()
		return &t
	}
	return nil
}

// firefishEmploymentType maps the feed's JobType onto the employment-type vocabulary. The
// measured corpus spells Permanent and Contract; anything else leaves the field to the
// pipeline's dictionaries rather than guessing.
func firefishEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "permanent", "full time", "full-time":
		return "full_time"
	case "part time", "part-time":
		return "part_time"
	case "contract", "temporary", "fixed term", "fixed-term":
		return "contract"
	case "internship", "apprenticeship", "graduate":
		return "internship"
	default:
		return ""
	}
}

// firefishSalaryPeriod maps the feed's PaymentRate onto the period vocabulary.
func firefishSalaryPeriod(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "per year", "per annum":
		return "year"
	case "per month":
		return "month"
	case "per day":
		return "day"
	case "per hour":
		return "hour"
	default:
		return ""
	}
}

// firefishAmount reads a positive pay figure; the platform writes 0.00 for "not stated".
func firefishAmount(s string) (int, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return int(f), true
}
