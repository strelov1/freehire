package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// workstream adapts Workstream (www.workstream.us), the hourly/frontline ATS restaurants,
// retail, care homes and franchise groups hire on. One board is one Workstream EMPLOYER
// account, and its id is the eight hex characters every canonical career-site URL opens with:
//
//	https://www.workstream.us/j/965a796b            <- the board, one employer ("FineCasual")
//	https://www.workstream.us/j/965a796b/moxies     <- one of its brands
//	https://www.workstream.us/j/965a796b/moxies/pickering-79247/server-b8dc419f  <- a posting
//
// That the employer — not the brand and not the store — is the board was settled live:
// Workstream's own careers URL, "/j/<employer-slug>" (the "finecasual", "sarku_japan",
// "hdl_management_usa_corporation" spellings), 301-redirects onto "/j/<hex>", and the employer
// listing at "/j/<hex>/positions" unions every brand's postings (verified: FineCasual's 32
// postings arrive under both "moxies" and "chop-steakhouse-bar"). Crawling brands instead would
// need a second discovery pass to learn which brands an employer runs, and would miss the ones
// it never saw.
//
// Traps, all verified live against the platform on 2026-09-02:
//
//   - "/j/<board>/positions" is the employer-wide listing only for an employer running more than
//     one brand. An employer running exactly one is 301-redirected to that brand's root, which is
//     a LOCATIONS listing, not a positions one. Both pages state where their positions listing is
//     in an inline "var searchBaseUrl" — so the crawl asks for the employer listing, reads that
//     variable, and walks whatever it names. Comparing it against the URL asked for is what keeps
//     the multi-brand case at one request rather than two.
//   - "var totalPages" bounds the walk exactly (unlike SEEK's totalCount): the last page is short
//     and the page after it renders no cards at all, so the added==0 rule agrees with it.
//   - Ten cards a page, and a card carries every field except the body — hence HydratingSource.
//   - The pay line is the platform's own compensation field rendered to a fixed grammar, but its
//     "$" is UNQUALIFIED and Workstream serves US and Canadian employers alike (Chop Steakhouse &
//     Bar's postings are in Vaughan, Ontario), so the symbol alone cannot say which dollar it is.
//     The line is stated in the description verbatim rather than guessed into the structured
//     salary fields — SEEK's salaryLabel reading, for a different cause.
//   - Neither page states a publish date in any form, so PostedAt is always nil.
//   - The employer account is what the board file names, so a multi-brand group's postings all
//     file under the group. The brand is in the posting URL and in the posting page's breadcrumb,
//     but nowhere on a listing card — reading it would cost the SeenRefresh path a request it
//     exists to save, and would leave the employer named two different ways across the two paths.
//   - The origin is AWS API Gateway and it meters by request RATE per IP: measured live, 1 req/s
//     was refused nothing over 40 cold boards while ~1.4 req/s was refused 48%, and the penalty
//     outlives the burst that earned it. Hence the pacer (workstreamRequestInterval).
//   - A retired employer answers 410 Gone rather than 404.
type workstream struct {
	http workstreamHTTP
}

// workstreamHTTP is the transport workstream needs: the listing pages and each posting page as
// HTML.
type workstreamHTTP interface{ HTMLGetter }

// NewWorkstream builds the Workstream adapter over the given HTTP client (the rate-paced one in
// production — see registry.go).
func NewWorkstream(c workstreamHTTP) Source { return workstream{http: c} }

func (workstream) Provider() string { return "workstream" }

const (
	workstreamBaseURL = "https://www.workstream.us"
	// workstreamMaxPages caps the walk when the page states no totalPages. Ten cards a page puts
	// it well above the largest employer met live (86 postings); it is a backstop against a
	// markup change that keeps yielding new links, not an expected limit.
	workstreamMaxPages = 200
)

// workstreamBase is the site origin, parsed once so a listing href resolves against it.
var workstreamBase, _ = url.Parse(workstreamBaseURL)

