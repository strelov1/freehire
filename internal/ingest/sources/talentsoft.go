package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// talentsoft adapts Talentsoft career sites (Cegid Talent, formerly Talentsoft — the ATS behind
// EDF, Safran, SPIE, Sodexo, Geodis, Framatome and much of the French public sector). Each
// customer runs one server-rendered ASP.NET site on its own host: <tenant>.talent-soft.com,
// <tenant>.profils.org (the vendor's older domain), or the employer's own domain
// (passerelles.economie.gouv.fr). The board is that HOST.
//
// The listing is /offre-de-emploi/liste-toutes-offres.aspx?all=1&mode=list&page=<n>, the same
// path on every site whatever its language. "mode=list" is what makes the page parameter work
// without a session (the default map/tile view ignores it), and no LCID is sent: a language
// the site offers but its postings were not published in FILTERS them out (asadventure-bever
// lists 104 postings by default and 0 under LCID=1036). Page size is the tenant's own (5 to 30
// measured), so it is read from page one; the site states its total in
// #…_Pagination_TotalOffers, and an empty site renders #…_NoResult_NoResult instead.
//
// The site publishes no feed beyond an RSS export capped at 20 items and no JSON-LD, so the body
// is read from the posting page's #contenu-ficheoffre block — only for postings the catalogue
// does not hold (HydratingSource). A posting the site no longer serves redirects to the home
// page under HTTP 200, which is the platform's "gone".
//
// Pacing: each site is read at most once a second, and the provider as a whole at most
// talentsoftVendorRate requests a second, since most sites share the vendor's hosting.
type talentsoft struct {
	http talentsoftHTTP
	// vendor spaces every request of the run across all sites.
	vendor waiter
	// siteInterval spaces one site's requests; zero disables per-site pacing (tests).
	siteInterval   time.Duration
	maxNewPerRun   int
	deadlineMargin time.Duration
}

// talentsoftHTTP is the transport talentsoft needs: HTML with the final URL after redirects,
// because a withdrawn posting answers by redirecting home.
type talentsoftHTTP interface {
	HTMLResolvedGetter
}

// NewTalentsoft builds the Talentsoft adapter over the given HTTP client.
func NewTalentsoft(c talentsoftHTTP) Source {
	return talentsoft{
		http:           c,
		vendor:         rate.NewLimiter(talentsoftVendorRate, 1),
		siteInterval:   time.Second,
		maxNewPerRun:   talentsoftMaxNewPerBoard,
		deadlineMargin: talentsoftDeadlineMargin,
	}
}

func (talentsoft) Provider() string { return "talentsoft" }

const (
	// talentsoftVendorRate bounds the run's aggregate request rate across every site.
	talentsoftVendorRate = rate.Limit(3)
	// talentsoftMaxNewPerBoard bounds the posting pages one site buys in a run. Safran's 4,532
	// postings (measured 2026-10-28) arrive over about ten runs instead of one run starving
	// every other site of the vendor budget.
	talentsoftMaxNewPerBoard = 500
	// talentsoftDeadlineMargin is the time hydration leaves the pipeline to write what it read.
	talentsoftDeadlineMargin = 5 * time.Minute
	// talentsoftResultCap is the most results one search serves: past it the listing answers page
	// one again (Safran's 4,546 postings at 20 a page stop at page 50; Crédit Agricole's 1,320 at
	// 10 a page stop at page 100). A larger slice is split along a search facet.
	talentsoftResultCap = 1000
	// talentsoftMaxSplits bounds how many facets deep a slice is split before the crawl fails.
	talentsoftMaxSplits = 3
	// talentsoftMaxPages bounds one slice's walk, which the cap keeps under 200 pages at the
	// smallest page size measured (5); needing more fails the crawl rather than ending it early.
	talentsoftMaxPages = 250
	talentsoftListPath = "/offre-de-emploi/liste-toutes-offres.aspx"
)

