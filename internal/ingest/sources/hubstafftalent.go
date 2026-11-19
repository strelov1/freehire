package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// hubstafftalent adapts Hubstaff Talent (hubstafftalent.net), a free remote and freelance job
// marketplace. Boardless and multi-company: each posting names its client. See
// providers/hubstafftalent.md for the measurements behind each rule.
//
// There is no JSON API and no job sitemap. The search page fills #results from a Rails UJS
// answer: the same URL asked with X-Requested-With returns `$('#results').html("<escaped
// markup>")`, fifteen postings a page. The cards carry the title, client, date and hourly pay but
// only a clipped bio, so a posting's page supplies the body, and only for postings the
// catalogue lacks. Cloudflare answers a burst with 429 (error 1015), so every request goes
// through one limiter.
type hubstafftalent struct {
	http         hubstafftalentHTTP
	limiter      waiter
	maxNewPerRun int
}

type hubstafftalentHTTP interface {
	HeaderTextGetter
	HTMLGetter
}

const (
	// hubstafftalentBase is the canonical host: talent.hubstaff.com 302s every path onto it.
	hubstafftalentBase = "https://hubstafftalent.net"
	hubstafftalentList = hubstafftalentBase + "/search/jobs?page=%d"
	// hubstafftalentMaxPages is a safety ceiling over the ~50 pages the site has.
	hubstafftalentMaxPages = 100
	// hubstafftalentRequestInterval holds the run under Cloudflare's rate rule: one request every
	// 6s was answered in full across 25 pages, one a second was refused 6 times in 10.
	hubstafftalentRequestInterval = 5 * time.Second
	// hubstafftalentMaxNewPerRun bounds the postings one run hydrates (~10 minutes at the pace
	// above); the first run reads the newest, later runs the rest.
	hubstafftalentMaxNewPerRun = 120
)

// NewHubstaffTalent builds the Hubstaff Talent adapter over the given HTTP client.
func NewHubstaffTalent(c hubstafftalentHTTP) Source {
	return hubstafftalent{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(hubstafftalentRequestInterval), 1),
		maxNewPerRun: hubstafftalentMaxNewPerRun,
	}
}

func (hubstafftalent) Provider() string { return "hubstafftalent" }

func (hubstafftalent) boardless() {}

func (hubstafftalent) aggregator() {}

// hubstafftalentHourly reads a card's "$8/hr" pay. A fixed-price card shows a budget ("$500"),
// not a rate, and is left unread.
var hubstafftalentHourly = regexp.MustCompile(`^\$(\d+)/hr$`)

// hubstafftalentPosted is the tooltip on a card's age, in UTC: "November 18, 2026 at 9:45 pm".
const hubstafftalentPosted = "January 2, 2006 at 3:04 pm"

// Fetch is the list-only fallback HydratingSource specifies: every posting is unseen.
func (s hubstafftalent) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting, refreshes the ones already stored and hydrates up to
// maxNewPerRun of the rest, newest first. A posting whose page cannot be read is not yielded:
// nothing is stored for it yet, so a later run simply tries again.
func (s hubstafftalent) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var unseen, jobs []Job
	for _, j := range cards {
		switch {
		case seen(j.ExternalID):
			j.SeenRefresh = true
			jobs = append(jobs, j)
		case len(unseen) < s.maxNewPerRun:
			unseen = append(unseen, j)
		}
	}
	var read atomic.Int64
	jobs = append(jobs, fetchDetails(unseen, 2, func(j Job) (Job, bool) {
		body, err := s.detail(ctx, j)
		if err != nil {
			log.Printf("hubstafftalent: body %s failed; deferring: %v", j.ExternalID, err)
			return Job{}, false
		}
		read.Add(1)
		j.Description = body
		return j, true
	})...)
	// Postings were due a body and none could be read: the pages changed shape or are walled,
	// which a list-only success would report as a healthy crawl.
	if len(unseen) > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("hubstafftalent: fetched %d posting pages and read none of them", len(unseen))
	}
	return jobs, nil
}

