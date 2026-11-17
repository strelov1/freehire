package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// njoyn adapts Njoyn career sites (<tenant>.njoyn.com; CGI's is the one verified). Every page
// of a site is one classic-ASP script,
// /<site>/xweb/xweb.asp, and what it shows is in the query: the client id (clid) and the page
// (joblisting, JobDetails). So the board is "<tenant>/<site>/<clid>", lower-cased: the site
// segment differs between tenants (CGI's is "corp") and the script 404s under any other.
//
// What the platform serves:
//   - The listing (Page=joblisting) is a table (.table-result-search), 50 rows a page, paged
//     with &pn=N, and a line stating "Page 1 of 61". Each row links the posting
//     (Page=JobDetails&Jobid=<id>&BRID=<n>) and shows its id, title, category, city and
//     country; the row's own attributes carry RemoteWork and CountryIDs. The pager states pages,
//     not postings, so the walk is complete when it has read the last page the pager names.
//   - The posting page carries a schema.org JobPosting ld+json (datePosted, validThrough,
//     employmentType, a structured address), but its description is only the posting's first
//     section. The whole posting is the page's .job-posting-content, which is taken instead.
//   - The listing is authoritative for which postings exist: a posting gone from it is gone. A
//     posting page that cannot be read (an error, or a 200 without its JobPosting, which is
//     also what a removed posting's id answers) defers that new posting to the next crawl;
//     a crawl that had new postings to read and read none of them fails.
//   - The site refuses a client it has decided is crawling too fast with a bare 200 body,
//     "Invalid request XWP10022 <the client's IP>", on every page. That is a refusal, never
//     an empty or vanished board, and fails the crawl (njoynRefused).
//   - There is no robots.txt. Requests are paced to one every njoynRequestInterval per board,
//     because the refusal above was drawn at a slower pace than one a second (what was
//     measured is in providers/njoyn.md).
//
// A steady-state crawl is one listing request per 50 postings plus a posting page only for a
// posting the catalogue has not seen (HydratingSource). The first crawl of a board reads every
// posting page.
type njoyn struct {
	http HTMLGetter
	// interval spaces the requests of one board's crawl; zero disables pacing (tests).
	interval time.Duration
}

// NewNjoyn builds the Njoyn adapter over the given HTTP client.
func NewNjoyn(c HTMLGetter) Source { return njoyn{http: c, interval: njoynRequestInterval} }

func (njoyn) Provider() string { return "njoyn" }

// fullBoardListing: the listing is walked to the last page its pager states, and the crawl
// fails otherwise.
func (njoyn) fullBoardListing() {}

const (
	njoynRequestInterval = 5 * time.Second
	// njoynMaxPages bounds the listing walk (50 rows a page). Reaching it before the pager's
	// last page FAILS the crawl, never a silent end.
	njoynMaxPages = 200
)

// njoynBoard is a parsed "<tenant>/<site>/<clid>" board.
type njoynBoard struct{ tenant, site, clid string }

var njoynBoardPattern = regexp.MustCompile(`^([a-z0-9-]+)/([a-z0-9_-]+)/(\d+)$`)

func parseNjoynBoard(board string) (njoynBoard, error) {
	m := njoynBoardPattern.FindStringSubmatch(strings.ToLower(board))
	if m == nil {
		return njoynBoard{}, fmt.Errorf("njoyn: board %q is not <tenant>/<site>/<clid>", board)
	}
	return njoynBoard{tenant: m[1], site: m[2], clid: m[3]}, nil
}

func (b njoynBoard) script() string {
	return fmt.Sprintf("https://%s.njoyn.com/%s/xweb/xweb.asp", b.tenant, b.site)
}

// listingURL is one listing page, in English (lang=1): the language of the site's labels,
// not of the postings, which are written in whichever language the employer wrote them.
func (b njoynBoard) listingURL(page int) string {
	return fmt.Sprintf("%s?NTKN=c&clid=%s&Page=joblisting&lang=1&pn=%d", b.script(), b.clid, page)
}

// detailURL is a posting's page, which is also its public permalink.
func (b njoynBoard) detailURL(id, brid string) string {
	u := fmt.Sprintf("%s?NTKN=c&clid=%s&Page=JobDetails&Jobid=%s", b.script(), b.clid, url.QueryEscape(id))
	if brid != "" {
		u += "&BRID=" + url.QueryEscape(brid)
	}
	return u + "&lang=1"
}

