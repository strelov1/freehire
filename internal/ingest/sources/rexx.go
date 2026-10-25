package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// rexx adapts rexx systems job portals (rexx-systems.com), a German HR suite whose recruiting
// module serves each tenant a server-rendered portal. The board is the portal's BASE — a host,
// optionally with the path the portal lives under: <tenant>-portal.rexx-systems.com,
// <tenant>-portal.rexx-recruitment.com, or an employer's own domain (jobs.newyorker.de/karriere).
//
// The listing is <base>/stellenangebote.html, 100 postings a page under ?start=<offset>. The
// portal's markup is a per-tenant template (a table on one, cards on another), so postings are
// read from their permalinks, which every template shares: /<slug>-<lang>-j<id>.html. A portal
// on the vendor host routinely links its postings on the employer's own domain
// (zieglersche-portal.rexx-recruitment.com lists jobs.zieglersche.de/...-de-j3040.html), so the
// permalink host is not checked — the id pattern is what identifies a posting. Each link carries
// a per-request session id (?sid=), which is dropped.
//
// A posting page renders a schema.org JobPosting whose description already holds the page's
// task, profile and benefit sections, so the adapter hydrates (HydratingSource): a detail
// request only for a posting the catalogue does not have.
type rexx struct {
	http HTMLGetter
}

// NewRexx builds the rexx adapter over the given HTTP client.
func NewRexx(c HTMLGetter) Source { return rexx{http: c} }

func (rexx) Provider() string { return "rexx" }

// rexxPageSize is how many postings one listing page carries; ?start advances by it.
const rexxPageSize = 100

// rexxMaxPages bounds the listing walk. The largest portal measured (newyorker, 918 postings)
// has ten pages; reaching the bound without a natural end fails the crawl (crawlAllPagedLinks).
const rexxMaxPages = 100

// rexxBoard is the shape of a portal base: a lowercase host, optionally followed by path
// segments, with no scheme, query or trailing slash.
var rexxBoard = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+(/[A-Za-z0-9._-]+)*$`)

// rexxPermalink captures the posting id from a permalink's last path segment
// ("Altenpflegehelfer-mwd-de-j3040.html"). The language code is the portal's (de, eng, fr, it).
var rexxPermalink = regexp.MustCompile(`-[a-z]{2,3}-j(\d+)\.html$`)

// Fetch is the list-only fallback HydratingSource specifies; it hydrates every posting.
func (s rexx) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the whole listing and fetches a posting page only for an id the catalogue
// does not already have; a seen posting is emitted as a liveness refresh.
func (s rexx) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.TrimSuffix(strings.TrimSpace(e.Board), "/")
	if host, rest, _ := strings.Cut(board, "/"); host != "" {
		board = strings.ToLower(host) + strings.TrimSuffix("/"+rest, "/")
	}
	if !rexxBoard.MatchString(board) {
		return nil, fmt.Errorf("rexx: board %q is not a portal base", e.Board)
	}
	base, err := url.Parse("https://" + board + "/")
	if err != nil {
		return nil, fmt.Errorf("rexx: board %q: %w", e.Board, err)
	}
	listing := base.JoinPath("stellenangebote.html").String()
	// Any page failing fails the board: a walk cut short would leave the postings past the
	// failed page unseen, and the sweep would close them.
	// A page that is not a rexx listing at all (a maintenance or challenge page under HTTP 200)
	// yields no permalink and would read as the listing's natural end; every listing carries
	// the vendor's footer link, so a page without one fails the board instead.
	foreign := 0
	urls, err := crawlAllPagedLinks(ctx, s.http, rexxMaxPages, func(page int) string {
		return listing + "?start=" + strconv.Itoa((page-1)*rexxPageSize)
	}, func(root *html.Node) []string {
		if !rexxIsListing(root) {
			foreign++
		}
		return rexxLinks(base, root)
	})
	if err != nil {
		return nil, fmt.Errorf("rexx: listing %s: %w", board, err)
	}
	if foreign > 0 {
		return nil, fmt.Errorf("rexx: listing %s: %d page(s) are not a rexx listing", board, foreign)
	}
	links := rexxDedupe(urls)
	return fetchDetails(links, defaultDetailWorkers, func(l rexxLink) (Job, bool) {
		if seen(l.id) {
			return Job{ExternalID: l.id, URL: l.url, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, l)
	}), nil
}

// rexxLink is one posting the listing links: its id and its permalink without the session id.
type rexxLink struct {
	id  string
	url string
}

// rexxLinks returns a listing page's posting permalinks, absolute and stripped of their query
// and fragment, so the same posting reads as the same link on every page whatever session id
// the portal minted for that request.
func rexxLinks(base *url.URL, root *html.Node) []string {
	var out []string
	for _, u := range jobLinks(base, root, func(href string) bool { return rexxID(href) != "" }) {
		p, err := url.Parse(u)
		if err != nil {
			continue
		}
		p.RawQuery, p.Fragment = "", ""
		out = append(out, p.String())
	}
	return out
}

// rexxIsListing reports whether a page is a rexx portal page: every portal measured
// (115 of the 115 serving a real listing, 2026-10-25) links the vendor in its footer.
func rexxIsListing(root *html.Node) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "a" && strings.Contains(Attr(n, "href"), "rexx-systems.com") {
			found = true
			return false
		}
		return true
	})
	return found
}

// rexxDedupe keys the walk's links by posting id: the slug and language are not part of a
// posting's identity (the portal serves an id under any slug), so one posting linked twice is
// one posting. First-seen order, first-seen URL.
func rexxDedupe(urls []string) []rexxLink {
	var links []rexxLink
	seen := map[string]bool{}
	for _, u := range urls {
		id := rexxID(u)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		links = append(links, rexxLink{id: id, url: u})
	}
	return links
}

// rexxID extracts the posting id from a permalink, or "" when the URL is not one. Only the path
// is matched, so an id in a query string never counts.
func rexxID(u string) string {
	p := u
	if parsed, err := url.Parse(u); err == nil {
		p = parsed.Path
	}
	return firstSubmatch(rexxPermalink, p)
}

// rexxPosting is the posting page's schema.org JobPosting, the fields the adapter maps.
type rexxPosting struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	DatePosted      string       `json:"datePosted"`
	EmploymentType  string       `json:"employmentType"`
	JobLocationType string       `json:"jobLocationType"`
	JobLocation     schemaPlaces `json:"jobLocation"`
	BaseSalary      *struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail fetches one posting page and maps its JobPosting. A posting the portal no longer
// serves is dropped; one it failed to serve is reported unreadable, never silently missing.
func (s rexx) detail(ctx context.Context, e CompanyEntry, l rexxLink) (Job, bool) {
	root, err := s.http.GetHTML(ctx, l.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(l.id, l.url, e.Company), true
		}
		return Job{}, false
	}
	var p rexxPosting
	if !LDJobPosting(root, &p) || strings.TrimSpace(p.Title) == "" {
		return unreadableDetail(l.id, l.url, e.Company), true
	}
	j := Job{
		ExternalID: l.id,
		URL:        l.url,
		Title:      strings.TrimSpace(p.Title),
		// hiringOrganization.name is the PORTAL's name ("Home Shopping Europe GmbH Jobportal"
		// on a third of portals), so the catalogue's employer stands.
		Company:        e.Company,
		Description:    sanitizeHTML(p.Description),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
	}
	if strings.EqualFold(strings.TrimSpace(p.JobLocationType), "TELECOMMUTE") {
		j.Remote, j.WorkMode = true, "remote"
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
