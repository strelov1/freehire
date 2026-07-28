package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// opencats adapts a self-hosted OpenCATS career portal. The board is the portal root — a host,
// optionally followed by the path the portal is mounted under, since installs differ on that
// (atscareers.g4s.com serves it at the web root, careers.boomit.pt/careers nests it). The
// listing at index.php?m=careers&p=showAll links every open posting; the showJob detail page
// carries the labelled fields and the body. There is no pagination — the listing is complete.
type opencats struct {
	http HTMLGetter
}

// opencatsListingQuery is the portal's "show all" route, appended to the board root.
const opencatsListingQuery = "index.php?m=careers&p=showAll"

// NewOpencats builds the OpenCATS adapter over the given HTML client.
func NewOpencats(c HTMLGetter) Source { return opencats{http: c} }

func (opencats) Provider() string { return "opencats" }

func (s opencats) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse("https://" + strings.Trim(e.Board, "/") + "/")
	if err != nil {
		return nil, fmt.Errorf("opencats: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String()+opencatsListingQuery)
	if err != nil {
		return nil, fmt.Errorf("opencats: listing %s: %w", e.Board, err)
	}
	return fetchDetails(opencatsListings(base, root), defaultDetailWorkers, func(c opencatsListing) (Job, bool) {
		return s.detail(ctx, e, c)
	}), nil
}

// detail fetches one posting's page for the fields the listing cannot be trusted to carry,
// returning ok=false when the fetch fails so the caller skips just that posting. Location and
// body come from the detail page rather than a listing column because listing columns are
// positional and differ per install; the detail page names its fields.
func (s opencats) detail(ctx context.Context, e CompanyEntry, c opencatsListing) (Job, bool) {
	root, err := s.http.GetHTML(ctx, c.URL)
	if err != nil {
		return Job{}, false
	}
	location := opencatsLocation(root)
	description := ""
	if body := opencatsBody(root); body != nil {
		description = sanitizeHTML(innerHTML(body))
	}
	return Job{
		ExternalID:  c.ID,
		URL:         c.URL,
		Title:       c.Title,
		Company:     e.Company,
		Location:    location,
		Description: description,
		Remote:      isRemote(location),
	}, true
}

// opencatsBodyContainers name the elements installs put the posting body in, as an id or a
// class. The shipped template uses "descriptive"; installs that rebuilt the page rename it, and
// one carries the rename's typo (careers.crewlogix.com ships "job-decription"). This is a
// dictionary rather than a fuzzy match on purpose: an unknown container yields no description,
// which is visible and fixable, where guessing yields navigation chrome stored as a job body.
var opencatsBodyContainers = []string{
	"descriptive",
	"job-description",
	"job-decription",
	"description",
}

// opencatsBody returns the element holding the posting body, or nil when the install uses a
// container we do not know.
func opencatsBody(root *html.Node) *html.Node {
	for _, name := range opencatsBodyContainers {
		if n := firstByID(root, name); n != nil {
			return n
		}
		if n := firstByClass(root, name); n != nil {
			return n
		}
	}
	return nil
}

// opencatsLocation reads the posting's location, preferring the labelled field and falling back
// to the first row of the details table. The fallback exists because installs serve the portal
// in their own language (opencats.gorgany.com labels it "Місцезнаходження"), and while the label
// text is translated, the shipped template's row order is not: location is the first row.
func opencatsLocation(root *html.Node) string {
	if v := opencatsLabelledField(root, "location"); v != "" {
		return v
	}
	return opencatsFirstDetailValue(root)
}

// opencatsFirstDetailValue returns the value cell of the details table's first row, or "".
func opencatsFirstDetailValue(root *html.Node) string {
	table := firstByID(root, "detailsTable")
	if table == nil {
		return ""
	}
	var value string
	walk(table, func(n *html.Node) bool {
		if value != "" || n.Type != html.ElementNode || n.Data != "tr" {
			return true
		}
		if label := firstElementChild(n); label != nil {
			if next := nextElement(label); next != nil {
				value = strings.TrimSpace(textContent(next))
			}
		}
		return false
	})
	return value
}

// firstElementChild returns the node's first element child, skipping text nodes.
func firstElementChild(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

// opencatsLabelledField reads a value from the posting's details table by its label, matching
// case-insensitively on a substring so a template that renames "Location" to "Work Location"
// still resolves. The value is the next element cell after the label cell.
func opencatsLabelledField(root *html.Node, label string) string {
	var value string
	walk(root, func(n *html.Node) bool {
		if value != "" || n.Type != html.ElementNode || (n.Data != "td" && n.Data != "th") {
			return true
		}
		if !strings.Contains(strings.ToLower(textContent(n)), label) {
			return true
		}
		if next := nextElement(n); next != nil {
			value = strings.TrimSpace(textContent(next))
		}
		return true
	})
	return value
}

// opencatsGeneralApplicationMarkers name the talent-pool entries installs park in the portal
// alongside real openings ("Can't find what you're looking for? Apply here" is the one seen in
// the wild). They use the posting route but are a standing form, not a position. The markers
// are phrases rather than single words so a genuine role — an Application Security Engineer, a
// General Manager — is not swallowed with them.
var opencatsGeneralApplicationMarkers = []string{
	"apply here",
	"general application",
	"open application",
	"spontaneous application",
}

// opencatsIsGeneralApplication reports whether a posting title names a standing application
// form rather than an open position.
func opencatsIsGeneralApplication(title string) bool {
	t := strings.ToLower(title)
	for _, m := range opencatsGeneralApplicationMarkers {
		if strings.Contains(t, m) {
			return true
		}
	}
	return false
}

// nextElement returns the next sibling that is an element, skipping the whitespace text nodes a
// server-rendered table is full of.
func nextElement(n *html.Node) *html.Node {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode {
			return s
		}
	}
	return nil
}

// opencatsListing is one posting read from a portal listing: its absolute detail URL, the
// native numeric id, and the title carried by the anchor text.
type opencatsListing struct {
	URL   string
	ID    string
	Title string
}

// opencatsJobIDPattern captures the native posting id from a careers-route URL. The id is
// matched independently of parameter order, since a rewritten template may emit them in any
// sequence.
var opencatsJobIDPattern = regexp.MustCompile(`[?&]ID=(\d+)`)

// opencatsJobID extracts the native posting id from a portal URL, or "" when the URL is not a
// job posting. A URL only counts as a posting when it carries the showJob route: the portal
// reuses the same id on other routes (candidateRegistration, for one), which are actions on a
// posting rather than postings themselves.
func opencatsJobID(loc string) string {
	if !strings.Contains(loc, "p=showJob") {
		return ""
	}
	return firstSubmatch(opencatsJobIDPattern, loc)
}

// opencatsListings parses each posting from a portal listing. Installs customise the template
// freely — CSS classes, element types, column order, and column count differ between them — so
// this reads the one thing a rewrite cannot change without breaking the portal itself: the
// showJob route. Postings are de-duplicated by id, keeping the first carrier, because a
// listing commonly points at one posting twice (its title and an apply button) and the title
// comes first.
func opencatsListings(base *url.URL, root *html.Node) []opencatsListing {
	var out []opencatsListing
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		loc, title := opencatsRouteCarrier(n)
		id := opencatsJobID(loc)
		if id == "" {
			return true
		}
		if _, ok := seen[id]; ok {
			return true
		}
		if opencatsIsGeneralApplication(title) {
			return true
		}
		ref, err := url.Parse(loc)
		if err != nil {
			return true
		}
		seen[id] = struct{}{}
		out = append(out, opencatsListing{
			URL:   base.ResolveReference(ref).String(),
			ID:    id,
			Title: title,
		})
		return true
	})
	return out
}

// opencatsRouteCarrier returns the posting URL an element points at and the title that goes
// with it, or ("", "") when the element points at no posting. Two carriers exist in the wild:
// an anchor, whose own text is the title, and a clickable table row, which navigates from an
// onclick handler and has no anchor text — there the title is the row's first cell. Both are
// the same routing invariant; only the carrier differs.
func opencatsRouteCarrier(n *html.Node) (loc, title string) {
	if n.Data == "a" {
		return attr(n, "href"), strings.TrimSpace(textContent(n))
	}
	if target := opencatsClickTarget(attr(n, "onclick")); target != "" {
		return target, strings.TrimSpace(nodeText(firstElementChild(n)))
	}
	return "", ""
}

// opencatsClickTargetPattern captures the URL a row's onclick handler navigates to.
var opencatsClickTargetPattern = regexp.MustCompile(`location\.href\s*=\s*['"]([^'"]+)['"]`)

// opencatsClickTarget extracts the navigation target from an onclick handler, or "".
func opencatsClickTarget(onclick string) string {
	if onclick == "" {
		return ""
	}
	return firstSubmatch(opencatsClickTargetPattern, onclick)
}