// njoynRow is one listing row.
type njoynRow struct {
	id, brid, title, city, country string
	remote                         bool
}

func (s njoyn) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew opens a posting page only for a posting the catalogue does not already have. A seen
// posting yields its listing row alone, flagged SeenRefresh.
func (s njoyn) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parseNjoynBoard(e.Board)
	if err != nil {
		return nil, err
	}
	get := pacedHTMLGetter(s.http, s.interval, 1)
	rows, err := njoynList(ctx, get, b)
	if err != nil {
		return nil, fmt.Errorf("njoyn: listing %s: %w", e.Board, err)
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(rows, defaultDetailWorkers, func(r njoynRow) (Job, bool) {
		if seen(r.id) {
			j := r.job(e, b)
			j.SeenRefresh = true
			return j, true
		}
		wanted.Add(1)
		j, ok := njoynDetail(ctx, get, e, b, r)
		if ok {
			read.Add(1)
		}
		return j, ok
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("njoyn: %s: listed %d new postings and read none of their pages", e.Board, wanted.Load())
	}
	return jobs, nil
}

// njoynList walks the listing to the last page its pager states. A page that fails, a page
// that adds nothing before the last, the page count moving mid-walk, or the cap all fail the
// crawl rather than returning part of the board.
func njoynList(ctx context.Context, get HTMLGetter, b njoynBoard) ([]njoynRow, error) {
	var out []njoynRow
	listed := map[string]bool{}
	last := -1
	for page := 1; page <= njoynMaxPages; page++ {
		root, err := get.GetHTML(ctx, b.listingURL(page))
		if err != nil {
			return nil, err
		}
		rows, at, of, err := njoynListingPage(root)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if of == 0 { // no pager and no rows: a board with no openings
			return out, nil
		}
		if at != page {
			return nil, fmt.Errorf("asked for page %d, the pager states page %d", page, at)
		}
		if last < 0 {
			last = of
		} else if of != last {
			return nil, fmt.Errorf("page %d: page count changed from %d to %d mid-walk", page, last, of)
		}
		added := 0
		for _, r := range rows {
			if !listed[r.id] {
				listed[r.id] = true
				out = append(out, r)
				added++
			}
		}
		if page == last {
			return out, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("page %d of %d added no posting", page, last)
		}
	}
	return nil, fmt.Errorf("stopped at the %d-page cap before page %d", njoynMaxPages, last)
}

var njoynPagerPattern = regexp.MustCompile(`Page\s+(\d+)\s+of\s+(\d+)`)

// njoynListingPage reads one listing page: its rows and the pager's "Page <at> of <of>". A
// page without the results table is not a listing and is an error; a table with rows but no
// pager cannot be shown to be the whole board, and is an error too.
func njoynListingPage(root *html.Node) (rows []njoynRow, at, of int, err error) {
	if njoynRefused(root) {
		return nil, 0, 0, errNjoynRefused
	}
	table := firstByClass(root, "table-result-search")
	if table == nil {
		return nil, 0, 0, fmt.Errorf("no results table")
	}
	walk(table, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return true
		}
		if r, ok := njoynListingRow(n); ok {
			rows = append(rows, r)
		}
		return false
	})
	m := njoynPagerPattern.FindStringSubmatch(textContent(root))
	if m == nil {
		if len(rows) > 0 {
			return nil, 0, 0, fmt.Errorf("%d rows but no pager", len(rows))
		}
		return nil, 0, 0, nil
	}
	at, _ = strconv.Atoi(m[1])
	of, _ = strconv.Atoi(m[2])
	return rows, at, of, nil
}

// njoynListingRow reads one <tr>: id, title, category, city, country. A row without a posting
// link (the header) is skipped.
func njoynListingRow(tr *html.Node) (njoynRow, bool) {
	var cells []string
	var href string
	for td := tr.FirstChild; td != nil; td = td.NextSibling {
		if td.Type != html.ElementNode || td.Data != "td" {
			continue
		}
		cells = append(cells, strings.Join(strings.Fields(textContent(td)), " "))
		if a := firstByTag(td, "a"); a != nil && href == "" {
			href = Attr(a, "href")
		}
	}
	id, brid := njoynPostingRef(href)
	if id == "" || len(cells) < 5 {
		return njoynRow{}, false
	}
	return njoynRow{
		id:      id,
		brid:    brid,
		title:   cells[1],
		city:    cells[3],
		country: cells[4],
		remote:  strings.EqualFold(Attr(tr, "remotework"), "true"),
	}, true
}