var (
	// talentsoftBoard is a bare lowercase host.
	talentsoftBoard = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	// talentsoftPostingPath captures the offer id that ends every posting path, whatever the
	// site's language (/offre-de-emploi/emploi-<slug>_<id>.aspx, /stelle/stelle-<slug>_<id>.aspx,
	// /job/job-<slug>_<id>.aspx).
	talentsoftPostingPath = regexp.MustCompile(`_(\d+)\.aspx$`)
	talentsoftDigits      = regexp.MustCompile(`\d+`)
	// talentsoftListDate is the publication date a listing row may carry (dd/mm/yyyy, or with
	// dots on German-language sites).
	talentsoftListDate = regexp.MustCompile(`^(\d{1,2})[/.](\d{1,2})[/.](\d{4})$`)
)

// talentsoftPosting is one listed posting: its id, its page, and what the listing row says.
type talentsoftPosting struct {
	id, url, title string
	posted         *time.Time
	rowItems       []string
}

// Fetch is the list-only fallback HydratingSource specifies; it hydrates every posting.
func (s talentsoft) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the whole listing and buys a posting page only for an offer the catalogue does
// not hold, at most maxNewPerRun of them and none once the run nears its deadline. A seen posting
// is re-listed as a SeenRefresh; an unseen one left unread waits for a later run.
func (s talentsoft) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(e.Board), "/"))
	if !talentsoftBoard.MatchString(board) {
		return nil, fmt.Errorf("talentsoft: board %q is not a careers host", e.Board)
	}
	get := talentsoftPaced{s: s, site: s.siteLimiter()}
	postings, err := s.list(ctx, get, board)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []talentsoftPosting
	deferred := 0
	for _, p := range postings {
		switch {
		case seen(p.id):
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, Title: p.title, Company: e.Company, SeenRefresh: true})
		case len(fresh) < s.maxNewPerRun:
			fresh = append(fresh, p)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("talentsoft: board %q: hydration budget %d reached; %d new postings deferred to a later run",
			board, s.maxNewPerRun, deferred)
	}
	outOfTime := 0
	for _, p := range fresh {
		if s.nearDeadline(ctx) {
			outOfTime++
			continue
		}
		if j, ok := s.detail(ctx, get, e, p); ok {
			jobs = append(jobs, j)
		}
	}
	if outOfTime > 0 {
		log.Printf("talentsoft: board %q: %d new postings left unread as the run neared its deadline, deferred to later runs",
			board, outOfTime)
	}
	return jobs, nil
}

// siteLimiter is one site's pacer for one crawl, or nil when per-site pacing is off.
func (s talentsoft) siteLimiter() waiter {
	if s.siteInterval <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Every(s.siteInterval), 1)
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s talentsoft) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// talentsoftPaced routes one site's requests through its own pacer and the vendor's.
type talentsoftPaced struct {
	s    talentsoft
	site waiter
}

func (p talentsoftPaced) get(ctx context.Context, u string) (*html.Node, string, error) {
	for _, w := range []waiter{p.site, p.s.vendor} {
		if w == nil {
			continue
		}
		if err := w.Wait(ctx); err != nil {
			return nil, "", err
		}
	}
	return p.s.http.GetHTMLResolved(ctx, u)
}

// list walks the site's listing. Every page must answer and carry rows: a walk cut short would
// leave the postings past it unseen. The walk is not checked against the stated total exactly —
// a posting published mid-walk shifts the date-ordered pages under it, so the next run, not this
// one, sees what slid past a page boundary — but a walk that comes back far short of it fails.
func (s talentsoft) list(ctx context.Context, get talentsoftPaced, board string) ([]talentsoftPosting, error) {
	base := &url.URL{Scheme: "https", Host: board}
	byID := map[string]bool{}
	var out []talentsoftPosting
	keep := func(rows []talentsoftPosting) {
		for _, r := range rows {
			if !byID[r.id] {
				byID[r.id] = true
				out = append(out, r)
			}
		}
	}
	total, err := s.walk(ctx, get, base, url.Values{}, keep, 0)
	if err != nil {
		return nil, err
	}
	if len(out)*10 < total*9 {
		return nil, fmt.Errorf("talentsoft: listing %s: walk collected %d of the %d postings the site states",
			board, len(out), total)
	}
	return out, nil
}

