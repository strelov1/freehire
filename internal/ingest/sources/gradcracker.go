package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// gradcracker adapts Gradcracker (www.gradcracker.com), the UK board for STEM graduate jobs,
// placements and internships. It is a multi-company aggregator crawled as ONE boardless
// catalogue: the board would naturally be a discipline, but a posting is listed under every
// discipline it accepts — 1,018 of electronic-electrical's 1,350 postings were also on
// computing-technology's 1,455 (2026-10-18) — and the pipeline namespaces external_id by the
// board string, so a board per discipline would store each of those postings twice. One crawl
// walks every discipline in gradcrackerDisciplines and keeps each posting once.
//
// The listing card states everything but the body (title, employer, location, salary,
// deadline, degree, start, duration, accepted disciplines); the posting page carries the body.
// Neither carries schema.org JobPosting. It is a HydratingSource — a posting the catalogue
// holds is refreshed from its card and costs no posting-page request — and CoverageGated, so
// no body is bought for a posting the pipeline will discard.
//
// Every crawl reads every listing page of every discipline or fails, which is what makes it a
// fullCatalog: a stored posting no listing names is gone. The proof is the listing's own
// count: the walk must reach the page past the last (404) AND read as many cards as the
// listing's "Show N results" states, otherwise the run fails instead of sweeping on a
// shrunken catalogue.
//
// Every path sits behind Cloudflare. Go's default TLS fingerprint and curl are answered with a
// managed challenge (403 "Just a moment...") from a residential IP and from both of our hosts;
// the Chrome-fingerprint transport (fingerprintHTTP) is served 200, keyless. The edge also
// RATE-limits per IP — see gradcrackerRequestInterval — and blocks an IP that exceeds it for
// about an hour, so the first refusal ends the run's hydration.
type gradcracker struct {
	http HTMLGetter
	// maxNewPerRun bounds how many posting pages one run hydrates; the rest are deferred
	// (not yielded, so not stored) and offered again by a later run.
	maxNewPerRun int
}

const (
	gradcrackerBaseURL = "https://www.gradcracker.com"
	// gradcrackerMaxPages is a safety ceiling on one discipline's walk (80 cards a page;
	// computing-technology was 19 pages on 2026-10-18). Reaching it fails the run.
	gradcrackerMaxPages = 60
	// gradcrackerRequestInterval paces every request one run makes, listing and posting
	// pages alike, through one limiter: the edge counts the IP, not the path. Measured
	// 2026-10-18: at one request per 3s (and per 0.5s, with or without a cookie jar) the
	// edge served ~35 and then refused every path from that IP for 55-65 minutes — from the
	// crawl host, fsn and a residential IP alike; at one per 10s it served 60 posting pages
	// from one IP and the full 37-page listing walk plus the sitemap from another, without
	// a single refusal. The pace stays at the measured one rather than above it.
	gradcrackerRequestInterval = 10 * time.Second
	gradcrackerRequestBurst    = 1
	// gradcrackerDefaultMaxNewPerRun is the per-run hydration budget: at the pace above,
	// ~33 minutes on top of the ~6-minute listing walk, inside the 50-minute run timeout.
	// Steady state needs a few dozen; the first crawl (~1,800 postings) accretes over runs.
	gradcrackerDefaultMaxNewPerRun = 200
)

// gradcrackerDisciplines are the discipline slugs whose listings are crawled: the computing
// family (software, IT, data, AI, cyber, hardware) and electronic/electrical engineering. The
// site's other disciplines (civil, mechanical, chemical, aerospace, science, maths/business)
// are never listed, so their postings are never stored.
var gradcrackerDisciplines = []string{"computing-technology", "electronic-electrical"}

var (
	// gradcrackerPostingPath is a posting page's path: /hub/<employer-id>/<employer-slug>/
	// <opportunity-type>/<posting-id>/<slug>. The type is graduate-job,
	// work-placement-internship or degree-apprenticeship; employer hub pages are shorter.
	gradcrackerPostingPath = regexp.MustCompile(`^/hub/\d+/[^/]+/([a-z-]+)/(\d+)(?:/|$)`)
	// gradcrackerResults is the listing's own count ("Show 1455 results").
	gradcrackerResults = regexp.MustCompile(`Show\s+([\d,]+)\s+results`)
	gradcrackerBase, _ = url.Parse(gradcrackerBaseURL)
)

