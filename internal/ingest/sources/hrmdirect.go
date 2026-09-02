package sources

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// hrmdirect adapts HRM Direct career sites. The board is the tenant subdomain, so the career
// site is "<board>.hrmdirect.com".
//
// The listing lives at "/employment/job-openings.php?search=true". The bare path renders the
// filter form above an empty result ("Select options from the menus above and click Search"),
// and "search=true" with no department/city/state is the site's own "all openings" query: it
// server-renders the WHOLE board in one page, with no pagination control anywhere (measured
// live over 918 tenants — the largest board answered 1,480 postings, the heaviest page ran to
// 880 KB, and no tenant carried a next-page link). So the listing is a single request and there
// is no paged walk.
//
// The listing's latency is erratic and that is the platform, not a rate limit. Sequential
// fetches of the same 50 KB page, spaced and unloaded, measured 0.7-0.9s for most of a run of
// fifteen and 5.8s, 22.7s, 46.8s and 50.9s for the rest — the query behind it is a database
// search the origin sometimes stalls on. So a slice of boards will trip the shared client's
// 15s timeout on any given crawl and land in board_health; a following crawl clears it, and
// nothing here is paced, because spacing requests does not make a stalled origin answer.
//
// A posting is keyed by the PAIR (req, req_loc), never by req alone: one requisition open in
// several locations is listed once per location, each row linking its own page, so req alone
// would collide on the (source, external_id) dedup key and the locations would overwrite each
// other. The job page carries the title in an h2, a label/value table (Department, Location,
// plus whatever the tenant adds) and the body in a div.jobDesc; it exposes no ld+json, no
// publish date and no structured remote flag, so the description comes from a per-job detail
// fetch and remote falls back to the location text.
type hrmdirect struct {
	http hrmdirectHTTP
}

// hrmdirectHTTP is the transport this adapter needs. The two halves of a crawl are read
// differently on purpose. A job page is read as RAW BYTES so its character set can be settled
// before parsing (see detail); the listing goes through the ordinary DOM path, so its
// windows-1252 punctuation does reach it as U+FFFD. That is deliberate. The listing supplies
// hrefs, which are ASCII, and the row titles — and a title from here is never stored, only
// weighed by the catalogue's non-technical dictionary on a liveness refresh, which matches
// whole words and cannot be swayed by a replaced apostrophe. Reading the listing as bytes
// would instead put GetText's silently-truncating 2 MiB cap over a page already measured at
// 880 KB, and a truncated listing on a per-board-swept provider closes the postings it never
// reached.
type hrmdirectHTTP interface {
	HTMLGetter
	TextGetter
}

// NewHRMDirect builds the HRM Direct adapter over the given HTTP client.
func NewHRMDirect(c hrmdirectHTTP) Source { return hrmdirect{http: c} }

func (hrmdirect) Provider() string { return "hrmdirect" }