// walk reads one slice of the listing — the whole site, or the postings matching filters — and
// returns the total it states. The platform serves at most talentsoftResultCap results of any
// search (page 51 of a 20-a-page listing answers page 1 again), so a slice stating more is split
// along one of the search facets the page offers and each value is walked in turn.
func (s talentsoft) walk(ctx context.Context, get talentsoftPaced, base *url.URL, filters url.Values, keep func([]talentsoftPosting), depth int) (int, error) {
	board := base.Host
	pageURL := func(n int) string {
		q := url.Values{}
		for k, v := range filters {
			q[k] = v
		}
		q.Set("all", "1")
		q.Set("mode", "list")
		q.Set("page", strconv.Itoa(n))
		return base.String() + talentsoftListPath + "?" + q.Encode()
	}
	root, final, err := get.get(ctx, pageURL(1))
	if err != nil {
		return 0, fmt.Errorf("talentsoft: listing %s%s: %w", board, talentsoftSlice(filters), err)
	}
	if !strings.HasSuffix(strings.ToLower(pathOf(final)), talentsoftListPath) {
		return 0, fmt.Errorf("talentsoft: listing %s: redirected to %s, not a Talentsoft listing", board, final)
	}
	total, stated := talentsoftTotal(root)
	rows := talentsoftRows(base, root)
	if !stated {
		if len(rows) == 0 && talentsoftNoResult(root) {
			return 0, nil
		}
		return 0, fmt.Errorf("talentsoft: listing %s%s: the page states no total", board, talentsoftSlice(filters))
	}
	if total == 0 {
		return 0, nil
	}
	if total > talentsoftResultCap {
		dim, ok := talentsoftPartition(root, filters, total, depth == 0)
		if !ok || depth >= talentsoftMaxSplits {
			return 0, fmt.Errorf("talentsoft: listing %s%s: states %d postings, above the %d one search serves, and no facet splits them",
				board, talentsoftSlice(filters), total, talentsoftResultCap)
		}
		covered := 0
		for _, v := range dim.values {
			sub := url.Values{}
			for k, vv := range filters {
				sub[k] = vv
			}
			sub.Set("facet_"+dim.name, v.id)
			n, err := s.walk(ctx, get, base, sub, keep, depth+1)
			if err != nil {
				return 0, err
			}
			covered += n
		}
		if total-covered > talentsoftSplitSlack(total) {
			return 0, fmt.Errorf("talentsoft: listing %s%s: splitting %d postings by %s reached only %d",
				board, talentsoftSlice(filters), total, dim.name, covered)
		}
		return total, nil
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("talentsoft: listing %s%s: states %d postings and lists none", board, talentsoftSlice(filters), total)
	}
	pages := (total + len(rows) - 1) / len(rows)
	if pages > talentsoftMaxPages {
		return 0, fmt.Errorf("talentsoft: listing %s: %d postings need %d pages, above the %d-page bound",
			board, total, pages, talentsoftMaxPages)
	}
	keep(rows)
	for n := 2; n <= pages; n++ {
		root, _, err := get.get(ctx, pageURL(n))
		if err != nil {
			return 0, fmt.Errorf("talentsoft: listing %s%s page %d: %w", board, talentsoftSlice(filters), n, err)
		}
		rows := talentsoftRows(base, root)
		if len(rows) == 0 {
			return 0, fmt.Errorf("talentsoft: listing %s%s: page %d of %d lists no posting", board, talentsoftSlice(filters), n, pages)
		}
		keep(rows)
	}
	return total, nil
}

// talentsoftSlice names a filtered slice in an error message, or "" for the whole site.
func talentsoftSlice(filters url.Values) string {
	if len(filters) == 0 {
		return ""
	}
	return " (" + filters.Encode() + ")"
}

var (
	// talentsoftFacetCount is the count a facet value's label ends in ("CDI (3382)").
	talentsoftFacetCount = regexp.MustCompile(`\((\d+)\)\s*$`)
	talentsoftFacetID    = regexp.MustCompile(`^[0-9]+$`)
)

// talentsoftDim is one search dimension a slice can be split along: the facet_<name> URL
// parameter and its values with their counts. Counts read from the facet panel are the slice's
// own; counts read from the search form are the whole site's.
type talentsoftDim struct {
	name        string
	values      []talentsoftDimValue
	conditional bool
}