// NewGradcracker builds the Gradcracker adapter over the Chrome-fingerprint transport, which
// production paces (pacedGradcracker). A non-positive maxNewPerRun takes the default.
func NewGradcracker(c HTMLGetter, maxNewPerRun int) Source {
	if maxNewPerRun <= 0 {
		maxNewPerRun = gradcrackerDefaultMaxNewPerRun
	}
	return gradcracker{http: c, maxNewPerRun: maxNewPerRun}
}

// pacedGradcracker paces every request one run makes through one limiter.
func pacedGradcracker(c HTMLGetter) HTMLGetter {
	return pacedHTMLGetter(c, gradcrackerRequestInterval, gradcrackerRequestBurst)
}

func (gradcracker) Provider() string { return "gradcracker" }

// One catalogue; see the type comment for why it is not a board per discipline.
func (gradcracker) boardless() {}

// Many employers, the employer read from each listing card.
func (gradcracker) aggregator() {}

// Every run reads the whole listing or fails; see the type comment.
func (gradcracker) fullCatalog() {}

// gradcrackerCard is one listing card: everything Gradcracker states about a posting bar its body.
type gradcrackerCard struct {
	ID          string
	URL         string
	Type        string // graduate-job | work-placement-internship | degree-apprenticeship
	Title       string
	Company     string
	Disciplines string
	Facts       []gradcrackerFact // the card's <dl>, in page order
}

// gradcrackerFact is one labelled value from a card's <dl> (Deadline, Salary, Location, ...).
type gradcrackerFact struct{ Label, Value string }

func (c gradcrackerCard) fact(label string) string {
	for _, f := range c.Facts {
		if f.Label == label {
			return f.Value
		}
	}
	return ""
}

func (g gradcracker) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return g.FetchNew(ctx, e, func(string) bool { return false })
}

func (g gradcracker) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return g.FetchNewGated(ctx, e, seen, DetailGate{})
}

func (g gradcracker) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := g.list(ctx)
	if err != nil {
		return nil, err
	}
	var out []Job
	var fresh []gradcrackerCard
	for _, c := range cards {
		if seen(c.ID) {
			j := c.job()
			j.SeenRefresh = true
			out = append(out, j)
			continue
		}
		fresh = append(fresh, c)
	}
	hydrated, err := g.hydrate(ctx, fresh, gate)
	if err != nil {
		return nil, err
	}
	return append(out, hydrated...), nil
}

// list walks every discipline's listing, newest first, and returns each posting's card once.
// ANY page failing fails the run, and so does a walk that read fewer cards than the listing
// says it holds: this is a fullCatalog, and a truncated walk would read as a shrunken catalogue
// to the source-scoped sweep.
func (g gradcracker) list(ctx context.Context) ([]gradcrackerCard, error) {
	byID := map[string]bool{}
	var out []gradcrackerCard
	for _, d := range gradcrackerDisciplines {
		total, read, reachedEnd := 0, 0, false
		for page := 1; page <= gradcrackerMaxPages; page++ {
			root, err := g.http.GetHTML(ctx, gradcrackerListingURL(d, page))
			if err != nil {
				var se *StatusError
				if page > 1 && errors.As(err, &se) && se.Code == http.StatusNotFound {
					reachedEnd = true // the page past the last
					break
				}
				return nil, fmt.Errorf("gradcracker: listing %s page %d: %w", d, page, err)
			}
			if page == 1 {
				// The count is the walk's only proof of completeness: without it, a page
				// that came back empty mid-walk would end the walk looking like the last.
				if total = gradcrackerListingTotal(root); total == 0 {
					return nil, fmt.Errorf("gradcracker: listing %s page 1 states no result count", d)
				}
			}
			cards := parseGradcrackerListing(root)
			if len(cards) == 0 {
				if page == 1 {
					return nil, fmt.Errorf("gradcracker: listing %s page 1 carries no posting card", d)
				}
				reachedEnd = true
				break
			}
			read += len(cards)
			for _, c := range cards {
				if !byID[c.ID] {
					byID[c.ID] = true
					out = append(out, c)
				}
			}
		}
		if !reachedEnd {
			return nil, fmt.Errorf("gradcracker: listing %s still yielding cards after %d pages", d, gradcrackerMaxPages)
		}
		// A posting added or withdrawn mid-walk shifts the pages by one, so the count is
		// allowed a little slack; a lost page (80 cards) is far outside it.
		if read < total-gradcrackerCountSlack {
			return nil, fmt.Errorf("gradcracker: listing %s read %d cards where it states %d results", d, read, total)
		}
	}
	return out, nil
}

