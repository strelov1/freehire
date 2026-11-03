package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// umantis adapts Abacus Umantis career sites (SwissHR's applicant-tracking product). The board
// is the whole careers HOST, because the same application answers under the vendor's own
// domain (recruitingapp-<n>.umantis.com) and under each customer's vanity domain
// (jobs.bobst.com), and nothing in a bare tenant number says which. The listing is
// https://<board>/Jobs/All.
//
// The listing is server-rendered and paginated by a tokenised link: the page announces the
// next one in a data-pagination-next-href attribute (a relative "?tc<table>=p2&_search_token…"
// query that needs no session cookie), and the last page simply omits it. The adapter follows
// that link rather than building page URLs, since the table number and token are the site's.
//
// Each row links its posting at /Vacancies/<id>/Description/<lang>. The trailing number is the
// language the link was rendered for, not part of the posting, so the id is the vacancy number
// alone. The listing's columns are configured per customer (and labelled in the site's
// language), so only the title and the location/date elements Umantis keys by fixed element
// ids are read from it; a site that does not show a location column yields a posting without
// one rather than a guessed place. The description is the posting page's own body: those pages
// are customer-built templates with no schema.org JobPosting block, so the whole page content,
// minus its scripts, styles and chrome, is taken and sanitized.
type umantis struct {
	http HTMLGetter
}

// NewUmantis builds the Umantis adapter over the given HTTP client.
func NewUmantis(c HTMLGetter) Source { return umantis{http: c} }

func (umantis) Provider() string { return "umantis" }

// umantisMaxPages bounds the listing walk. Sites list 10-25 postings a page, so the cap is a
// guard against a pagination link that never ends, far above any real board.
const umantisMaxPages = 200

// umRow is one posting a listing row links.
type umRow struct {
	id, url, title, location string
	postedAt                 *time.Time
}

func (s umantis) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if e.Board == "" || strings.ContainsAny(e.Board, "/?# ") {
		return nil, fmt.Errorf("umantis: board %q is not a careers host", e.Board)
	}
	next := fmt.Sprintf("https://%s/Jobs/All", e.Board)
	var rows []umRow
	seen := make(map[string]bool)
	for page := 1; next != "" && page <= umantisMaxPages; page++ {
		base, err := url.Parse(next)
		if err != nil {
			return nil, fmt.Errorf("umantis: board %q: %w", e.Board, err)
		}
		root, err := s.http.GetHTML(ctx, next)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("umantis: listing %s: %w", e.Board, err)
			}
			break // a later page failing ends the walk with the postings gathered so far
		}
		added := 0
		for _, r := range umRows(base, root) {
			if !seen[r.id] {
				seen[r.id] = true
				rows = append(rows, r)
				added++
			}
		}
		if added == 0 {
			break // an empty page, or one repeating the last: the natural end
		}
		next = umNextPage(base, root)
	}
	return fetchDetails(rows, defaultDetailWorkers, func(r umRow) (Job, bool) {
		return s.detail(ctx, e, r)
	}), nil
}

// detail fetches one posting page and maps it to a Job, returning ok=false when the page
// cannot be read or carries no content, so the caller skips just that posting.
func (s umantis) detail(ctx context.Context, e CompanyEntry, r umRow) (Job, bool) {
	root, err := s.http.GetHTML(ctx, r.url)
	if err != nil {
		return Job{}, false
	}
	desc := sanitizeHTML(umBody(root))
	if desc == "" {
		return Job{}, false
	}
	remote := isRemote(r.location)
	return Job{
		ExternalID:  r.id,
		URL:         r.url,
		Title:       r.title,
		Company:     e.Company,
		Location:    r.location,
		Description: desc,
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		PostedAt:    r.postedAt,
	}, true
}

// umVacancyPattern captures the vacancy id from a posting link's path; the trailing number is
// the rendering language and is deliberately not part of the id.
var umVacancyPattern = regexp.MustCompile(`^/Vacancies/(\d+)/Description/\d+`)

// umLocationElements are the listing's fixed element ids that Umantis templates use for the
// place, in preference order: the "place" column most sites show, then the postal-code-and-town
// column of the newer table layout.
var umLocationElements = []string{"tableaslist_element_1152495", "tableaslist_element_1184124"}

// umDateElement is the element id of the "Online since: dd.mm.yyyy" column.
const umDateElement = "tableaslist_element_1152487"

var (
	umDatePattern  = regexp.MustCompile(`(\d{2})\.(\d{2})\.(\d{4})`)
	umLabelPattern = regexp.MustCompile(`^[^:\d]{1,30}:\s*`)
)

// umRows returns the postings a listing page links, resolved against base, with the title,
// location and date of their row.
func umRows(base *url.URL, root *html.Node) []umRow {
	var out []umRow
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		ref, err := url.Parse(Attr(n, "href"))
		if err != nil {
			return true
		}
		id := firstSubmatch(umVacancyPattern, ref.Path)
		if id == "" {
			return true
		}
		abs := base.ResolveReference(ref)
		abs.RawQuery, abs.Fragment = "", ""
		row := umEnclosingRow(n)
		r := umRow{id: id, url: abs.String(), title: strings.TrimSpace(textContent(n))}
		if row != nil {
			for _, class := range umLocationElements {
				if loc := umCell(row, class); loc != "" {
					r.location = umLabelPattern.ReplaceAllString(loc, "")
					break
				}
			}
			if m := umDatePattern.FindStringSubmatch(umCell(row, umDateElement)); m != nil {
				r.postedAt = parseDate(m[3] + "-" + m[2] + "-" + m[1])
			}
		}
		if r.title != "" {
			out = append(out, r)
		}
		return true
	})
	return out
}

// umEnclosingRow returns the <tr> holding n, or nil.
func umEnclosingRow(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "tr" {
			return p
		}
	}
	return nil
}

// umCell returns the text of the first element under row carrying the class, minus the
// column separator Umantis prefixes to it.
func umCell(row *html.Node, class string) string {
	var text string
	walk(row, func(n *html.Node) bool {
		if text != "" {
			return false
		}
		if n.Type == html.ElementNode && HasClass(n, class) {
			text = strings.Trim(strings.Map(func(r rune) rune {
				if r == ' ' {
					return ' '
				}
				return r
			}, textContent(n)), " |")
			return false
		}
		return true
	})
	return text
}

// umNextPage returns the absolute URL of the listing's next page, or "" on the last one.
func umNextPage(base *url.URL, root *html.Node) string {
	var href string
	walk(root, func(n *html.Node) bool {
		if href != "" {
			return false
		}
		if n.Type == html.ElementNode {
			href = Attr(n, "data-pagination-next-href")
		}
		return href == ""
	})
	if href == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	next := base.ResolveReference(ref)
	next.Fragment = ""
	return next.String()
}

// umBody renders a posting page's content: the body without scripts, styles, forms and the
// page chrome that is not the posting.
func umBody(root *html.Node) string {
	body := firstByTag(root, "body")
	if body == nil {
		return ""
	}
	var drop []*html.Node
	walk(body, func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "header", "nav", "form", "img", "link":
				drop = append(drop, n)
				return false
			}
		}
		return true
	})
	for _, n := range drop {
		n.Parent.RemoveChild(n)
	}
	return strings.TrimSpace(innerHTML(body))
}