type talentsoftDimValue struct {
	id    string
	count int
}

// talentsoftSplitSlack is how far a split's values may fall short of the slice they split: a
// posting filed under no value of a field counts in the total and under none of its values.
// Safran's 2,104 permanent European postings sum to 2,103 by job family (2026-10-28); EDF's
// 1,997 sum to 1,934 by region, the rest being filed under a country alone. What a split
// leaves out is not read, and the walk's own 90% floor bounds the whole loss.
func talentsoftSplitSlack(total int) int { return max(2, total*5/100) }

// talentsoftPartition picks the dimension to split a slice along. Two sources offer them. The
// facet panel (Safran, Crédit Agricole) links each value as ?changefacet=1&facet_<dim>=<id> with
// the slice's own count. A site without one (EDF) still takes the same parameters, which its
// search form names only by field: the contract select is facet_JobDescription_Contract, and a
// geographic checkbox is facet_Country, facet_Region or facet_Department by its depth in the
// tree (its aria-labelledby lists its ancestors). Form counts are the whole site's, so they can
// vouch for a split only at the top of the walk; deeper, a form split is checked after it is
// walked.
//
// Of the dimensions whose counts cover the slice (or that cannot be checked yet), the one whose
// largest value is smallest is taken, which keeps the next level of splitting shallow; checked
// ones come first.
func talentsoftPartition(root *html.Node, filters url.Values, total int, top bool) (talentsoftDim, bool) {
	var best talentsoftDim
	bestChecked, bestMax, found := false, 0, false
	for _, d := range talentsoftDims(root) {
		if filters.Has("facet_"+d.name) || len(d.values) == 0 {
			continue
		}
		sum, largest := 0, 0
		for _, v := range d.values {
			sum += v.count
			largest = max(largest, v.count)
		}
		checked := d.conditional || top
		if checked && (sum > total || total-sum > talentsoftSplitSlack(total)) {
			continue
		}
		switch {
		case !found, checked && !bestChecked:
		case checked != bestChecked, largest > bestMax, largest == bestMax && d.name >= best.name:
			continue
		}
		best, bestChecked, bestMax, found = d, checked, largest, true
	}
	return best, found
}

// talentsoftDims reads every dimension the page offers: the facet panel's links, then the
// search form's contract select and geographic checkboxes.
func talentsoftDims(root *html.Node) []talentsoftDim {
	byName := map[string]*talentsoftDim{}
	var order []string
	add := func(name, id string, count int, conditional bool) {
		d, ok := byName[name]
		if !ok {
			d = &talentsoftDim{name: name, conditional: conditional}
			byName[name] = d
			order = append(order, name)
		}
		for _, v := range d.values {
			if v.id == id {
				return
			}
		}
		d.values = append(d.values, talentsoftDimValue{id: id, count: count})
	}
	count := func(label string) (int, bool) {
		m := talentsoftFacetCount.FindStringSubmatch(talentsoftClean(label))
		if m == nil {
			return 0, false
		}
		n, err := strconv.Atoi(m[1])
		return n, err == nil
	}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case n.Data == "a":
			u, err := url.Parse(Attr(n, "href"))
			if err != nil {
				return false
			}
			c, ok := count(textContent(n))
			if !ok {
				return false
			}
			for k, v := range u.Query() {
				dim, isFacet := strings.CutPrefix(k, "facet_")
				if isFacet && len(v) == 1 && talentsoftFacetID.MatchString(v[0]) {
					add(dim, v[0], c, true)
				}
			}
			return false
		case n.Data == "select" && strings.HasSuffix(Attr(n, "name"), "EngineCriteriaCollection$Contract"):
			for o := n.FirstChild; o != nil; o = o.NextSibling {
				if o.Type != html.ElementNode || o.Data != "option" || !talentsoftFacetID.MatchString(Attr(o, "value")) || Attr(o, "value") == "0" {
					continue
				}
				if c, ok := count(textContent(o)); ok {
					add("JobDescription_Contract", Attr(o, "value"), c, false)
				}
			}
			return false
		case n.Data == "input" && Attr(n, "type") == "checkbox" && strings.Contains(Attr(n, "name"), "GeographicalAreaCollection$"):
			id := Attr(n, "value")
			c, ok := count(Attr(n, "aria-label"))
			level := len(strings.Fields(Attr(n, "aria-labelledby")))
			if !ok || !talentsoftFacetID.MatchString(id) || level < 1 || level > len(talentsoftGeoLevels) {
				return false
			}
			add(talentsoftGeoLevels[level-1], id, c, false)
			return false
		}
		return true
	})
	out := make([]talentsoftDim, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}