// gradcrackerCountSlack is how far a walk's card count may fall short of the listing's own
// total before the walk is treated as truncated.
const gradcrackerCountSlack = 5

// gradcrackerListingURL is a discipline's listing, newest first.
func gradcrackerListingURL(discipline string, page int) string {
	u := fmt.Sprintf("%s/search/%s/jobs?order=dateAdded", gradcrackerBaseURL, discipline)
	if page > 1 {
		u += "&page=" + strconv.Itoa(page)
	}
	return u
}

// hydrate buys the posting page of each new card the pipeline will keep, newest first, within
// the run's budget. A card the gate condemns is yielded body-less — the pipeline discards it and
// counts why. A new posting whose page could not be read, or past the budget, is DEFERRED
// rather than stored body-less: a stored row is seen, and nothing would buy its body again.
// Once the edge refuses one request the rest of the run's hydration is deferred too: the
// refusal blocks the IP, and every further request would only be refused alike.
func (g gradcracker) hydrate(ctx context.Context, fresh []gradcrackerCard, gate DetailGate) ([]Job, error) {
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(fresh))
		for _, c := range fresh {
			companies = append(companies, c.Company)
		}
		covered = gate.Covered(companies)
	}
	var out []Job
	var toHydrate []gradcrackerCard
	deferred := 0
	for _, c := range fresh {
		switch j := c.job(); {
		case covered[c.Company] || (gate.Rejected != nil && gate.Rejected(j)):
			out = append(out, j)
		case len(toHydrate) < g.maxNewPerRun:
			toHydrate = append(toHydrate, c)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("gradcracker: hydration budget %d reached; %d new postings wait for a later run", g.maxNewPerRun, deferred)
	}
	// Each attempted page ends one of four ways. A body read is stored. A 404/410 is the
	// posting gone — an answer, never stored. A 200 that carries no body is NOT an answer: it
	// is the parser failing on the page, and it is deferred rather than stored body-less (a
	// stored row is seen, and nothing would buy its body again). A refusal, a timeout or a
	// 5xx is no answer at all.
	attempted, read, gone, bodyless := 0, 0, 0, 0
	for _, c := range toHydrate {
		attempted++
		root, err := g.http.GetHTML(ctx, c.URL)
		if err != nil {
			if !detailUnreadable(err) {
				gone++
				continue
			}
			if isRateLimited(err) {
				log.Printf("gradcracker: posting pages refused (%v); deferring the rest of this run's hydration", err)
				break
			}
			continue
		}
		body, ok := gradcrackerBody(root)
		if !ok {
			bodyless++
			continue
		}
		j := c.job()
		j.Description = sanitizeHTML(c.factsHTML() + body)
		out = append(out, j)
		read++
	}
	if unanswered := attempted - read - gone - bodyless; unanswered > 0 || bodyless > 0 {
		log.Printf("gradcracker: of %d posting pages, %d read, %d gone, %d without a body, %d unanswered; the unread wait for a later run",
			attempted, read, gone, bodyless, unanswered)
	}
	// Two endings are failures, not a quiet source. Every request unanswered: a refusing edge
	// would otherwise hide behind a green unit. Pages fetched (200) but not one body parsed:
	// the posting page's layout has changed, and a silent layout change is the worse outcome.
	// One stuck page among good ones passes, and so does a run whose pages were only "gone" —
	// either keeps every stored posting's refresh and the sweep.
	if fetched := read + bodyless; fetched > 0 && read == 0 {
		return nil, fmt.Errorf("gradcracker: %d posting pages fetched and none yielded a body; the page layout may have changed", fetched)
	}
	if attempted > 0 && read+gone+bodyless == 0 {
		return nil, fmt.Errorf("gradcracker: none of the %d posting pages attempted was answered", attempted)
	}
	return out, nil
}

// job is the card as a body-less Job. The opportunity type is the platform's own structured
// split, so a placement/internship states its employment type; the grade is left to the title
// dictionary (internal/dict/classify), which reads Gradcracker's "Graduate" and "Industrial
// Placement" vocabulary.
func (c gradcrackerCard) job() Job {
	location := c.fact("Location")
	j := Job{
		ExternalID: c.ID,
		URL:        c.URL,
		Title:      c.Title,
		Company:    c.Company,
		Location:   location,
		Remote:     isRemote(location),
	}
	if c.Type == "work-placement-internship" {
		j.EmploymentType = "internship"
	}
	return j
}

