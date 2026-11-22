package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// linkedIn reads a bounded recent search, never an authoritative live catalogue.
// Board is the keyword query; Region is the location passed to LinkedIn search.
type linkedIn struct {
	http    HTMLResolvedGetter
	pace    waiter
	backoff time.Duration
	// configErr is a bad LINKEDIN_REQUEST_INTERVAL; Fetch reports it instead of crawling.
	configErr error
}

var errLinkedInEmptyFirstListing = errors.New("linkedin: first listing has no recognizable postings")

const linkedInSearchURL = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"

// NewLinkedIn shares one request budget across all boards and their detail requests.
func NewLinkedIn(c HTMLResolvedGetter) Source {
	interval, err := linkedInRequestInterval(os.Getenv(linkedInIntervalEnv))
	return linkedIn{http: c, pace: rate.NewLimiter(rate.Every(interval), 1), backoff: linkedInThrottleBackoff, configErr: err}
}

// LINKEDIN_REQUEST_INTERVAL sets the shared limiter's spacing (default 2s). Requests leave
// through the Scrapling tier's rotating proxy, a new exit IP each, so LinkedIn's per-IP limit
// does not see the process's rate; after the 429 retry, 158 searches at 2s met no refusal
// (2026-11-17). The bounds keep a typo from becoming a flood or a stall; a bad value fails
// the crawl rather than silently keeping the default.
const (
	linkedInIntervalEnv     = "LINKEDIN_REQUEST_INTERVAL"
	linkedInDefaultInterval = 2 * time.Second
	linkedInMinInterval     = 100 * time.Millisecond
	linkedInMaxInterval     = 10 * time.Second
)

func linkedInRequestInterval(raw string) (time.Duration, error) {
	if raw = strings.TrimSpace(raw); raw == "" {
		return linkedInDefaultInterval, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < linkedInMinInterval || d > linkedInMaxInterval {
		return linkedInDefaultInterval, fmt.Errorf("linkedin: %s=%q must be a duration between %s and %s", linkedInIntervalEnv, raw, linkedInMinInterval, linkedInMaxInterval)
	}
	return d, nil
}

// A 429 through the rotating proxy usually names one throttled exit, not the whole crawl:
// in the 2026-11-17 run, refusals clustered within seconds while the next requests were
// served, and stopping on the first one ended searches after 7-64 of ~100 postings. Retry it
// a bounded number of times after a growing pause; a persistent 429 still stops the board.
// 403 and 999 remain immediate refusals.
const (
	linkedInThrottleRetries = 2
	linkedInThrottleBackoff = 10 * time.Second
)

func (linkedIn) Provider() string { return "linkedin" }
func (linkedIn) Aggregator()      {}
func (linkedIn) LivenessProbed()  {}
func (linkedIn) ProbeClosing()    {}

// LinkedInPostingURL is the canonical public URL of a numeric posting id, the one spelling
// every adapter path and the pipeline's identity rule share.
func LinkedInPostingURL(id string) string { return "https://www.linkedin.com/jobs/view/" + id + "/" }

var linkedInID = regexp.MustCompile(`(?:^|-)\d+$`)

// isLinkedInURL reports an https LinkedIn host on the default port, refusing a lookalike host
// such as linkedin.com.example.test.
func isLinkedInURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.Port() == "" && (host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"))
}

// LinkedInPostingID returns the native numeric ID of a public LinkedIn job URL.
// Non-LinkedIn hosts and paths are rejected before a liveness request is issued.
func LinkedInPostingID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !isLinkedInURL(u) {
		return ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasPrefix(path, "/jobs/view/") {
		return ""
	}
	slug := strings.TrimPrefix(path, "/jobs/view/")
	if strings.Contains(slug, "/") {
		return ""
	}
	return strings.TrimPrefix(linkedInID.FindString(slug), "-")
}

// LinkedInPosting reads only a complete posting with a verified identity and an explicit
// primary liveness signal. Removed describes the page evidence; ingest does not emit it
// as a removal event, because cmd/liveness must confirm closure twice across runs.
func LinkedInPosting(root *html.Node, finalURL, id string) (Job, error) {
	u, err := url.Parse(finalURL)
	guest := err == nil && u.Scheme == "https" && u.Host == "www.linkedin.com" && u.Path == "/jobs-guest/jobs/api/jobPosting/"+id
	if id == "" || (!guest && LinkedInPostingID(finalURL) != id) {
		return Job{}, fmt.Errorf("linkedin: posting identity mismatch")
	}
	if canonical := metaProperty(root, "og:url"); canonical != "" && LinkedInPostingID(canonical) != id {
		return Job{}, fmt.Errorf("linkedin: canonical identity mismatch")
	}
	card := firstByClass(root, "top-card-layout")
	if card == nil {
		return Job{}, fmt.Errorf("linkedin: missing primary posting card")
	}
	text := func(class string) string {
		n := firstByClass(card, class)
		if n == nil {
			return ""
		}
		return strings.Join(strings.Fields(textContent(n)), " ")
	}
	j := Job{ExternalID: id, URL: LinkedInPostingURL(id), Title: text("top-card-layout__title"), Company: text("topcard__org-name-link")}
	if org := firstByClass(card, "topcard__org-name-link"); org != nil {
		if u, err := url.Parse(Attr(org, "href")); err == nil && u.Scheme == "https" && linkedInHost(u.Hostname()) && strings.HasPrefix(u.Path, "/company/") && u.User == nil && u.Port() == "" {
			u.RawQuery, u.Fragment = "", ""
			j.CompanyProfileURL = u.String()
		}
	}
	walk(card, func(n *html.Node) bool {
		if j.Location == "" && HasClass(n, "topcard__flavor") && HasClass(n, "topcard__flavor--bullet") {
			j.Location = strings.Join(strings.Fields(textContent(n)), " ")
		}
		return true
	})
	if body := firstByClass(root, "show-more-less-html__markup"); body != nil {
		j.Description = sanitizeHTML(innerHTML(body))
	}
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, fmt.Errorf("linkedin: incomplete posting")
	}
	closed := text("closed-job__flavor--closed")
	method, target, apply := linkedInApplication(card)
	j.ApplyMethod, j.ExternalApplyURL = method, target
	if strings.EqualFold(closed, "No longer accepting applications") && !apply {
		j.Removed = true
		return j, nil
	}
	if closed == "" && apply {
		return j, nil
	}
	return Job{}, fmt.Errorf("linkedin: no unambiguous posting status")
}

