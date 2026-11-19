package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// onlyfy adapts onlyfy by XING (formerly prescreen), a German-speaking ATS whose hosted career
// sites live at https://<board>.onlyfy.jobs (the older <board>.jobbase.io redirects there).
//
// The career site loads its listing from <site>/candidate/job/ajax_list, an HTML fragment of
// job rows paged by display_length/page. Each fragment states the board's posting count in its
// pager script ($('#pageInfoTotalCount').text(N)), so the walk is checked against it. A page
// past the end answers a "no vacant positions" row. An unknown site answers 404.
//
// A posting page (<site>/job/<id>, which redirects to a language-prefixed path) renders a
// schema.org JobPosting with the body, place, employment type and salary, so the adapter
// hydrates (HydratingSource): a detail request only for a posting the catalogue lacks.
type onlyfy struct {
	http HTMLGetter
}

// onlyfyRequestInterval/onlyfyRequestBurst pace every request the adapter makes. All sites share
// one platform edge, which answers 429 to the run's aggregate rate: unpaced, boards and posting
// pages fanned out concurrently get refused. A single client at 8-way concurrency was never
// refused, so the pace sits at about twice that. Tune it as the other pacers are tuned: down while
// boards 429.
const (
	onlyfyRequestInterval = 333 * time.Millisecond // ~3 req/s
	onlyfyRequestBurst    = 3
)

// NewOnlyfy builds the onlyfy adapter over the given HTTP client.
func NewOnlyfy(c HTMLGetter) Source { return onlyfy{http: c} }

func (onlyfy) Provider() string { return "onlyfy" }

// fullBoardListing: the walk ends only when the distinct postings reach the count the listing
// states, and any page failing fails the board.
func (onlyfy) FullBoardListing() {}

// onlyfyPageSize is how many rows one listing request asks for.
const onlyfyPageSize = 100

// onlyfyMaxPages bounds the walk well above the largest board.
const onlyfyMaxPages = 100