// talentsoftGeoLevels names the URL parameter of a geographic checkbox by its depth in the
// search form's tree, as the site's own "show the search URL" spells them.
var talentsoftGeoLevels = []string{"Country", "Region", "Department"}

// pathOf is a URL's path, or "" when it does not parse.
func pathOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Path
}

// talentsoftTotal reads the stated posting count ("4532 offre(s)", "130 Stellenangebote") from
// the pagination header, reporting whether the page carries one.
func talentsoftTotal(root *html.Node) (int, bool) {
	var text string
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && strings.HasSuffix(Attr(n, "id"), "_Pagination_TotalOffers") {
			text, found = textContent(n), true
			return false
		}
		return true
	})
	if !found {
		return 0, false
	}
	digits := talentsoftDigits.FindString(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '.' || r == ',' {
			return -1 // thousands separators: "1 993", "1.993"
		}
		return r
	}, text))
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}

// talentsoftNoResult reports whether the page is the platform's own empty result.
func talentsoftNoResult(root *html.Node) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && strings.HasSuffix(Attr(n, "id"), "_NoResult_NoResult") {
			found = true
			return false
		}
		return true
	})
	return found
}

// talentsoftRows reads a listing page's postings: each li.ts-offer-list-item names its posting
// in a.ts-offer-list-item__title-link and lists reference, date, contract and place in its
// description items, in an order the tenant configures.
func talentsoftRows(base *url.URL, root *html.Node) []talentsoftPosting {
	var out []talentsoftPosting
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "ts-offer-list-item") {
			return true
		}
		link := firstByClass(n, "ts-offer-list-item__title-link")
		if link == nil {
			return false
		}
		u, id := talentsoftPostingURL(base, Attr(link, "href"))
		if id == "" {
			return false
		}
		p := talentsoftPosting{id: id, url: u, title: talentsoftClean(textContent(link))}
		if desc := firstByClass(n, "ts-offer-list-item__description"); desc != nil {
			for c := desc.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode || c.Data != "li" {
					continue
				}
				item := talentsoftClean(textContent(c))
				if item == "" {
					continue
				}
				p.rowItems = append(p.rowItems, item)
				if p.posted == nil {
					p.posted = talentsoftDate(item)
				}
			}
		}
		out = append(out, p)
		return false
	})
	return out
}

// talentsoftPostingURL resolves a posting link against the site and pins it there over https,
// returning it with its offer id, or "" when the link is not a posting.
func talentsoftPostingURL(base *url.URL, href string) (string, string) {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", ""
	}
	u := base.ResolveReference(ref)
	id := firstSubmatch(talentsoftPostingPath, u.Path)
	if id == "" {
		return "", ""
	}
	pinned := url.URL{Scheme: "https", Host: base.Host, Path: u.Path}
	return pinned.String(), id
}

// talentsoftDate reads a row item as a day-first date, or nil when it is not one.
func talentsoftDate(item string) *time.Time {
	m := talentsoftListDate.FindStringSubmatch(item)
	if m == nil {
		return nil
	}
	day, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	year, _ := strconv.Atoi(m[3])
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day || int(t.Month()) != month {
		return nil // not a real calendar date (a month-first site, or garbage)
	}
	return NotFuture(&t)
}

