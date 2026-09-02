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

// gusto adapts Gusto Hiring (jobs.gusto.com), the hosted job board Gusto's payroll customers
// publish from. One board is one employer and its whole board file entry is the path segment of
// the board URL — "<company-slug>-<company-uuid>", as in
//
//	https://jobs.gusto.com/boards/grupo-ei-el-paso-7feb4b68-5288-41cc-b169-f56b3ec26120
//
// The whole segment is the id, not the uuid it ends with: the uuid alone and the slug alone both
// 404 (verified live). Everything is server-rendered HTML — there is no JSON endpoint, no
// embedded page state and no ld+json anywhere on either page — so the listing and the body are
// both read off the DOM.
//
// Traps, all verified live against the platform:
//
//   - Cloudflare answers a managed challenge (403) to Go's default TLS+HTTP/2 fingerprint on
//     every path, so this adapter is wired with the shared Chrome-fingerprint transport
//     (fingerprintHTTP) rather than the shared client — the bayt/gulftalent/meta/uber case.
//   - A RENAMED employer keeps its old slug resolving against the same uuid, so two spellings of
//     one board both answer 200 and list identical postings. The pipeline namespaces an external
//     id by BOARD, so a board file carrying both spellings would store every one of that
//     employer's postings twice. gustoBoardIdentity folds them for the board-file dedup, and
//     sources/gusto.yml carries one entry per company uuid.
//   - The listing serves 25 postings a page under ?page=N and renders "no open positions
//     currently" past the last one, so the walk ends on a page that adds no posting.
//   - The listing carries every field except the body, which only the posting page holds — hence
//     HydratingSource: a re-listed posting the catalogue already has costs no detail request.
//   - Neither page states a publish date in any form, so PostedAt is always nil.
type gusto struct {
	http gustoHTTP
}

// gustoHTTP is the transport gusto needs: the board listing and each posting page as HTML.
type gustoHTTP interface{ HTMLGetter }

// NewGusto builds the Gusto Hiring adapter over the given HTTP client (the shared
// Chrome-fingerprint fingerprintHTTP in production — see registry.go).
func NewGusto(c gustoHTTP) Source { return gusto{http: c} }

func (gusto) Provider() string { return "gusto" }

const (
	gustoBaseURL = "https://jobs.gusto.com"
	// gustoMaxPages caps the per-board pagination. The listing serves 25 postings a page and
	// the largest board seen live holds 35, so the cap sits far above anything the platform
	// produces; it is here so a markup change that keeps yielding "new" links cannot loop
	// unboundedly.
	gustoMaxPages = 40
)

// gustoBase is the board origin, parsed once so a listing href resolves against it.
var gustoBase, _ = url.Parse(gustoBaseURL)

