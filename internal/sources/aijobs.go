package sources

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

// aijobs adapts aijobs.net, a large AI/ML job aggregator (~47k postings). It is boardless
// (one global feed, no per-tenant board) and multi-company, so it stays in the source facet
// and takes each posting's company from the feed. The listing is a Django CSRF-protected
// POST endpoint (not a public JSON API), so the adapter needs a cookie-jar-backed client
// (built with newCookieClient in the registry, mirroring taleo's session-bound flow) rather
// than the shared client.
type aijobs struct {
	http aijobsHTTP
	// maxNewPerRun bounds how many unseen postings FetchNew hydrates in one run (see
	// crawlListing's newBudget and design.md Decision 3). Always positive: NewAijobs
	// substitutes aijobsDefaultMaxNewPerRun for a zero/negative value, because
	// crawlListing reads newBudget <= 0 as "unlimited," not "nothing" — forwarding a raw
	// zero here would mean "crawl the whole backlog in one run," not "crawl nothing new."
	maxNewPerRun int
}

// aijobsHTTP is the transport aijobs needs: the CSRF-protected listing POST and the
// per-posting detail-page GET.
type aijobsHTTP interface {
	HeaderFormPoster
	HTMLGetter
	CookieReader
}

// aijobsDefaultMaxNewPerRun is AIJOBS_MAX_NEW_PER_RUN's default when unset (see
// registry.go's aijobsMaxNewPerRunFromEnv): the day-one backlog is aijobs.net's whole
// ~47k-posting catalogue, and each unseen posting costs one detail-page GET, so a run
// needs a bound well short of "all of it" (mirrors APPLY_FORM_MAX_PER_RUN's shape in
// cmd/capture-apply-form for the same kind of backlog).
const aijobsDefaultMaxNewPerRun = 500

// NewAijobs builds the aijobs adapter over the given HTTP client (a cookie-jar-backed
// client in production, see cookieSessionSource in registry.go). maxNewPerRun <= 0 is
// replaced with aijobsDefaultMaxNewPerRun.
func NewAijobs(c aijobsHTTP, maxNewPerRun int) Source {
	if maxNewPerRun <= 0 {
		maxNewPerRun = aijobsDefaultMaxNewPerRun
	}
	return aijobs{http: c, maxNewPerRun: maxNewPerRun}
}

func (aijobs) Provider() string { return "aijobs" }

// aijobs needs no board id (one global feed), so its config carries no board.
func (aijobs) boardless() {}

// aijobs aggregates postings from many companies, so it stays in the source facet.
func (aijobs) aggregator() {}

// FetchNew is the hydrating crawl cmd/ingest actually drives: it walks the listing
// (crawlListing, bounded by maxNewPerRun and the seen-page stop) and fetches detail only
// for a posting seen reports as not already in the catalogue.
func (a aijobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	links, err := a.crawlListing(ctx, seen, a.maxNewPerRun)
	if err != nil {
		return nil, err
	}
	return fetchDetails(links, defaultDetailWorkers, func(href string) (Job, bool) {
		return a.detail(ctx, href)
	}), nil
}

// Fetch is the list-only fallback HydratingSource specifies for a caller that cannot
// supply a seen predicate. aijobs has no such thing as a list-only Job — the listing
// carries no company, and Job.Company is required (see detail) — so this hydrates
// everything the listing yields (as if nothing were seen yet), still bounded by
// maxNewPerRun.
func (a aijobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return a.FetchNew(ctx, e, func(string) bool { return false })
}

// aijobsMaxNewPerRunFromEnv reads AIJOBS_MAX_NEW_PER_RUN, falling back to
// aijobsDefaultMaxNewPerRun when unset or not a positive integer — read once at registry
// construction (see registry.go), not per crawl.
func aijobsMaxNewPerRunFromEnv() int {
	v := os.Getenv("AIJOBS_MAX_NEW_PER_RUN")
	if v == "" {
		return aijobsDefaultMaxNewPerRun
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return aijobsDefaultMaxNewPerRun
	}
	return n
}

const aijobsBaseURL = "https://aijobs.net"

// aijobsJobIDPattern matches a job-detail path (/job/<slug>-<id>/) and captures the
// trailing numeric id, anchored so a company or skill-filter link (which shares no such
// suffix) never matches.
var aijobsJobIDPattern = regexp.MustCompile(`^/job/[^/]+-(\d+)/?$`)