// detail buys one posting page. A redirect off the posting (home, under HTTP 200) or a 404/410
// is the platform's "gone" and drops it; any other failure, or a page without the posting block,
// is an unreadable marker — the page is the only source of the body.
func (s talentsoft) detail(ctx context.Context, get talentsoftPaced, e CompanyEntry, p talentsoftPosting) (Job, bool) {
	root, final, err := get.get(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		return Job{}, false
	}
	if firstSubmatch(talentsoftPostingPath, pathOf(final)) != p.id {
		return Job{}, false
	}
	block := firstByID(root, "contenu-ficheoffre")
	if block == nil {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	description := sanitizeHTML(innerHTML(block))
	if strings.TrimSpace(description) == "" {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	j := Job{
		ExternalID:  p.id,
		URL:         p.url,
		Title:       firstNonEmpty(p.title, talentsoftField(root, "fldjobdescription_jobtitle")),
		Company:     e.Company,
		Description: description,
		PostedAt:    p.posted,
		Location: firstNonEmpty(
			joinNonEmpty(talentsoftField(root, "fldlocation_joblocation"), talentsoftArea(talentsoftField(root, "fldlocation_location_geographicalareacollection"))),
			p.listedPlace(),
		),
		EmploymentType: talentsoftEmploymentType(append(talentsoftContractLabels(root), p.rowItems...)),
	}
	if j.Title == "" {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	return j, true
}

// talentsoftContractLabels gathers what a page says about the contract: the standard contract
// and working-time fields, plus the job description's custom code fields, which is where a
// tenant that renamed the contract field keeps it (adsn files "CDI" under customcodetablevalue1).
func talentsoftContractLabels(root *html.Node) []string {
	labels := []string{talentsoftField(root, "fldjobdescription_contract"), talentsoftField(root, "fldjobdescription_jobtime")}
	for i := 1; i <= 3; i++ {
		labels = append(labels, talentsoftField(root, "fldjobdescription_customcodetablevalue"+strconv.Itoa(i)))
	}
	return labels
}

// listedPlace is the listing row's place, for a page that labels none (Sodexo's renders no
// field at all): the platform's default row is reference, date, contract, place, so the LAST
// item is the place unless the tenant's row ends on the reference or the date.
func (p talentsoftPosting) listedPlace() string {
	if len(p.rowItems) == 0 {
		return ""
	}
	last := p.rowItems[len(p.rowItems)-1]
	lower := strings.ToLower(last)
	if talentsoftListDate.MatchString(last) || strings.HasPrefix(lower, "réf") || strings.HasPrefix(lower, "ref") {
		return ""
	}
	return last
}

// talentsoftField is the text of one of the posting page's labelled fields (<p id="fld…">), with
// its whitespace collapsed, or "".
func talentsoftField(root *html.Node, id string) string {
	n := firstByID(root, id)
	if n == nil {
		return ""
	}
	return talentsoftClean(textContent(n))
}

// talentsoftArea cleans the geographic-area path ("Europe, France, Ile-de-France, Essonne (91)"),
// dropping the empty segments a continent-only area leaves ("Afrique, ").
func talentsoftArea(area string) string {
	var parts []string
	for _, p := range strings.Split(area, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// talentsoftEmploymentType reads the employment type from the contract, working-time and listing
// row labels, which a tenant words in its own language ("CDI", "CDD-Temps plein", "Stage",
// "Part time permanent", "Teilzeit"). A contract's duration alone (CDI, CDD) is full-time unless
// a label says otherwise; a label naming nothing leaves it unknown.
func talentsoftEmploymentType(labels []string) string {
	text := " " + strings.ToLower(strings.Join(labels, " ")) + " "
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("stage", "internship", " intern ", "praktik", "stagiaire", "trainee"):
		return "internship"
	case has("temps partiel", "part time", "part-time", "teilzeit", "deeltijd"):
		return "part_time"
	case has("intérim", "interim", "freelance", "indépendant", "independant"):
		return "contract"
	case has(" cdi", " cdd", "temps plein", "full time", "full-time", "vollzeit", "permanent", "durée indéterminée", "durée déterminée", "unbefristet", "befristet"):
		return "full_time"
	default:
		return ""
	}
}

// talentsoftClean folds every whitespace run, non-breaking spaces included, to one space.
func talentsoftClean(s string) string { return strings.Join(strings.Fields(s), " ") }