func (s linkedIn) get(ctx context.Context, raw string) (*html.Node, string, error) {
	for attempt := 0; ; attempt++ {
		if err := s.pace.Wait(ctx); err != nil {
			return nil, "", err
		}
		root, final, err := s.http.GetHTMLResolved(ctx, raw)
		var status *StatusError
		throttled := errors.As(err, &status) && status.Code == 429
		if !throttled || attempt == linkedInThrottleRetries {
			return root, final, err
		}
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(time.Duration(attempt+1) * s.backoff):
		}
	}
}

// Fetch is the list-only fallback used when the pipeline supplies no seen-set: every listed
// posting's detail is read.
func (s linkedIn) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.fetch(ctx, e, nil)
}

// FetchNew reads the detail only of listed postings the catalogue does not hold. A seen
// posting is emitted as an identity-only SeenRefresh (no detail request), which the pipeline
// turns into a liveness touch. The pipeline's seen-set withholds rows whose body is older
// than BODY_REFRESH_DAYS, so a stored posting is re-read (and its application method
// re-checked) only that often instead of on every crawl.
func (s linkedIn) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.fetch(ctx, e, seen)
}

func (s linkedIn) fetch(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	if s.configErr != nil {
		return nil, s.configErr
	}
	if strings.TrimSpace(e.Board) == "" || strings.TrimSpace(e.Region) == "" {
		return nil, fmt.Errorf("linkedin: board keyword and region location are required")
	}
	// A firehose search (keyword "*") lists the whole location.
	firehose := strings.TrimSpace(e.Board) == LinkedInAnyKeyword
	pageLimit, windowSeconds, easyPages, err := linkedInSearchOptions(e.Tenants)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	ids := make(map[string]bool)
	listed, read, refreshed := 0, 0, 0
	// The buffered pipeline discards every job returned alongside an error. Once any
	// posting was read, preserve progress on a later failure; no unseen sweep can act
	// on this partial listing, so stopping successfully cannot falsely close anything.
	stop := func(err error) ([]Job, error) {
		if read+refreshed > 0 {
			log.Printf("linkedin: keeping %d postings after partial crawl: %v", len(jobs), err)
			return jobs, nil
		}
		return jobs, err
	}
	// pass pages one listing: the primary search, or the same search narrowed by f_AL. The
	// filter is only a hint for where to spend detail requests; every card goes through the
	// same path and the detail page alone decides the application method.
	pass := func(extra url.Values, limit int, emptyOK bool) error {
		offset := 0
		for page := 0; page < limit; page++ {
			params := url.Values{"location": {e.Region}, "f_TPR": {"r" + strconv.Itoa(windowSeconds)}, "sortBy": {"DD"}, "start": {strconv.Itoa(offset)}}
			if !firehose {
				params.Set("keywords", e.Board)
			}
			for k, v := range extra {
				params[k] = v
			}
			root, final, err := s.get(ctx, linkedInSearchURL+"?"+params.Encode())
			if err != nil {
				return fmt.Errorf("linkedin: listing: %w", err)
			}
			resolved, err := url.Parse(final)
			if err != nil || resolved.Host != "www.linkedin.com" || resolved.Path != "/jobs-guest/jobs/api/seeMoreJobPostings/search" {
				return fmt.Errorf("linkedin: search redirected away from listing")
			}
			var cards []Job
			pageSize, duplicates := 0, 0
			walk(root, func(n *html.Node) bool {
				urn := Attr(n, "data-entity-urn")
				if !strings.HasPrefix(urn, "urn:li:jobPosting:") {
					return true
				}
				id := strings.TrimPrefix(urn, "urn:li:jobPosting:")
				pageSize++
				if id == "" || LinkedInPostingID(LinkedInPostingURL(id)) != id {
					return false
				}
				if ids[id] {
					duplicates++
					return false
				}
				title, company := firstByClass(n, "base-search-card__title"), firstByClass(n, "base-search-card__subtitle")
				if title == nil || company == nil {
					return false
				}
				j := Job{ExternalID: id, URL: LinkedInPostingURL(id), Title: strings.TrimSpace(textContent(title)), Company: strings.TrimSpace(textContent(company))}
				if j.Title != "" && j.Company != "" {
					ids[id] = true
					cards = append(cards, j)
				}
				return false
			})
			offset += pageSize
			// A page whose cards were all listed by an earlier pass (the Easy Apply pass's newest
			// cards are mostly in the primary listing) is not the end; only a page with nothing
			// recognizable ends the listing, and the page limit bounds the loop.
			if len(cards) == 0 && duplicates > 0 {
				continue
			}
			if len(cards) == 0 {
				if page == 0 && !emptyOK {
					return errLinkedInEmptyFirstListing
				}
				break
			}
			for _, card := range cards {
				listed++
				if seen != nil && seen(card.ExternalID) {
					card.SeenRefresh = true
					refreshed++
					jobs = append(jobs, card)
					continue
				}
				j, err := s.Posting(ctx, card.URL)
				var status *StatusError
				if errors.As(err, &status) && (status.Code == 403 || status.Code == 429 || status.Code == 999) {
					return fmt.Errorf("linkedin: detail refused: %w", err)
				}
				if err != nil {
					card.Unreadable = true
					jobs = append(jobs, card)
					continue
				}
				read++
				if !j.Removed {
					jobs = append(jobs, j)
				}
			}
		}
		return nil
	}
	if err := pass(nil, pageLimit, false); err != nil {
		if errors.Is(err, errLinkedInEmptyFirstListing) {
			return nil, err
		}
		return stop(err)
	}
	easyListed := 0
	if easyPages > 0 {
		before := listed
		err := pass(url.Values{"f_AL": {"true"}}, easyPages, true)
		easyListed = listed - before
		if err != nil {
			return stop(err)
		}
	}
	if listed > 0 && read+refreshed == 0 {
		return jobs, fmt.Errorf("linkedin: none of %d listed postings could be read", listed)
	}
	// A firehose search already lists the whole market newest first; one posting's
	// recommendations would add older postings at up to 20 detail reads per search.
	if !firehose {
		recs, closedSeed := s.related(ctx, jobs, ids, seen)
		if closedSeed != "" {
			jobs = slices.DeleteFunc(jobs, func(j Job) bool { return j.SeenRefresh && j.ExternalID == closedSeed })
		}
		jobs = append(jobs, recs...)
	}
	log.Printf("linkedin: search keyword=%q region=%q page_limit=%d window_hours=%d easyapply_pages=%d easyapply_listed=%d listed=%d read=%d refreshed=%d returned=%d", e.Board, e.Region, pageLimit, windowSeconds/3600, easyPages, easyListed, listed, read, refreshed, len(jobs))
	return jobs, nil
}