// workstreamPostingIDPattern captures the posting id a career-site URL path ends with, and by
// matching the WHOLE path it also rejects the brand, location and apply links a listing page
// carries alongside its postings. The id is the trailing hex: the slug in front of it is built
// from the job title and moves whenever the employer edits the title.
var workstreamPostingIDPattern = regexp.MustCompile(
	`^/j/[0-9a-f]{8}/[^/]+/[^/]+/[^/]+-([0-9a-f]{8})$`)

// workstreamPostingID extracts the native posting id from a posting URL, "" when the URL is not
// a posting page. It matches the parsed PATH, so an absolute link and a root-relative one both
// resolve and the "?locale=en" every rendered link carries never reaches the pattern.
func workstreamPostingID(loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	return firstSubmatch(workstreamPostingIDPattern, u.Path)
}

// workstreamSearchBasePattern captures the positions-listing URL a career-site page states for
// itself. Every page of the site carries it, and it is the only thing that says where a
// single-brand employer's positions live once "/j/<board>/positions" has redirected away.
var workstreamSearchBasePattern = regexp.MustCompile(`var searchBaseUrl = '([^']+)'`)

// workstreamTotalPagesPattern captures the listing's page count, stated alongside searchBaseUrl.
var workstreamTotalPagesPattern = regexp.MustCompile(`var totalPages = (\d+)`)

