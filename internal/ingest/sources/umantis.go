package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
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
// language), so only the title, the location and the date are read from it: the location from
// the element ids Umantis keys it by, else from a cell the newer layout labels as the place
// ("Anstellungsort"; the same element id means another column on another site). A site that
// shows no place at all still gets its country from the host (umFallbackCountries), never a
// guessed town. The description is the posting page's own body: those pages
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
	var read atomic.Int64
	jobs := fetchDetails(rows, defaultDetailWorkers, func(r umRow) (Job, bool) {
		j, ok := s.detail(ctx, e, r)
		if ok && !j.Unreadable {
			read.Add(1)
		}
		return j, ok
	})
	if len(rows) > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("umantis: %s: listed %d postings and read none of their details", e.Board, len(rows))
	}
	return jobs, nil
}

// detail fetches one posting page and maps it to a Job. A 404/410 page is dropped (ok=false);
// an unreadable or empty one becomes an unreadable marker rather than the posting's absence.
func (s umantis) detail(ctx context.Context, e CompanyEntry, r umRow) (Job, bool) {
	root, err := s.http.GetHTML(ctx, r.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(r.id, r.url, e.Company), true
		}
		return Job{}, false // 404/410: the posting is gone
	}
	desc := sanitizeHTML(umBody(root))
	if desc == "" {
		return unreadableDetail(r.id, r.url, e.Company), true
	}
	remote := isRemote(r.location)
	return Job{
		ExternalID:        r.id,
		URL:               r.url,
		Title:             r.title,
		Company:           e.Company,
		Location:          r.location,
		Description:       desc,
		Remote:            remote,
		WorkMode:          workModeFromRemote(remote),
		PostedAt:          r.postedAt,
		FallbackCountries: umFallbackCountries(e.Board),
	}, true
}

// umFallbackCountries is the country a board's host places it in, for a posting whose listing
// states no place (many sites show no place column). Umantis serves German customers from its
// German data centre (<tenant>.de.umantis.com) and Swiss ones from the vendor's own domain; a
// vanity domain says it only through its country TLD. Anything else stays unplaced.
func umFallbackCountries(host string) []string {
	switch {
	case strings.HasSuffix(host, ".de.umantis.com"), strings.HasSuffix(host, ".de"):
		return []string{"de"}
	case strings.HasSuffix(host, ".umantis.com"), strings.HasSuffix(host, ".ch"):
		return []string{"ch"}
	}
	return nil
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
			// A cell labelled as the place wins: in the labelled layout an element id can be
			// any column, so the fixed ids are only the fallback.
			r.location = umLabelledPlace(row)
			for _, class := range umLocationElements {
				if r.location != "" {
					break
				}
				if loc := umCell(row, class); loc != "" {
					r.location = umLabelPattern.ReplaceAllString(loc, "")
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

// umPlaceLabels are the column labels (lower-cased) that name the place of work in the newer
// layout, which pairs each value with a visually hidden label: span#column_label_<n> holds the
// label and span#column_value_<n> the value.
var umPlaceLabels = map[string]bool{
	"anstellungsort": true, "arbeitsort": true, "einsatzort": true, "dienstort": true,
	"standort": true, "ort": true, "lieu de travail": true, "lieu": true, "luogo di lavoro": true,
	"location": true, "work location": true,
}

// umLabelledPlace returns the value of the row's cell labelled as the place, or "".
func umLabelledPlace(row *html.Node) string {
	var places []string           // column numbers labelled as the place, in document order
	values := map[string]string{} // column number → value
	walk(row, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "span" {
			id := Attr(n, "id")
			text := strings.TrimSpace(textContent(n))
			if num, ok := strings.CutPrefix(id, "column_label_"); ok && umPlaceLabels[strings.ToLower(text)] {
				places = append(places, num)
			} else if num, ok := strings.CutPrefix(id, "column_value_"); ok {
				values[num] = text
			}
		}
		return true
	})
	for _, num := range places {
		if values[num] != "" {
			return values[num]
		}
	}
	return ""
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