// factsHTML renders the card's labelled facts and accepted disciplines as a short list ahead of
// the body: the start date, duration, degree and deadline are stated only there.
func (c gradcrackerCard) factsHTML() string {
	var b strings.Builder
	for _, f := range c.Facts {
		if f.Value != "" {
			b.WriteString("<li><strong>" + html.EscapeString(f.Label) + ":</strong> " + html.EscapeString(f.Value) + "</li>")
		}
	}
	if c.Disciplines != "" {
		b.WriteString("<li><strong>Disciplines:</strong> " + html.EscapeString(c.Disciplines) + "</li>")
	}
	if b.Len() == 0 {
		return ""
	}
	return "<ul>" + b.String() + "</ul>"
}

// parseGradcrackerListing reads every posting card on a listing page. Only an <article> whose
// wire:key is the posting id counts: the page also carries a visually hidden honeypot card
// (wire:key="hp") linking a "featured graduate employer" that does not exist.
func parseGradcrackerListing(root *html.Node) []gradcrackerCard {
	var out []gradcrackerCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "article" {
			return true
		}
		if c, ok := parseGradcrackerCard(n); ok {
			out = append(out, c)
		}
		return false
	})
	return out
}

func parseGradcrackerCard(article *html.Node) (gradcrackerCard, bool) {
	var c gradcrackerCard
	if h2 := firstByTag(article, "h2"); h2 != nil {
		if a := firstByTag(h2, "a"); a != nil {
			c.URL = gradcrackerAbsURL(Attr(a, "href"))
			c.Title = collapseSpace(textContent(a))
		}
	}
	m := gradcrackerPostingMatch(c.URL)
	if m == nil || m[2] != Attr(article, "wire:key") {
		return gradcrackerCard{}, false
	}
	c.Type, c.ID = m[1], m[2]
	if fig := firstByTag(article, "figure"); fig != nil {
		if img := firstByTag(fig, "img"); img != nil {
			c.Company = collapseSpace(Attr(img, "alt"))
		}
	}
	if h3 := firstByTag(article, "h3"); h3 != nil {
		c.Disciplines = strings.TrimSuffix(collapseSpace(textContent(h3)), ".")
	}
	walk(article, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "dt" {
			if dd := nextElement(n); dd != nil && dd.Data == "dd" {
				c.Facts = append(c.Facts, gradcrackerFact{Label: collapseSpace(textContent(n)), Value: collapseSpace(textContent(dd))})
			}
			return false
		}
		return true
	})
	if c.Title == "" || c.Company == "" {
		return gradcrackerCard{}, false
	}
	return c, true
}

// gradcrackerListingTotal is the listing's own result count, 0 when it states none.
func gradcrackerListingTotal(root *html.Node) int {
	m := gradcrackerResults.FindStringSubmatch(textContent(root))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	return n
}

// gradcrackerPostingMatch matches a posting URL on the Gradcracker host ([path, type, id]),
// nil otherwise.
func gradcrackerPostingMatch(raw string) []string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Hostname(), "www.gradcracker.com") {
		return nil
	}
	return gradcrackerPostingPath.FindStringSubmatch(u.Path)
}

func gradcrackerAbsURL(href string) string {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	return gradcrackerBase.ResolveReference(ref).String()
}

// gradcrackerBody returns the posting page's description markup: the .body-content block
// without the furniture Gradcracker injects into it — the embedded video/insight panels (class
// not-body-content), the "feedback pledge" badge and the content-hash paragraph. ok=false when
// the page carries no body.
func gradcrackerBody(root *html.Node) (string, bool) {
	body := firstByClass(root, "body-content")
	if body == nil {
		return "", false
	}
	var drop []*html.Node
	walk(body, func(n *html.Node) bool {
		if n == body || n.Type != html.ElementNode {
			return true
		}
		if HasClass(n, "not-body-content") || HasClass(n, "hash") || strings.Contains(Attr(n, "onclick"), "feedback-pledge") {
			drop = append(drop, n)
			return false
		}
		return true
	})
	for _, n := range drop {
		n.Parent.RemoveChild(n)
	}
	if textContent(body) == "" {
		return "", false
	}
	return strings.TrimSpace(innerHTML(body)), true
}