// workstreamPosting is one card of a positions listing: everything the platform states about a
// posting except its body, which only the posting page carries.
type workstreamPosting struct {
	id  string
	url string
	// title, location and employmentType are the card's own fields; location is the store's
	// full street address, which is the only place the listing states one.
	title          string
	location       string
	employmentType string
	// pay is the rendered pay line ("$15.00 - 18.00 per hour", "Starting at $11.00 per hour"),
	// empty when the employer states none. It is stated in the description, never parsed into
	// the structured salary fields — see the type comment.
	pay string
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (s workstream) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p workstreamPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have — seen
// reports whether an id is already ingested. A seen posting yields its list-only job flagged
// SeenRefresh, so the pipeline refreshes liveness without spending a request or overwriting the
// body hydrated when the posting was new. The title travels with that refresh on purpose: the
// refresh path re-applies the catalogue filter to it, which is how a stored posting the
// dictionary now turns away ages out instead of being kept alive by its own re-listing.
func (s workstream) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p workstreamPosting) (Job, bool) {
		if seen(p.id) {
			base := p.job(e, "")
			base.SeenRefresh = true
			return base, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// list walks a board's positions listing and returns every posting it advertises. The FIRST page
// failing is a board-level error; a later page failing ends the walk with what was gathered, so a
// mid-listing hiccup costs a page rather than the board. It restates the shared crawlPagedLinks
// loop because it needs each card's whole row — title, address, pay and employment type — and not
// just its link, and because page 1 is already in hand from resolving the listing URL.
func (s workstream) list(ctx context.Context, e CompanyEntry) ([]workstreamPosting, error) {
	root, base, err := s.listing(ctx, e)
	if err != nil {
		return nil, err
	}
	pages := workstreamTotalPages(root)

	var out []workstreamPosting
	listed := make(map[string]bool)
	for page := 1; page <= pages; page++ {
		if page > 1 {
			root, err = s.http.GetHTML(ctx, workstreamPageURL(base, page))
			if err != nil {
				break // a later page failing just ends pagination; page 1's postings still ingest
			}
		}
		added := 0
		for _, p := range workstreamListing(root) {
			if listed[p.id] {
				continue
			}
			listed[p.id] = true
			out = append(out, p)
			added++
		}
		if added == 0 {
			break // an empty page: the listing is exhausted
		}
	}
	return out, nil
}

// listing fetches the board's positions listing and returns its first page along with the URL to
// page the rest from. It asks for the employer-wide listing and then believes the page's own
// searchBaseUrl: for a multi-brand employer that names the URL just fetched, so the page in hand
// IS page 1; for a single-brand one the request was redirected to the brand root and searchBaseUrl
// names the brand's positions listing, which costs one more request. A page stating no
// searchBaseUrl is not a career site, and saying so beats paging an unrelated document.
func (s workstream) listing(ctx context.Context, e CompanyEntry) (*html.Node, *url.URL, error) {
	asked := workstreamPositionsURL(e.Board)
	root, err := s.http.GetHTML(ctx, asked)
	if err != nil {
		return nil, nil, fmt.Errorf("workstream: listing %s: %w", e.Board, err)
	}
	stated := firstSubmatch(workstreamSearchBasePattern, workstreamScripts(root))
	if stated == "" {
		return nil, nil, fmt.Errorf("workstream: listing %s: no positions listing on the page", e.Board)
	}
	// Resolved rather than believed verbatim: the site states an absolute URL today, and
	// resolving costs nothing while leaving a relative one still fetchable.
	ref, err := url.Parse(stated)
	if err != nil {
		return nil, nil, fmt.Errorf("workstream: listing %s: positions listing %q: %w", e.Board, stated, err)
	}
	base := workstreamBase.ResolveReference(ref)
	if base.String() == asked {
		return root, base, nil
	}
	root, err = s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, nil, fmt.Errorf("workstream: listing %s: %w", e.Board, err)
	}
	return root, base, nil
}

// workstreamTotalPages reads the page count a listing states for itself, falling back to
// workstreamMaxPages when the page states none or states one this crawl will not follow. The
// stated count is exact — the last page is short and the page after it renders no card — so it
// is what ends the walk, with the added==0 rule as the second guard.
func workstreamTotalPages(root *html.Node) int {
	n, err := strconv.Atoi(firstSubmatch(workstreamTotalPagesPattern, workstreamScripts(root)))
	if err != nil || n < 1 || n > workstreamMaxPages {
		return workstreamMaxPages
	}
	return n
}

// workstreamPositionsURL builds a board's employer-wide positions listing URL.
func workstreamPositionsURL(board string) string {
	return fmt.Sprintf("%s/j/%s/positions", workstreamBaseURL, url.PathEscape(board))
}

// workstreamPageURL builds one page of a positions listing, SETTING rather than appending the
// page parameter so a listing URL that ever carries a query of its own stays well-formed.
func workstreamPageURL(base *url.URL, page int) string {
	u := *base
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.String()
}

// detail fetches one posting page and returns the listing card's job with its body filled in.
// It reports ok=false — so the caller skips just this posting — when the page cannot be fetched
// OR when it yields no body at all. Deferring a posting by one crawl is recoverable; storing it
// body-less is not, because a stored row is `seen` and so is never hydrated again once it ages
// past the pipeline's hydration-retry window, leaving a posting no search can reach.
func (s workstream) detail(ctx context.Context, e CompanyEntry, p workstreamPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	body := workstreamDescription(root)
	if body == "" {
		return Job{}, false
	}
	return p.job(e, body), true
}

// job maps a listing card plus the body read from its posting page onto a Job. The employer is
// the configured entry's: a board is one Workstream account, whatever brands it trades under.
// A body-less job is a liveness refresh, which writes no content, so the pay line joins the
// description only when there is a description for it to join.
func (p workstreamPosting) job(e CompanyEntry, description string) Job {
	if description != "" {
		description += p.payParagraph()
	}
	return Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          p.title,
		Company:        e.Company,
		Location:       p.location,
		Description:    description,
		Remote:         isRemote(p.location),
		EmploymentType: workstreamEmploymentType(p.employmentType),
		PostedAt:       nil, // neither page states one
	}
}

