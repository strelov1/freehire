package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// applicantstack adapts ApplicantStack career sites (<board>.applicantstack.com), a
// server-rendered ATS used mostly by small US employers. The board is the tenant subdomain,
// lower-cased (hosts are case-insensitive, and the same tenant is linked as both "M30" and
// "m30").
//
// What the platform serves:
//   - /x/openings is the whole listing: a table (#data-table) linking each posting as
//     /x/detail/<id>, 100 rows a page, paged with ?pge=N, and a pager stating
//     "Showing 1 - 100 of 282". A board with no openings renders the table with no rows and no
//     pager; a page past the end answers "Invalid page number" with no table; an unknown tenant
//     redirects to the vendor's marketing site, which carries no table either.
//   - The listing's columns are the tenant's own choice (Location, Department, Salary Range,
//     County - Territory …), so only the posting link and its title are read from it.
//   - The posting page carries a schema.org JobPosting ld+json: the body, datePosted and a
//     structured address. Its "title" is a separately configured field the platform often
//     leaves null, so the title is the listing's, which is what the page itself displays.
//   - A posting that is gone answers 404.
//   - robots.txt declares "Crawl-delay: 5" for every user agent and disallows nothing this
//     reads. Every tenant is its own host, so the delay is honoured per board: one request every
//     applicantstackCrawlDelay on that board's host, while other boards run alongside it.
//
// A steady-state crawl is one listing request per 100 postings plus a detail request only for a
// posting the catalogue has not seen (HydratingSource).
type applicantstack struct {
	http HTMLGetter
	// crawlDelay spaces the requests of one board's crawl; zero disables pacing (tests).
	crawlDelay time.Duration
}

// NewApplicantStack builds the ApplicantStack adapter over the given HTTP client.
func NewApplicantStack(c HTMLGetter) Source {
	return applicantstack{http: c, crawlDelay: applicantstackCrawlDelay}
}

func (applicantstack) Provider() string { return "applicantstack" }

// fullBoardListing: list pages the listing until the distinct postings read equal the total the
// pager states, and fails the crawl otherwise. See the fullBoardListing interface for the bar.
func (applicantstack) FullBoardListing() {}

const (
	// applicantstackCrawlDelay is robots.txt's Crawl-delay, applied per tenant host.
	applicantstackCrawlDelay = 5 * time.Second
	// applicantstackMaxPages bounds the listing walk (100 rows a page). It is a ceiling that
	// FAILS the crawl when reached before the stated total is read, never a silent end.
	applicantstackMaxPages = 50
)

// applicantstackPosting is one listing row: the posting's id and the title the row displays.
type applicantstackPosting struct {
	id    string
	title string
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (s applicantstack) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have. A seen
// posting yields the listing row alone, flagged SeenRefresh, so the pipeline refreshes liveness
// without spending a request or overwriting the stored body.
func (s applicantstack) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	get := pacedHTMLGetter(s.http, s.crawlDelay, 1)
	postings, err := applicantstackList(ctx, get, e.Board)
	if err != nil {
		return nil, fmt.Errorf("applicantstack: listing %s: %w", e.Board, err)
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p applicantstackPosting) (Job, bool) {
		if seen(p.id) {
			j := p.job(e)
			j.SeenRefresh = true
			return j, true
		}
		return applicantstackDetail(ctx, get, e, p)
	}), nil
}

// applicantstackList walks the board's listing to the total its pager states. Every way the walk
// can fall short of that total — a page that fails, a page that adds nothing, the total moving
// between pages, the page cap — fails the crawl rather than returning part of the board.
func applicantstackList(ctx context.Context, get HTMLGetter, board string) ([]applicantstackPosting, error) {
	var out []applicantstackPosting
	listed := map[string]bool{}
	want := -1
	for page := 1; page <= applicantstackMaxPages; page++ {
		root, err := get.GetHTML(ctx, applicantstackListingURL(board, page))
		if err != nil {
			return nil, err
		}
		rows, total, err := applicantstackListingPage(root)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if want < 0 {
			want = total
		} else if total != want {
			return nil, fmt.Errorf("page %d: total changed from %d to %d mid-walk", page, want, total)
		}
		added := 0
		for _, r := range rows {
			if !listed[r.id] {
				listed[r.id] = true
				out = append(out, r)
				added++
			}
		}
		if len(out) >= want {
			if len(out) != want {
				return nil, fmt.Errorf("read %d postings, the pager states %d", len(out), want)
			}
			return out, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("page %d added no posting at %d of %d", page, len(out), want)
		}
	}
	return nil, fmt.Errorf("stopped at the %d-page cap with %d of %d postings", applicantstackMaxPages, len(out), want)
}