// njoynPostingRef reads a posting link's Jobid and BRID (the query keys are spelled in either
// case across a site's pages).
func njoynPostingRef(href string) (id, brid string) {
	if href == "" {
		return "", ""
	}
	u, err := url.Parse(html.UnescapeString(href))
	if err != nil {
		return "", ""
	}
	for k, v := range u.Query() {
		switch strings.ToLower(k) {
		case "jobid":
			id = strings.TrimSpace(v[0])
		case "brid":
			brid = strings.TrimSpace(v[0])
		}
	}
	return id, brid
}

// njoynPosting is the part of the posting page's JobPosting ld+json read here.
type njoynPosting struct {
	Title          string       `json:"title"`
	DatePosted     string       `json:"datePosted"`
	ValidThrough   string       `json:"validThrough"`
	EmploymentType string       `json:"employmentType"`
	JobLocation    schemaPlaces `json:"jobLocation"`
}

// njoynDetail opens one posting page and fills the listing row's job from it. It reports
// ok=false, deferring this new posting to the next crawl, when the page cannot be read, is
// refused, or carries no JobPosting or no body: the listing, not this page, says whether the
// posting exists.
func njoynDetail(ctx context.Context, get HTMLGetter, e CompanyEntry, b njoynBoard, r njoynRow) (Job, bool) {
	link := b.detailURL(r.id, r.brid)
	root, err := get.GetHTML(ctx, link)
	if err != nil {
		log.Printf("njoyn: %s: %v; deferring", link, err)
		return Job{}, false
	}
	var ld njoynPosting
	content := firstByClass(root, "job-posting-content")
	if njoynRefused(root) || !ldJobPosting(root, &ld) || content == nil {
		log.Printf("njoyn: %s carries no posting; deferring", link)
		return Job{}, false
	}
	description := sanitizeHTML(innerHTML(content))
	if description == "" {
		return Job{}, false
	}
	j := r.job(e, b)
	if places, countries := njoynPlaces(ld.JobLocation); places != "" {
		j.Location = places
		j.Countries = countries
	}
	j.Description = description
	j.PostedAt = parseDate(strings.TrimSpace(ld.DatePosted))
	// Not parseDate: it drops a date in the future, which a deadline is.
	if d, err := time.Parse("2006-01-02", strings.TrimSpace(ld.ValidThrough)); err == nil {
		j.ApplicationDeadline = &d
	}
	j.EmploymentType = schemaEmploymentType(ld.EmploymentType)
	return j, true
}

// njoynPlaces renders each jobLocation as "City, Region, Country" (several joined by "; ") and
// collects their countries. The address parts carry stray padding ("CA ").
func njoynPlaces(places schemaPlaces) (string, []string) {
	var parts, codes []string
	seen := map[string]bool{}
	for _, pl := range places {
		a := pl.Address
		country := strings.TrimSpace(a.AddressCountry)
		part := joinNonEmpty(strings.TrimSpace(a.AddressLocality), strings.TrimSpace(a.AddressRegion), country)
		if part != "" && !seen[part] {
			seen[part] = true
			parts = append(parts, part)
		}
		codes = append(codes, country)
	}
	return strings.Join(parts, "; "), countriesFromCodes(codes)
}

// errNjoynRefused is the site's refusal of this client (see the type comment).
var errNjoynRefused = errors.New("the site refused this client (Invalid request XWP10022)")

// njoynRefused reports whether a page is the site's refusal rather than a page of the site.
func njoynRefused(root *html.Node) bool {
	return strings.Contains(textContent(root), "XWP10022")
}

// job maps a listing row onto the identity every path shares: the shape a SeenRefresh carries
// and the base a hydrated posting fills in.
func (r njoynRow) job(e CompanyEntry, b njoynBoard) Job {
	loc := joinNonEmpty(njoynPlace(r.city), r.country)
	return Job{
		ExternalID: r.id,
		URL:        b.detailURL(r.id, r.brid),
		Title:      r.title,
		Company:    e.Company,
		Location:   loc,
		Remote:     r.remote || isRemote(loc),
	}
}

// njoynPlace drops the listing's "N/A" city.
func njoynPlace(city string) string {
	if strings.EqualFold(strings.TrimSpace(city), "n/a") {
		return ""
	}
	return city
}