// aijobsJobID extracts the native aijobs.net posting id from a job-detail href, "" when
// href is not a job-detail link.
func aijobsJobID(href string) string {
	m := aijobsJobIDPattern.FindStringSubmatch(href)
	if m == nil {
		return ""
	}
	return m[1]
}

// aijobsListingLinks collects every job-detail link on a listing page, in document order
// (the feed's own newest-first sort), filtering out the company/skill/education/role
// filter links a listing card also carries.
func aijobsListingLinks(root *html.Node) []string {
	var hrefs []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := attr(n, "href"); aijobsJobIDPattern.MatchString(href) {
				hrefs = append(hrefs, href)
			}
		}
		return true
	})
	return hrefs
}

// aijobsMaxPages is the hard pagination safety cap, independent of the seen-based stop
// (see crawlListing): ~47k postings at 50/page is ~956 pages, so this leaves headroom
// while still bounding a run if the feed's sort order or markup ever silently changes.
// A var, not a const, so a test can shrink it rather than paginate to the real cap.
var aijobsMaxPages = 1200

// bootstrapSession GETs the aijobs.net home page to obtain the csrftoken cookie the
// listing POST must echo back (Django's double-submit CSRF pattern — see
// Client.CookieValue). Returns an error if the site sets no such cookie.
func (a aijobs) bootstrapSession(ctx context.Context) (string, error) {
	if _, err := a.http.GetHTML(ctx, aijobsBaseURL+"/"); err != nil {
		return "", err
	}
	token := a.http.CookieValue(aijobsBaseURL+"/", "csrftoken")
	if token == "" {
		return "", fmt.Errorf("aijobs: no csrftoken cookie set by %s", aijobsBaseURL)
	}
	return token, nil
}

// listPage requests one listing page, authorized by the session's csrfToken (echoed as
// both the x-csrftoken header and the csrfmiddlewaretoken form field — the value the
// cookie already carries, so no separate per-page token scrape is needed). Referer is
// required: aijobs.net's CSRF check rejects a same-value token without it.
func (a aijobs) listPage(ctx context.Context, csrfToken string, page int) (*html.Node, error) {
	values := url.Values{
		"csrfmiddlewaretoken": {csrfToken},
	}
	headers := map[string]string{
		"x-csrftoken": csrfToken,
		"referer":     aijobsBaseURL + "/",
		// The site's own frontend paginates this endpoint via htmx (confirmed live), which
		// marks every one of its requests this way; sent for parity with the real client.
		"hx-request": "true",
	}
	return a.http.PostFormWithHeaders(ctx, fmt.Sprintf("%s/?page=%d", aijobsBaseURL, page), headers, values)
}

// crawlListing walks the listing pages (newest posting first) and returns the job-detail
// hrefs of postings seen reports as NOT already in the catalogue, in discovery order. It
// stops — without requesting a further page — as soon as either condition holds:
//   - a whole page's postings are all already seen: the feed has been caught up to, so
//     every following page (older still) would be too;
//   - newBudget unseen postings have been found (0 means unbounded): this run's
//     per-run detail-fetch budget (see AIJOBS_MAX_NEW_PER_RUN); the postings past the
//     cap remain unseen and are picked up on a later run.
//
// A first-page failure is a board-level error; a later page failing ends the walk with
// what was gathered so far (a partial crawl survives a mid-listing hiccup).
func (a aijobs) crawlListing(ctx context.Context, seen func(externalID string) bool, newBudget int) ([]string, error) {
	csrfToken, err := a.bootstrapSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("aijobs: session bootstrap: %w", err)
	}

	var unseen []string
	for page := 1; page <= aijobsMaxPages; page++ {
		root, err := a.listPage(ctx, csrfToken, page)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("aijobs: listing page %d: %w", page, err)
			}
			break
		}
		links := aijobsListingLinks(root)
		if len(links) == 0 {
			break // empty page, or a page number clamped past the last one
		}
		allSeen := true
		for _, href := range links {
			id := aijobsJobID(href)
			if id == "" || seen(id) {
				continue
			}
			allSeen = false
			unseen = append(unseen, href)
			if newBudget > 0 && len(unseen) >= newBudget {
				return unseen, nil
			}
		}
		if allSeen {
			break // caught up to already-known postings; every later page is older still
		}
	}
	return unseen, nil
}