// applicantstackListingPage reads one listing page: its posting rows and the total the pager
// states. A page without the openings table is not a listing (see the type comment) and is an
// error. A table with no pager is an empty board only when it also has no rows — rows without a
// stated total cannot be shown to be the whole board.
func applicantstackListingPage(root *html.Node) ([]applicantstackPosting, int, error) {
	table := firstByID(root, "data-table")
	if table == nil {
		return nil, 0, fmt.Errorf("no openings table")
	}
	var rows []applicantstackPosting
	walk(table, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		if id := applicantstackJobID(Attr(n, "href")); id != "" {
			rows = append(rows, applicantstackPosting{id: id, title: strings.Join(strings.Fields(textContent(n)), " ")})
		}
		return false
	})
	pager := firstByClass(root, "tablepager")
	if pager == nil {
		if len(rows) > 0 {
			return nil, 0, fmt.Errorf("%d rows but no pager stating a total", len(rows))
		}
		return nil, 0, nil
	}
	m := applicantstackTotalPattern.FindStringSubmatch(textContent(pager))
	if m == nil {
		return nil, 0, fmt.Errorf("unreadable pager %q", textContent(pager))
	}
	total, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, 0, fmt.Errorf("unreadable pager total %q: %w", m[1], err)
	}
	return rows, total, nil
}

// applicantstackTotalPattern reads the total from the pager's "Showing 1 - 100 of 282".
var applicantstackTotalPattern = regexp.MustCompile(`\bof\s+(\d+)`)

// applicantstackJobIDPattern captures the posting id from a /x/detail/<id> link. Ids are
// lower-case base-36 ("a23rqqe2fctl"); a tenant on its own domain may use a slug, so letters of
// either case, digits and hyphens are accepted.
var applicantstackJobIDPattern = regexp.MustCompile(`/x/detail/([A-Za-z0-9_-]+)`)

// applicantstackJobID extracts the native posting id from a posting link, or "".
func applicantstackJobID(href string) string {
	return firstSubmatch(applicantstackJobIDPattern, href)
}

// applicantstackDetail fetches one posting page and fills the listing row's job from its
// JobPosting ld+json. It reports ok=false — deferring just this posting to the next crawl — when
// the page cannot be read, carries no JobPosting, or yields no body: a posting stored body-less
// is `seen` from then on and never hydrated again.
func applicantstackDetail(ctx context.Context, get HTMLGetter, e CompanyEntry, p applicantstackPosting) (Job, bool) {
	root, err := get.GetHTML(ctx, applicantstackDetailURL(e.Board, p.id))
	if err != nil {
		return Job{}, false
	}
	var ld struct {
		Description string      `json:"description"`
		DatePosted  string      `json:"datePosted"`
		JobLocation schemaPlace `json:"jobLocation"`
	}
	if !ldJobPosting(root, &ld) {
		return Job{}, false
	}
	description := sanitizeHTML(ld.Description)
	if description == "" {
		return Job{}, false
	}
	j := p.job(e)
	j.Location = ld.JobLocation.Address.Location()
	j.Description = description
	// The platform states no work arrangement, so the location text is the only remote signal
	// (never the title, which false-positives on "Remote …" role names).
	j.Remote = isRemote(j.Location)
	j.PostedAt = parseRFC3339(ld.DatePosted)
	return j, true
}

// job maps a listing row onto the identity every path shares — the shape a SeenRefresh carries
// and the base a hydrated posting fills in. The employer is the configured entry's: a board is
// one tenant, and the ld+json's organisation name is the tenant's legal spelling.
func (p applicantstackPosting) job(e CompanyEntry) Job {
	return Job{
		ExternalID: p.id,
		URL:        applicantstackDetailURL(e.Board, p.id),
		Title:      p.title,
		Company:    e.Company,
	}
}

// applicantstackListingURL is the board's listing page; page 1 is the bare /x/openings.
func applicantstackListingURL(board string, page int) string {
	u := fmt.Sprintf("https://%s.applicantstack.com/x/openings", board)
	if page > 1 {
		u += fmt.Sprintf("?pge=%d", page)
	}
	return u
}

// applicantstackDetailURL is a posting's page, which is also its public permalink.
func applicantstackDetailURL(board, id string) string {
	return fmt.Sprintf("https://%s.applicantstack.com/x/detail/%s", board, id)
}
