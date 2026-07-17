package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// catsone adapts CATS (catsone.com) career portals. The board is the careers host — a
// tenant subdomain (<board>.catsone.com) or a custom domain mapped to CATS (e.g.
// jobs.<company>.com). GET /careers redirects to the portal (/careers/<portalId>) whose
// server-rendered table lists every job as an <a class="table-row"> carrying the title
// (.title-cell) and location (data-label="Location") in cells; the
// /careers/<portalId>/jobs/<id> detail page holds the description (.job-description).
// There is no pagination — the portal table lists all open jobs.
type catsone struct {
	http HTMLGetter
}

// NewCatsone builds the CATS adapter over the given HTML client.
func NewCatsone(c HTMLGetter) Source { return catsone{http: c} }

func (catsone) Provider() string { return "catsone" }

// catsoneListing is one job row read from the portal table: its detail URL plus the title
// and location the row cells carry (so only the description needs a detail fetch).
type catsoneListing struct {
	URL      string
	Title    string
	Location string
}

func (s catsone) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s/careers", e.Board))
	if err != nil {
		return nil, fmt.Errorf("catsone: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("catsone: listing %s: %w", e.Board, err)
	}
	cards := catsoneListings(base, root)
	return fetchDetails(cards, defaultDetailWorkers, func(c catsoneListing) (Job, bool) {
		return s.detail(ctx, e, c)
	}), nil
}

// detail fetches one job's detail page for its description, returning ok=false when the
// fetch fails or the URL carries no native id so the caller skips just that posting.
func (s catsone) detail(ctx context.Context, e CompanyEntry, c catsoneListing) (Job, bool) {
	id := catsoneJobID(c.URL)
	if id == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, c.URL)
	if err != nil {
		return Job{}, false
	}
	description := ""
	if body := firstByClass(root, "job-description"); body != nil {
		description = sanitizeHTML(innerHTML(body))
	}
	return Job{
		ExternalID:  id,
		URL:         c.URL,
		Title:       c.Title,
		Company:     e.Company,
		Location:    c.Location,
		Description: description,
		Remote:      isRemote(c.Location),
	}, true
}

// catsoneJobIDPattern captures the numeric job id from a /careers/<portalId>/jobs/<id> URL.
var catsoneJobIDPattern = regexp.MustCompile(`/careers/\d+/jobs/(\d+)`)

// catsoneJobID extracts the native numeric job id from a detail URL, or "" when the URL is
// not a job posting.
func catsoneJobID(loc string) string {
	return firstSubmatch(catsoneJobIDPattern, loc)
}

// catsoneListings parses each job row from the portal table into its absolute detail URL,
// title (.title-cell), and location (data-label="Location"), deduplicated by URL.
func catsoneListings(base *url.URL, root *html.Node) []catsoneListing {
	var out []catsoneListing
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" || !hasClass(n, "table-row") {
			return true
		}
		href := attr(n, "href")
		if catsoneJobID(href) == "" {
			return true
		}
		ref, err := url.Parse(href)
		if err != nil {
			return true
		}
		abs := base.ResolveReference(ref).String()
		if _, ok := seen[abs]; ok {
			return true
		}
		seen[abs] = struct{}{}
		out = append(out, catsoneListing{
			URL:      abs,
			Title:    nodeText(firstByClass(n, "title-cell")),
			Location: nodeText(firstByAttr(n, "data-label", "Location")),
		})
		return true
	})
	return out
}

// nodeText is textContent guarded against a nil node (a missing cell yields "").
func nodeText(n *html.Node) string {
	if n == nil {
		return ""
	}
	return textContent(n)
}

// firstByAttr returns the first element whose named attribute equals val, or nil.
func firstByAttr(root *html.Node, key, val string) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == html.ElementNode && attr(n, key) == val {
			found = n
			return false
		}
		return true
	})
	return found
}