// list walks the search pages until one holds no posting. Any page failing fails the crawl: a
// short list would let the sweep close the postings it did not read.
func (s hubstafftalent) list(ctx context.Context) ([]Job, error) {
	var out []Job
	ids := make(map[string]bool)
	for page := 1; page <= hubstafftalentMaxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		body, err := s.http.GetTextWithHeaders(ctx, fmt.Sprintf(hubstafftalentList, page), map[string]string{
			"X-Requested-With": "XMLHttpRequest",
			"Accept":           "text/javascript",
		})
		if err != nil {
			return nil, fmt.Errorf("hubstafftalent: page %d: %w", page, err)
		}
		cards, err := hubstafftalentCards(body)
		if err != nil {
			return nil, fmt.Errorf("hubstafftalent: page %d: %w", page, err)
		}
		if len(cards) == 0 {
			if page == 1 {
				return nil, fmt.Errorf("hubstafftalent: first page holds no posting")
			}
			return out, nil
		}
		for _, j := range cards {
			if !ids[j.ExternalID] {
				ids[j.ExternalID] = true
				out = append(out, j)
			}
		}
	}
	return nil, fmt.Errorf("hubstafftalent: listing still not exhausted after %d pages", hubstafftalentMaxPages)
}

// hubstafftalentCards decodes one search answer: a JS call whose argument is the markup as a
// string literal, whose escapes (\" \/ \n) are JSON's.
func hubstafftalentCards(body string) ([]Job, error) {
	_, after, ok := strings.Cut(body, `$('#results').html(`)
	if !ok {
		return nil, fmt.Errorf("no #results update in the answer")
	}
	var markup string
	if err := json.NewDecoder(strings.NewReader(after)).Decode(&markup); err != nil {
		return nil, fmt.Errorf("results markup: %w", err)
	}
	root, err := xhtml.Parse(strings.NewReader(markup))
	if err != nil {
		return nil, err
	}
	var jobs []Job
	walk(root, func(n *xhtml.Node) bool {
		if n.Type == xhtml.ElementNode && HasClass(n, "search-result") {
			if j, ok := hubstafftalentCard(n); ok {
				jobs = append(jobs, j)
			}
			return false
		}
		return true
	})
	return jobs, nil
}

// hubstafftalentCard maps one result card to a body-less Job; ok=false when it lacks the slug or
// the client the company slug is built from.
func hubstafftalentCard(card *xhtml.Node) (Job, bool) {
	name := firstByClass(card, "name")
	agency := firstByClass(card, "job-agency")
	if name == nil || agency == nil {
		return Job{}, false
	}
	slug := strings.TrimPrefix(Attr(name, "href"), "/jobs/")
	company := collapseSpace(textContent(agency))
	title := collapseSpace(textContent(name))
	if slug == "" || slug == Attr(name, "href") || company == "" || title == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     slug,
		URL:            hubstafftalentBase + "/jobs/" + slug,
		Title:          title,
		Company:        company,
		CompanyWebsite: hubstafftalentWebsite(Attr(agency, "href")),
		Location:       "Remote",
	}
	if loc := firstByClass(card, "location"); loc != nil {
		j.Location = collapseSpace(strings.TrimPrefix(collapseSpace(textContent(loc)), "HQ:"))
	}
	// The site is a remote marketplace; the icon is the card's own statement of it.
	if firstByClass(card, "hi-remote") != nil {
		j.Remote, j.WorkMode = true, "remote"
	}
	if label := firstByClass(card, "label"); label != nil {
		switch collapseSpace(textContent(label)) {
		case "full time":
			j.EmploymentType = "full_time"
		case "hourly", "fixed price":
			j.EmploymentType = "contract"
		}
	}
	if pay := firstByClass(card, "pay-rate"); pay != nil {
		if m := hubstafftalentHourly.FindStringSubmatch(collapseSpace(textContent(pay))); m != nil {
			rate, _ := strconv.Atoi(m[1])
			j.SalaryMin, j.SalaryMax = &rate, &rate
			j.SalaryCurrency, j.SalaryPeriod = "USD", "hour"
		}
	}
	// The age tooltip holds the exact time; the visible text is "4 hrs ago".
	walk(card, func(n *xhtml.Node) bool {
		if j.PostedAt == nil && n.Type == xhtml.ElementNode && HasClass(n, "a-tooltip") {
			j.PostedAt = parseLayout(hubstafftalentPosted, collapseSpace(Attr(n, "data-original-title")))
		}
		return j.PostedAt == nil
	})
	return j, true
}

// hubstafftalentWebsite keeps the client's own site; the profile links nothing else.
func hubstafftalentWebsite(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return raw
}

// detail reads the posting page's body. A page without one is unreadable, never an empty body.
func (s hubstafftalent) detail(ctx context.Context, j Job) (string, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return "", err
	}
	root, err := s.http.GetHTML(ctx, j.URL)
	if err != nil {
		return "", err
	}
	desc := firstByClass(root, "job-description")
	if desc == nil {
		return "", fmt.Errorf("no job-description in the page")
	}
	body := sanitizeHTML(innerHTML(desc))
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("empty job-description")
	}
	return body, nil
}