// aijobsCompanySlugPattern matches a company-profile href and captures its slug, without
// the trailing "-<id>" suffix every such link carries.
var aijobsCompanySlugPattern = regexp.MustCompile(`^/company/(.+)-\d+/?$`)

// aijobsCompanyName finds the detail page's company-profile link and derives a display
// name from its slug — the page's own visible label is masked behind a PRO paywall
// ("@ M..."), but the slug in the href is not (see design.md Decision 5). "" if the page
// carries no such link.
func aijobsCompanyName(root *html.Node) string {
	var slug string
	walk(root, func(n *html.Node) bool {
		if slug == "" && n.Type == html.ElementNode && n.Data == "a" {
			if m := aijobsCompanySlugPattern.FindStringSubmatch(attr(n, "href")); m != nil {
				slug = m[1]
			}
		}
		return true
	})
	if slug == "" {
		return ""
	}
	return titleCaseSlug(slug)
}

// titleCaseSlug renders a hyphen-separated URL slug as a display name by upper-casing
// each word's first rune (a Django slug is already lower-case, so nothing else changes).
func titleCaseSlug(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// aijobsIsRemote reports whether the detail page carries the "R" work-arrangement badge
// (a span styled text-bg-success, exact text "R" — distinct from the same class used on
// other badges, which never render that exact text).
func aijobsIsRemote(root *html.Node) bool {
	remote := false
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "span" && hasClass(n, "text-bg-success") {
			if strings.TrimSpace(textContent(n)) == "R" {
				remote = true
			}
		}
		return true
	})
	return remote
}

// aijobsSections maps each <h5> section heading's trimmed text (Tasks, Perks/Benefits,
// Skills/Tech-stack, …) to its immediately following sibling content node (a <ul> or a
// <p>). It walks the whole page once: an <h5> is remembered as "pending" and paired with
// the next <ul>/<p> element the walk reaches — true for this page's layout, where each
// section heading is immediately followed by its own content block.
func aijobsSections(root *html.Node) map[string]*html.Node {
	sections := make(map[string]*html.Node)
	var pending string
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "h5":
			pending = strings.TrimSpace(textContent(n))
			return false // the heading's own text is not section content
		case "ul", "p":
			if pending != "" {
				if _, exists := sections[pending]; !exists {
					sections[pending] = n
				}
				pending = ""
			}
		}
		return true
	})
	return sections
}

// aijobsListItems reads a <ul>'s direct <li> children as trimmed text, nil if n is nil
// (section absent) or empty.
func aijobsListItems(n *html.Node) []string {
	if n == nil {
		return nil
	}
	var items []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "li" {
			if text := strings.TrimSpace(textContent(c)); text != "" {
				items = append(items, text)
			}
		}
	}
	return items
}

// aijobsAnchorTexts reads every <a>'s trimmed text within n, nil if n is nil.
func aijobsAnchorTexts(n *html.Node) []string {
	if n == nil {
		return nil
	}
	var texts []string
	walk(n, func(c *html.Node) bool {
		if c.Type == html.ElementNode && c.Data == "a" {
			if text := strings.TrimSpace(textContent(c)); text != "" {
				texts = append(texts, text)
			}
		}
		return true
	})
	return texts
}

// aijobsIsPlaceholder reports whether a section's only item is the literal text N/A —
// the site's own placeholder for "nothing here", observed on every sampled posting with
// no real perks.
func aijobsIsPlaceholder(items []string) bool {
	return len(items) == 1 && items[0] == "N/A"
}