var (
	onlyfyBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// onlyfyJobPath captures a posting id from a listing row's link ("/job/qqeva17cjz1x...").
	onlyfyJobPath = regexp.MustCompile(`^/job/([a-z0-9]+)$`)
	// onlyfyTotal reads the board's posting count out of the fragment's pager script.
	onlyfyTotal = regexp.MustCompile(`#pageInfoTotalCount'\)\.text\((\d+)\)`)
	// onlyfyRowDate is a listing row's publication date, dd.mm.yy.
	onlyfyRowDate = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{2}$`)
)

// Fetch is the list-only fallback HydratingSource specifies; it hydrates every posting.
func (s onlyfy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the whole listing and fetches a posting page only for an id the catalogue
// does not already have; a seen posting is emitted as a liveness refresh.
func (s onlyfy) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !onlyfyBoard.MatchString(board) {
		return nil, fmt.Errorf("onlyfy: board %q is not a site name", e.Board)
	}
	site := "https://" + board + ".onlyfy.jobs"
	rows, err := s.listing(ctx, site)
	if err != nil {
		return nil, fmt.Errorf("onlyfy: board %s: %w", board, err)
	}
	return fetchDetails(rows, defaultDetailWorkers, func(r onlyfyRow) (Job, bool) {
		url := site + "/job/" + r.id
		if seen(r.id) {
			return Job{ExternalID: r.id, URL: url, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, site, r)
	}), nil
}

// onlyfyRow is one listing row: the posting id and what the row itself states about it.
type onlyfyRow struct {
	id, title, location, posted string
}

// listing walks the board's pages and returns its rows in listing order.
func (s onlyfy) listing(ctx context.Context, site string) ([]onlyfyRow, error) {
	var ids []onlyfyRow
	have := map[string]bool{}
	total := -1
	for page := 1; page <= onlyfyMaxPages; page++ {
		url := fmt.Sprintf("%s/candidate/job/ajax_list?display_length=%d&page=%d&sort=date&sort_dir=DESC", site, onlyfyPageSize, page)
		root, err := s.http.GetHTML(ctx, url)
		if err != nil {
			var se *StatusError
			if page == 1 && errors.As(err, &se) && se.Code == http.StatusNotFound {
				return nil, ErrBoardGone
			}
			return nil, fmt.Errorf("listing page %d: %w", page, err)
		}
		n, ok := onlyfyCount(root)
		if !ok {
			return nil, fmt.Errorf("listing page %d states no posting count", page)
		}
		if total >= 0 && n != total {
			return nil, fmt.Errorf("listing page %d states %d postings, page 1 stated %d", page, n, total)
		}
		total = n
		rows := onlyfyRows(root)
		for _, r := range rows {
			if !have[r.id] {
				have[r.id] = true
				ids = append(ids, r)
			}
		}
		if len(ids) >= total || len(rows) == 0 {
			break
		}
	}
	if len(ids) != total {
		return nil, fmt.Errorf("listing yielded %d postings, it states %d", len(ids), total)
	}
	return ids, nil
}

// onlyfyCount reads the posting count a listing fragment states in its pager script.
func onlyfyCount(root *html.Node) (int, bool) {
	n, found := 0, false
	walk(root, func(node *html.Node) bool {
		if found {
			return false
		}
		if node.Type == html.ElementNode && node.Data == "script" {
			if m := onlyfyTotal.FindStringSubmatch(textContent(node)); m != nil {
				n, _ = strconv.Atoi(m[1])
				found = true
			}
			return false
		}
		return true
	})
	return n, found
}

// onlyfyRows returns a listing fragment's rows, in order. A row is a div of class row-table
// holding the title link, a cell marked with the map-marker icon (the place) and a cell with the
// publication date ("23.10.26").
func onlyfyRows(root *html.Node) []onlyfyRow {
	var rows []onlyfyRow
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "div" || !HasClass(n, "row-table") {
			return true
		}
		var r onlyfyRow
		walk(n, func(c *html.Node) bool {
			if c.Type != html.ElementNode {
				return true
			}
			switch {
			case c.Data == "a" && r.id == "":
				if m := onlyfyJobPath.FindStringSubmatch(Attr(c, "href")); m != nil {
					r.id, r.title = m[1], strings.TrimSpace(textContent(c))
				}
			case c.Data == "i" && HasClass(c, "icon-map-marker") && c.Parent != nil:
				r.location = strings.TrimSpace(textContent(c.Parent))
			case c.Data == "div" && HasClass(c, "inner"):
				if t := strings.TrimSpace(textContent(c)); onlyfyRowDate.MatchString(t) {
					r.posted = t
				}
			}
			return true
		})
		if r.id != "" {
			rows = append(rows, r)
		}
		return false
	})
	return rows
}

// onlyfyPosting is the posting page's schema.org JobPosting, the fields the adapter maps.
type onlyfyPosting struct {
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	DatePosted     string       `json:"datePosted"`
	EmploymentType string       `json:"employmentType"`
	JobLocation    schemaPlaces `json:"jobLocation"`
	BaseSalary     *struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one posting. Its page renders a schema.org JobPosting on most sites;
// a site with its own job template renders none, and there the posting is the platform's
// own job-ad view, <site>/job/show/<id>/full, whose #jobAdContainer every template shares, with
// the title, place and date the listing row states. A posting the site no longer serves is
// dropped; one it failed to serve is reported unreadable.
//
// jobLocationType is deliberately not read: the site sets TELECOMMUTE on postings whose body
// describes on-site work (a petrol-station clerk, a construction engineer), so the flag states
// nothing a work mode could rest on, and the description parser decides.
func (s onlyfy) detail(ctx context.Context, e CompanyEntry, site string, r onlyfyRow) (Job, bool) {
	id, url := r.id, site+"/job/"+r.id
	root, err := s.http.GetHTML(ctx, url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, url, e.Company), true
		}
		return Job{}, false
	}
	var p onlyfyPosting
	if !LDJobPosting(root, &p) || strings.TrimSpace(p.Title) == "" {
		return s.templateDetail(ctx, e, site, r)
	}
	j := Job{
		ExternalID:     id,
		URL:            url,
		Title:          strings.TrimSpace(p.Title),
		Company:        e.Company,
		Description:    sanitizeHTML(p.Description),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
	}
	var places []string
	seenCountry := map[string]bool{}
	for _, pl := range p.JobLocation {
		if loc := pl.Address.Location(); loc != "" {
			places = append(places, loc)
		}
		for _, c := range countryFromCode(pl.Address.AddressCountry) {
			if !seenCountry[c] {
				seenCountry[c] = true
				j.Countries = append(j.Countries, c)
			}
		}
	}
	j.Location = strings.Join(places, "; ")
	if sal := p.BaseSalary; sal != nil {
		if period := schemaSalaryPeriod(sal.Value.UnitText); period != "" && sal.Currency != "" {
			j.SalaryMin, j.SalaryMax = roundSalaryPart(sal.Value.MinValue), roundSalaryPart(sal.Value.MaxValue)
			if j.SalaryMin != nil || j.SalaryMax != nil {
				j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(strings.TrimSpace(sal.Currency)), period
			}
		}
	}
	return j, true
}

// templateDetail reads a posting from the platform's job-ad view, for a site whose posting page
// renders no JobPosting. The body is #jobAdContainer; the title, place and date are the listing
// row's, since the view states them only as free text.
func (s onlyfy) templateDetail(ctx context.Context, e CompanyEntry, site string, r onlyfyRow) (Job, bool) {
	url := site + "/job/" + r.id
	root, err := s.http.GetHTML(ctx, site+"/job/show/"+r.id+"/full?mode=candidate")
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(r.id, url, e.Company), true
		}
		return Job{}, false
	}
	ad := firstByID(root, "jobAdContainer")
	if ad == nil || r.title == "" {
		return unreadableDetail(r.id, url, e.Company), true
	}
	// The view carries a hidden second copy of the ad (div.stepstone-content, display:none) for
	// a job board's import; kept, every section would appear twice.
	for hidden := firstByClass(ad, "stepstone-content"); hidden != nil && hidden != ad; hidden = firstByClass(ad, "stepstone-content") {
		hidden.Parent.RemoveChild(hidden)
	}
	return Job{
		ExternalID:  r.id,
		URL:         url,
		Title:       r.title,
		Company:     e.Company,
		Location:    r.location,
		Description: sanitizeHTML(innerHTML(ad)),
		PostedAt:    parseLayout("02.01.06", r.posted),
	}, true
}