// payParagraph states the card's pay line at the end of the body, "" when the employer states
// none. It is quoted verbatim rather than parsed into SalaryMin/Max: the grammar is fixed (the
// platform renders its own compensation field — "$15.00 - 18.00 per hour", "Starting at $11.00
// per hour", "Up to $22.00 per hour") but the "$" is unqualified, and Workstream serves Canadian
// employers beside US ones, so nothing on either page says which dollar it is. Publishing a
// currency the platform never stated is the guess the structured-facet contract forbids.
func (p workstreamPosting) payParagraph() string {
	pay := strings.TrimSpace(p.pay)
	if pay == "" {
		return ""
	}
	return sanitizeHTML("<p>Pay: " + pay + "</p>")
}

// workstreamDescription returns the posting body: the one rich-text block the posting page
// renders. Everything else on the page — the store's address, the map, the share controls — is
// chrome, and the pay line is not on this page at all (job adds it from the listing card).
func workstreamDescription(root *html.Node) string {
	rich := firstByClass(root, "position-rich-text-content")
	if rich == nil {
		return ""
	}
	return sanitizeHTML(innerHTML(rich))
}

// workstreamListing extracts one listing page's postings in document order, skipping any card
// whose link carries no posting id.
func workstreamListing(root *html.Node) []workstreamPosting {
	var out []workstreamPosting
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !hasClass(n, "position-card") {
			return true
		}
		if p, ok := workstreamCard(n); ok {
			out = append(out, p)
		}
		return false // a card never nests another one
	})
	return out
}

// workstreamCard reads one listing card, reporting ok=false when it links no posting.
func workstreamCard(card *html.Node) (workstreamPosting, bool) {
	var p workstreamPosting
	var tags []string
	walk(card, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case n.Data == "a" && p.id == "":
			if id := workstreamPostingID(attr(n, "href")); id != "" {
				p.id, p.url = id, workstreamCanonicalURL(attr(n, "href"))
				p.title = workstreamText(n)
			}
		case hasClass(n, "position-address"):
			p.location = workstreamText(n)
		case hasClass(n, "tag"):
			tags = append(tags, workstreamText(n))
		case n.Data == "img" && attr(n, "data-icon") == "rate-of-pay":
			// The pay line is the sibling of its icon, so the icon's parent holds both.
			p.pay = workstreamText(n.Parent)
		}
		return true
	})
	if p.id == "" {
		return workstreamPosting{}, false
	}
	// A posting may offer several schedules at once ("Full-time" AND "Part-time"), which states
	// no single employment type, so only a lone tag is read — otherwise the first one would win
	// by accident and the description parser would never get to decide.
	if len(tags) == 1 {
		p.employmentType = tags[0]
	}
	return p, true
}

// workstreamCanonicalURL resolves a listing href to the posting's absolute address, dropping the
// "?locale=en" every rendered link carries so the stored URL is the posting's own rather than one
// pinned to the language this crawl asked for.
func workstreamCanonicalURL(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	return workstreamBase.ResolveReference(&url.URL{Path: u.Path}).String()
}

// workstreamText is textContent with runs of whitespace collapsed; the site indents its markup
// across several source lines, so a raw concatenation carries newlines mid-value.
func workstreamText(n *html.Node) string {
	return strings.Join(strings.Fields(textContent(n)), " ")
}

// workstreamScripts concatenates the page's inline script text, where the site states its
// listing URL and page count. They live in an unnamed <script> block, so there is no id to
// select and the blocks are read together.
func workstreamScripts(root *html.Node) string {
	var b strings.Builder
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "script" {
			b.WriteString(textContent(n))
			b.WriteByte('\n')
		}
		return true
	})
	return b.String()
}

// workstreamEmploymentType maps a card's schedule tag onto the freehire vocabulary. Live crawls
// rendered exactly two tags; anything else maps to "" so the description parser decides, per the
// structured-signal-only rule.
func workstreamEmploymentType(tag string) string {
	switch tag {
	case "Full-time":
		return "full_time"
	case "Part-time":
		return "part_time"
	default:
		return ""
	}
}