// gustoPostingIDPattern captures the posting uuid a /postings/<slug>-<uuid> path ends with.
// The uuid is the identity worth keying on: the slug in front of it is built from the job
// title, so it moves whenever the employer edits the title while the uuid stays put.
var gustoPostingIDPattern = regexp.MustCompile(
	`/postings/[^/]+-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// gustoPostingID extracts the native posting id from a posting URL, "" when the URL is not a
// posting page. Any query string or fragment is stripped first, so a listing href carrying a
// tracking suffix still matches.
func gustoPostingID(loc string) string {
	return firstSubmatch(gustoPostingIDPattern, trimURLSuffix(loc))
}

// gustoBoardUUIDPattern captures the company uuid a board id ends with, tolerating the format
// suffix the board route ignores.
var gustoBoardUUIDPattern = regexp.MustCompile(
	`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:\.[a-z]+)?$`)

// gustoBoardIdentity folds every spelling of a board onto the company uuid it addresses — the
// entry sources.boardIdentity needs for gusto. Two spellings of one board both answer 200, and
// both were met live: a renamed employer keeps its OLD slug resolving against the same uuid, and
// a trailing ".json" is accepted because the route ignores the format suffix. Left unfolded they
// would be crawled as two boards, and since external ids are namespaced by board, every posting
// of that employer would be stored twice — the failure icimsHost exists for, met again. A board
// carrying no uuid folds to itself, so a malformed entry is never quietly merged into another.
func gustoBoardIdentity(board string) string {
	if uuid := firstSubmatch(gustoBoardUUIDPattern, board); uuid != "" {
		return uuid
	}
	return board
}

// gustoPosting is one row of a board listing: everything the platform states about a posting
// except its body, which only the posting page carries.
type gustoPosting struct {
	id       string
	url      string
	title    string
	location string
	// salary is the pay line as the board renders it ("$70,000 - $90,000 per year"), empty
	// when the employer states no pay; applySalary is what turns it into the structured fields.
	salary string
	// employmentType is Gusto's own label ("Full time"), not yet mapped onto the vocabulary.
	employmentType string
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (s gusto) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p gustoPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have — seen
// reports whether an id is already ingested. A seen posting yields its list-only job flagged
// SeenRefresh, so the pipeline refreshes liveness without spending a request or overwriting the
// body hydrated when the posting was new. The title travels with that refresh on purpose: the
// refresh path re-applies the catalogue filter to it, which is how a stored posting the
// dictionary now turns away ages out instead of being kept alive by its own re-listing.
func (s gusto) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p gustoPosting) (Job, bool) {
		if seen(p.id) {
			base := p.job(e, "")
			base.SeenRefresh = true
			return base, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// list walks a board's paginated listing and returns every posting it advertises. The FIRST page
// failing is a board-level error; a later page failing ends the walk with what was gathered, so a
// mid-listing hiccup costs a page rather than the board. It restates the shared crawlPagedLinks
// loop because it needs each item's whole listing row — title, location, pay and employment type
// — and not just its link.
func (s gusto) list(ctx context.Context, e CompanyEntry) ([]gustoPosting, error) {
	var out []gustoPosting
	listed := make(map[string]bool)
	for page := 1; page <= gustoMaxPages; page++ {
		root, err := s.http.GetHTML(ctx, gustoBoardURL(e.Board, page))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("gusto: listing board %s: %w", e.Board, err)
			}
			break // a later page failing just ends pagination; page 1's postings still ingest
		}
		added := 0
		for _, p := range gustoListing(root) {
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

// gustoBoardURL builds a board's listing URL for a 1-based page number.
func gustoBoardURL(board string, page int) string {
	return fmt.Sprintf("%s/boards/%s?page=%d", gustoBaseURL, url.PathEscape(board), page)
}

// detail fetches one posting page and returns the listing row's job with its body filled in.
// It reports ok=false — so the caller skips just this posting — when the page cannot be fetched
// OR when it yields no body at all. Deferring a posting by one crawl is recoverable; storing it
// body-less is not, because a stored row is `seen` and so is never hydrated again once it ages
// past the pipeline's hydration-retry window, leaving a posting no search can reach.
func (s gusto) detail(ctx context.Context, e CompanyEntry, p gustoPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	description := gustoDescription(root)
	if description == "" {
		return Job{}, false
	}
	return p.job(e, description), true
}

// job maps a listing row plus the body read from its posting page onto a Job. The employer is
// the configured entry's: a board is one company, and the name it renders is the same one.
func (p gustoPosting) job(e CompanyEntry, description string) Job {
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          p.title,
		Company:        e.Company,
		Location:       p.location,
		Description:    description,
		Remote:         isRemote(p.location),
		EmploymentType: gustoEmploymentType(p.employmentType),
		PostedAt:       nil, // neither page states one
	}
	p.applySalary(&j)
	return j
}

// gustoListing extracts one listing page's postings in document order, skipping any anchor whose
// href carries no posting uuid.
func gustoListing(root *html.Node) []gustoPosting {
	var out []gustoPosting
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := attr(n, "href")
		id := gustoPostingID(href)
		if id == "" {
			return true
		}
		p := gustoPosting{id: id, url: gustoAbsURL(href)}
		if h := firstByTag(n, "h3"); h != nil {
			p.title = gustoText(h)
		}
		// A listing row renders exactly two paragraphs: the location, then the
		// "<pay> · <employment type>" line whose pay half is dropped when the employer
		// states none.
		if ps := gustoParagraphs(n); len(ps) > 0 {
			p.location = gustoText(ps[0])
			if len(ps) > 1 {
				p.salary, p.employmentType = gustoMetaLine(gustoText(ps[1]))
			}
		}
		out = append(out, p)
		return false // a posting anchor never nests another one
	})
	return out
}

// gustoAbsURL resolves a listing href against the board origin. The board renders root-relative
// hrefs; ResolveReference also leaves an already-absolute one alone.
func gustoAbsURL(href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return gustoBaseURL + href
	}
	return gustoBase.ResolveReference(ref).String()
}

// gustoParagraphs returns the <p> elements under n in document order.
func gustoParagraphs(n *html.Node) []*html.Node {
	var out []*html.Node
	walk(n, func(c *html.Node) bool {
		if c.Type == html.ElementNode && c.Data == "p" {
			out = append(out, c)
		}
		return true
	})
	return out
}

// gustoText is textContent with runs of whitespace collapsed. The board indents its text nodes
// across several source lines, so the raw concatenation carries newlines and runs of spaces in
// the middle of a value.
func gustoText(n *html.Node) string {
	return strings.Join(strings.Fields(textContent(n)), " ")
}

// gustoMetaLine splits a listing row's second paragraph — "$70,000 - $90,000 per year · Full
// time", or just "Full time" when the employer states no pay — into the pay line and Gusto's
// employment-type label. Both are read from the END of the line, so a row that states no pay
// yields its employment type rather than filing that label as a salary.
func gustoMetaLine(line string) (salary, employmentType string) {
	parts := strings.Split(line, "·")
	if len(parts) > 1 {
		salary = strings.TrimSpace(parts[len(parts)-2])
	}
	return salary, strings.TrimSpace(parts[len(parts)-1])
}

// gustoEmploymentType maps Gusto's employment-type label onto the freehire vocabulary. A full
// live crawl of the platform rendered exactly four labels; anything else maps to "" so the
// description parser decides, per the structured-signal-only rule.
func gustoEmploymentType(label string) string {
	switch label {
	case "Full time":
		return "full_time"
	case "Part time":
		return "part_time"
	case "Contractor":
		return "contract"
	case "Intern":
		return "internship"
	default:
		return ""
	}
}

// gustoPayPattern matches the board's rendered pay line. Every pay line a full live crawl
// produced had exactly this shape: a two-sided range, thousands-separated, optionally
// fractional, and a period word.
var gustoPayPattern = regexp.MustCompile(`^\$([\d,]+(?:\.\d+)?) - \$([\d,]+(?:\.\d+)?) per ([a-z]+)$`)

// applySalary maps the board's rendered pay line onto the job's structured salary fields,
// writing nothing when the line is absent, is shaped differently, or states a period freehire
// has no value for ("per week", which two live postings used). The line is the platform's own
// compensation field rendered to a fixed grammar rather than free text, which is what makes
// reading it structured signal; anything that does not match that grammar is left to the
// enrichment pass rather than guessed at.
//
// The currency is stated rather than read off the "$": the board renders no currency code, and
// Gusto Payroll runs US employers only, so the glyph is never another country's dollar.
func (p gustoPosting) applySalary(j *Job) {
	m := gustoPayPattern.FindStringSubmatch(p.salary)
	if m == nil || !isSalaryPeriod(m[3]) {
		return
	}
	min, max := gustoPayPart(m[1]), gustoPayPart(m[2])
	if min == nil && max == nil {
		return
	}
	j.SalaryMin, j.SalaryMax = min, max
	j.SalaryCurrency, j.SalaryPeriod = "USD", m[3]
}

// gustoPayPart parses one bound of a rendered pay range ("70,000", "18.50").
func gustoPayPart(v string) *int {
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	if err != nil {
		return nil
	}
	return roundSalaryPart(f)
}

// gustoDescription assembles a posting's body: the summary the page opens with, followed by the
// "Description" section. The "About <employer>" section between them is boilerplate about the
// company rather than about the role, so it is left out — the same call rippling makes for its
// own company blurb.
func gustoDescription(root *html.Node) string {
	return sanitizeHTML(gustoSummary(root) + gustoSection(root, "Description"))
}

// gustoSummary returns the rendered HTML of the unheaded block the posting page opens with — its
// one-paragraph pitch. It is identified structurally, as the element following the <h1>, because
// the block carries nothing else to select it by; every section after it opens with its own
// heading, so a posting rendered without a summary is recognised by that heading rather than
// having its "About <employer>" section mistaken for one.
func gustoSummary(root *html.Node) string {
	h1 := firstByTag(root, "h1")
	if h1 == nil {
		return ""
	}
	next := nextElement(h1)
	if next == nil || firstByTag(next, "h3") != nil || firstByTag(next, "h4") != nil {
		return ""
	}
	return innerHTML(next)
}

// gustoSection returns the rendered rich-text HTML of the section whose heading reads exactly
// heading, or "" when the page carries no such section. The heading text is the only stable
// handle the markup offers: every section is the same <div> under the same utility classes, and
// only its heading says which one it is ("About <employer>", "Description", "Salary").
func gustoSection(root *html.Node, heading string) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type != html.ElementNode || (n.Data != "h3" && n.Data != "h4") {
			return true
		}
		if gustoText(n) != heading || n.Parent == nil {
			return true
		}
		if rich := firstByClass(n.Parent, "rich-text-container"); rich != nil {
			out = innerHTML(rich)
		}
		return true
	})
	return out
}