// aijobsDescription renders the Tasks section as a bullet list (there is no free-text
// prose anywhere on the page — the site pre-processes every posting into these structured
// sections), followed by a Perks/Benefits bullet list when it carries real content (the
// literal-N/A placeholder is omitted rather than rendered as noise).
func aijobsDescription(sections map[string]*html.Node) string {
	var b strings.Builder
	for _, task := range aijobsListItems(sections["Tasks"]) {
		fmt.Fprintf(&b, "- %s\n", task)
	}
	if perks := aijobsListItems(sections["Perks/Benefits"]); len(perks) > 0 && !aijobsIsPlaceholder(perks) {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Perks/Benefits:\n")
		for _, perk := range perks {
			fmt.Fprintf(&b, "- %s\n", perk)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// aijobsPostedPrefixes are the two labels observed live for the same relative-time
// element: "Found X ago" on a posting crawled recently, "Published X ago" on an older one
// with a recorded original date (confirmed against
// https://aijobs.net/job/statistics-expert-phd-remote-238344/, which reads "Published
// 15d ago" — a shape task 4's single-sample fixture never exercised).
var aijobsPostedPrefixes = []string{"Found ", "Published "}

// aijobsPostedText extracts the detail page's relative-posting-time text (e.g. "8h ago"
// from "Found 8h ago" or "15d ago" from "Published 15d ago"), "" if the page carries no
// such element. Parsing this into an absolute PostedAt is aijobsParsePostedAt's job, not
// this one.
func aijobsPostedText(root *html.Node) string {
	var text string
	walk(root, func(n *html.Node) bool {
		if text != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "span" {
			t := strings.TrimSpace(textContent(n))
			for _, prefix := range aijobsPostedPrefixes {
				if strings.HasPrefix(t, prefix) {
					text = strings.TrimSpace(strings.TrimPrefix(t, prefix))
					break
				}
			}
		}
		return true
	})
	return text
}

// detail fetches one job's detail page and maps it to a Job. It returns ok=false when the
// page fetch fails, its heading is missing (a layout/error page, not a posting), or it
// carries no /company/ link — the page's own visible company label is masked behind a PRO
// paywall, and without the link there is no way to recover a real name (see
// aijobsCompanyName), so the posting is dropped rather than stored company-less.
func (a aijobs) detail(ctx context.Context, href string) (Job, bool) {
	id := aijobsJobID(href)
	if id == "" {
		return Job{}, false
	}
	link := aijobsBaseURL + href
	root, err := a.http.GetHTML(ctx, link)
	if err != nil {
		return Job{}, false
	}
	h1 := firstByTag(root, "h1")
	if h1 == nil {
		return Job{}, false
	}
	company := aijobsCompanyName(root)
	if company == "" {
		return Job{}, false
	}
	var location string
	if loc := firstByTag(root, "strong"); loc != nil {
		location = strings.TrimSpace(textContent(loc))
	}
	remote := aijobsIsRemote(root)
	sections := aijobsSections(root)
	return Job{
		ExternalID:  id,
		URL:         link,
		Title:       strings.TrimSpace(textContent(h1)),
		Company:     company,
		Location:    location,
		Description: aijobsDescription(sections),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		Skills:      aijobsAnchorTexts(sections["Skills/Tech-stack"]),
		PostedAt:    aijobsParsePostedAt(aijobsPostedText(root), time.Now()),
	}, true
}

// aijobsPostedTimePattern captures a relative-time value and its unit from text like
// "8h ago", "3d ago", "2w ago", "5mo ago", "1y ago" — only "h"/"d"/"w" were observed live,
// but the site's own broader vocabulary implies "mo"/"y" exist for older postings.
var aijobsPostedTimePattern = regexp.MustCompile(`^(\d+)(h|d|w|mo|y)\s+ago$`)

// aijobsParsePostedAt converts the detail page's relative-time text (see aijobsPostedText)
// into an absolute PostedAt relative to now. An unrecognized shape returns nil rather than
// failing the whole posting (see spec Requirement "Posted time is parsed from the
// relative-time string").
func aijobsParsePostedAt(text string, now time.Time) *time.Time {
	m := aijobsPostedTimePattern.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	var t time.Time
	switch m[2] {
	case "h":
		t = now.Add(-time.Duration(n) * time.Hour)
	case "d":
		t = now.AddDate(0, 0, -n)
	case "w":
		t = now.AddDate(0, 0, -7*n)
	case "mo":
		// Flat approximation (30 days/month), not calendar-aware AddDate(0, -n, 0):
		// PostedAt here is a freshness signal, not a legal date, so month-length
		// variance doesn't matter enough to justify "1mo ago" and "30d ago" disagreeing.
		t = now.AddDate(0, 0, -30*n)
	case "y":
		t = now.AddDate(0, 0, -365*n)
	default:
		return nil
	}
	return &t
}