func (s hrmdirect) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p hrmdirectPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew is the hydrating crawl: it enumerates the whole board, but fetches a posting's page
// only for an id the catalogue does not already have. A seen posting is emitted as a liveness
// refresh (identity only, no detail request, no content rewrite); an unseen one is hydrated.
//
// The listing already proves a posting is live, and the page adds only the description — which
// does not change once written. Measured over the 918 candidate tenants this file was harvested
// from, the fleet lists ~38.7k live postings, so the list-only Fetch would spend ~38.7k page
// fetches per cycle to discover the handful that are genuinely new. That is the same waste
// teamtailor's FetchNew was added for, at the same order of magnitude.
//
// It does not go to zero: a posting the pipeline's non-technical gate discards is never stored,
// so it is never seen and its page is bought again every run. That is the pipeline's decision to
// make, and it needs the description to make it — the same limit every hydrating adapter has.
func (s hrmdirect) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p hrmdirectPosting) (Job, bool) {
		if seen(p.externalID) {
			// What the listing itself states, and nothing more: the pipeline resolves the
			// stored row from the identity and must not re-upsert content, which would wipe
			// the description and the facets derived from it. The title travels because a
			// refresh faces the catalogue filter, and that is how a stored row ages out when
			// the non-technical dictionary later grows to cover its title.
			return Job{
				ExternalID:  p.externalID,
				URL:         p.url,
				Title:       p.title,
				Company:     e.Company,
				SeenRefresh: true,
			}, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// hrmdirectPosting is one row of the listing: a posting's identity, the title the row states,
// and the page that holds its body. Nothing else survives the listing — the row's own
// department/city/state columns are exactly what the job page restates, so they are read there.
type hrmdirectPosting struct {
	externalID string
	title      string
	url        string
}

// list enumerates the board's live postings from its single listing page.
func (s hrmdirect) list(ctx context.Context, e CompanyEntry) ([]hrmdirectPosting, error) {
	// base carries the scheme, host and the /employment/ directory the listing's relative
	// "job-opening.php?..." hrefs resolve against.
	base, err := url.Parse(fmt.Sprintf("https://%s.hrmdirect.com/employment/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("hrmdirect: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String()+"job-openings.php?search=true")
	if err != nil {
		return nil, fmt.Errorf("hrmdirect: listing %s: %w", e.Board, err)
	}

	return hrmdirectPostings(base, root), nil
}

// hrmdirectPostings reads the listing's posting rows, first-seen order, one per (req, req_loc).
// It walks the anchors itself rather than calling jobLinks because the row's link TEXT is the
// posting title, which the liveness refresh carries (see FetchNew).
func hrmdirectPostings(base *url.URL, root *html.Node) []hrmdirectPosting {
	var out []hrmdirectPosting
	emitted := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		req, loc, ok := hrmdirectRef(attr(n, "href"))
		if !ok {
			return true
		}
		id := req + "-" + loc
		if emitted[id] {
			return true
		}
		emitted[id] = true
		// The stored URL is rebuilt from the ids rather than taken from the href: the site
		// links its own postings with a trailing empty parameter and a "#job" fragment
		// ("job-opening.php?req=1&req_loc=2&&#job"), which is noise in the catalogue and in
		// the content hash.
		out = append(out, hrmdirectPosting{
			externalID: id,
			title:      textContent(n),
			url:        fmt.Sprintf("%sjob-opening.php?req=%s&req_loc=%s", base, req, loc),
		})
		return true
	})
	return out
}

// detail fetches one job page and maps it to a Job, returning ok=false when the page fetch
// fails or it carries no posting block, so the caller skips just that posting.
func (s hrmdirect) detail(ctx context.Context, e CompanyEntry, p hrmdirectPosting) (Job, bool) {
	body, err := s.http.GetText(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	// The pages declare no character set — not in the Content-Type header, not in a meta
	// element — and are served as windows-1252, which is exactly the case charset.NewReader
	// settles the way the HTML standard does: a declared charset wins, an undeclared body
	// whose first kilobyte carries valid non-ASCII UTF-8 is taken as UTF-8, and everything
	// else falls back to windows-1252. Handing the raw bytes to the parser instead turns
	// every smart quote and em dash a posting was pasted in with into U+FFFD, and most
	// postings are pasted in from a word processor. Measured over 67 tenants: 56 windows-1252
	// bodies, 11 pure ASCII, and not one page the fallback would decode wrongly.
	decoded, err := charset.NewReader(strings.NewReader(body), "")
	if err != nil {
		return Job{}, false
	}
	root, err := html.Parse(decoded)
	if err != nil {
		return Job{}, false
	}
	// Everything is read from inside the posting block: the pages wrap it in a tenant-authored
	// welcome blurb that is free-form CMS HTML and may carry headings of its own.
	posting := firstByClass(root, "reqResult")
	if posting == nil {
		return Job{}, false
	}
	title := firstElementText(posting, "h2")
	if title == "" {
		return Job{}, false
	}

	location := hrmdirectLocation(hrmdirectField(posting, "Location"))
	remote := isRemote(location)
	return Job{
		ExternalID:  p.externalID,
		URL:         p.url,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(elementInnerHTMLByClass(posting, "div", "jobDesc")),
		// The platform states no work arrangement of its own, so the location text is the
		// only remote signal (never the title, which false-positives on "Remote …" roles).
		Remote:   remote,
		WorkMode: workModeFromRemote(remote),
		PostedAt: nil, // HRM Direct job pages carry no publish date
	}, true
}

// hrmdirectPostingID matches one of the two ids in a posting link. They are numeric, and
// requiring that is what keeps a link with an empty or non-numeric parameter from becoming a
// board entry with a URL nothing answers.
var hrmdirectPostingID = regexp.MustCompile(`^\d+$`)

// hrmdirectRef reads a job link's (req, req_loc) pair, reporting ok=false when the link is not
// a posting permalink (the listing also links the filter form and the site's own navigation).
// Both ids are required: a link carrying only req is the filter form's, and keying on req alone
// would merge a requisition's separate locations onto one dedup key. The query is decoded
// rather than pattern-matched, so a link that ever orders its parameters differently still
// resolves.
func hrmdirectRef(href string) (req, loc string, ok bool) {
	u, err := url.Parse(href)
	if err != nil || path.Base(u.Path) != "job-opening.php" {
		return "", "", false
	}
	q := u.Query()
	req, loc = q.Get("req"), q.Get("req_loc")
	if !hrmdirectPostingID.MatchString(req) || !hrmdirectPostingID.MatchString(loc) {
		return "", "", false
	}
	return req, loc, true
}

// hrmdirectField returns the value of the job page's label/value row carrying the given label
// (the labels are rendered with a trailing colon), or "" when the page has no such row. It is
// keyed by label rather than by position because the table is tenant-configurable: every page
// has Department and Location, and some add an Office or Job Category row between them.
func hrmdirectField(posting *html.Node, label string) string {
	var value string
	walk(posting, func(n *html.Node) bool {
		if value != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "td" || !hasClass(n, "viewFieldName") {
			return true
		}
		if strings.TrimSuffix(strings.TrimSpace(textContent(n)), ":") != label {
			return true
		}
		// The value is the row's next cell; anything else after the label means the page's
		// markup has moved and there is no value to read.
		for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
			if sib.Type != html.ElementNode {
				continue
			}
			if hasClass(sib, "viewFieldValue") {
				value = textContent(sib)
			}
			break
		}
		return true
	})
	return value
}

// hrmdirectLocation cleans the Location field, which the platform renders as "<city>, <state>"
// from two independently-optional columns: a posting with no city answers ", NY" and one with
// neither answers "". Dropping the empty parts keeps a stray comma out of the stored location.
func hrmdirectLocation(raw string) string {
	var parts []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}
