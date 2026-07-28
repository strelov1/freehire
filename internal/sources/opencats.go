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

// opencatsListingQuery is the portal's "show all" route, appended to the board root.
const opencatsListingQuery = "index.php?m=careers&p=showAll"

// detail fetches one posting's page for the fields the listing cannot be trusted to carry,
// returning ok=false when the fetch fails so the caller skips just that posting. Location comes
// from the labelled details table rather than a listing column: column order and count differ
// per install, but the label does not.
func (s opencats) detail(ctx context.Context, e CompanyEntry, c opencatsListing) (Job, bool) {
	root, err := s.http.GetHTML(ctx, c.URL)
	if err != nil {
		return Job{}, false
	}
	location := opencatsLabelledField(root, "location")
	description := ""
	if body := firstByAttr(root, "id", "descriptive"); body != nil {
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
// this reads the two things a rewrite cannot change without breaking the portal itself: the
// showJob route, which carries the id, and the anchor text, which is the title. Postings are
// de-duplicated by id, keeping the first anchor, because a listing commonly links one posting
// twice (its title and an apply button) and the title link comes first.
func opencatsListings(base *url.URL, root *html.Node) []opencatsListing {
	var out []opencatsListing
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := attr(n, "href")
		id := opencatsJobID(href)
		if id == "" {
			return true
		}
		if _, ok := seen[id]; ok {
			return true
		}
		title := strings.TrimSpace(textContent(n))
		if opencatsIsGeneralApplication(title) {
			return true
		}
		ref, err := url.Parse(href)
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
