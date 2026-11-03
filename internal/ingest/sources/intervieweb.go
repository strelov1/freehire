package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// intervieweb adapts Intervieweb (Zucchetti's Italian ATS) career sites. The board is the
// full careers HOST (e.g. "autogrill.intervieweb.it"): the platform is white-labelled, so a
// tenant answers on its own subdomain and a customer may front it with its own domain, and a
// slug alone would not say which host to ask.
//
// The landing page <host>/en/career renders the first page of vacancies server-side, each as
// an anchor to /jobs/<slug>-<id>/<lang>/, and carries everything needed to read the rest:
// the "Page 1 of N" pager, the section key, and the AJAX endpoint the pager's
// researchAnnounces() posts to (with its CSRF token already in the URL, valid without a
// session cookie). Pages 2..N are that POST (act1=vacancyListCareer, section, order, page),
// answered as {"success":true,"data":"<html fragment>"} in the same card markup.
//
// The listing carries only a title and a location, so the description comes from each
// posting's page, whose schema.org JobPosting ld+json block states it with the location and
// the dates. The adapter hydrates (HydratingSource): a detail request is spent only on a
// posting the catalogue does not already have.
type intervieweb struct {
	http interviewebTransport
}

// interviewebTransport is the transport the adapter needs: the HTML pages and the pager's form POST.
type interviewebTransport interface {
	HTMLGetter
	FormJSONPoster
}

// NewIntervieweb builds the Intervieweb adapter over the given HTTP client.
func NewIntervieweb(c interviewebTransport) Source { return intervieweb{http: c} }

func (intervieweb) Provider() string { return "intervieweb" }

// intervieweb's pager is bounded defensively: a board states its page count up front, and a
// value past this is a misread page, not a real catalogue (the largest seen is 12 pages).
const interviewebMaxPages = 200

// Fetch is the list-only fallback HydratingSource specifies for a caller that cannot supply a
// seen set; it hydrates every posting. FetchNew is the path cmd/ingest drives.
func (s intervieweb) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole board and fetches a posting page only for an id the catalogue does
// not already have. A seen posting is emitted as a liveness refresh (identity only).
func (s intervieweb) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	host := strings.TrimSpace(e.Board)
	if host == "" || strings.ContainsAny(host, "/?# ") {
		return nil, fmt.Errorf("intervieweb: board %q is not a careers host", e.Board)
	}
	base, err := url.Parse("https://" + host + "/")
	if err != nil {
		return nil, fmt.Errorf("intervieweb: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.ResolveReference(&url.URL{Path: "/en/career"}).String())
	if err != nil {
		return nil, fmt.Errorf("intervieweb: listing %s: %w", e.Board, err)
	}

	links := ivJobLinks(base, root)
	pages := ivPageCount(root)
	if pages > interviewebMaxPages {
		return nil, fmt.Errorf("intervieweb: %s states %d listing pages", e.Board, pages)
	}
	if pages > 1 {
		more, err := s.morePages(ctx, e.Board, base, root, pages)
		if err != nil {
			return nil, err
		}
		links = ivMerge(links, more)
	}

	var wanted, read atomic.Int64
	jobs := fetchDetails(links, defaultDetailWorkers, func(l ivLink) (Job, bool) {
		if seen(l.id) {
			return Job{ExternalID: l.id, URL: l.url, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		j, ok := s.detail(ctx, e, l)
		if ok && !j.Unreadable {
			read.Add(1)
		}
		return j, ok
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("intervieweb: %s: listed %d new postings and read none of their details", e.Board, wanted.Load())
	}
	return jobs, nil
}

// morePages reads listing pages 2..pages through the pager's AJAX endpoint.
func (s intervieweb) morePages(ctx context.Context, board string, base *url.URL, root *html.Node, pages int) ([]ivLink, error) {
	endpoint, section := ivPagerConfig(root)
	if endpoint == "" || section == "" {
		return nil, fmt.Errorf("intervieweb: %s states %d listing pages but exposes no pager endpoint", board, pages)
	}
	var out []ivLink
	for page := 2; page <= pages; page++ {
		var resp struct {
			Success bool   `json:"success"`
			Data    string `json:"data"`
		}
		form := url.Values{
			"act1":    {"vacancyListCareer"},
			"section": {section},
			"order":   {"date"},
			"page":    {strconv.Itoa(page)},
		}
		if err := s.http.PostFormJSON(ctx, endpoint, form, &resp); err != nil {
			return nil, fmt.Errorf("intervieweb: %s page %d: %w", board, page, err)
		}
		if !resp.Success {
			return nil, fmt.Errorf("intervieweb: %s page %d answered success=false", board, page)
		}
		frag, err := html.Parse(strings.NewReader("<body>" + resp.Data + "</body>"))
		if err != nil {
			return nil, fmt.Errorf("intervieweb: %s page %d: %w", board, page, err)
		}
		out = ivMerge(out, ivJobLinks(base, frag))
	}
	return out, nil
}

// detail maps one posting page's JobPosting ld+json to a Job. A 404/410 page is dropped
// (ok=false); an unreadable one, or one with no JobPosting, becomes an unreadable marker.
func (s intervieweb) detail(ctx context.Context, e CompanyEntry, l ivLink) (Job, bool) {
	root, err := s.http.GetHTML(ctx, l.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(l.id, l.url, e.Company), true
		}
		return Job{}, false // 404/410: the posting is gone
	}
	var p ivPosting
	if !ldJobPosting(root, &p) || strings.TrimSpace(p.Title) == "" {
		return unreadableDetail(l.id, l.url, e.Company), true
	}
	j := Job{
		ExternalID:  l.id,
		URL:         l.url,
		Title:       strings.TrimSpace(p.Title),
		Company:     e.Company,
		Description: sanitizeHTML(p.Description),
		PostedAt:    parseDate(p.DatePosted),
	}
	// validThrough is the source's own last application day, a bare UTC date. Unlike a
	// posted_at it may legitimately sit in the future, so it is not run through NotFuture.
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(p.ValidThrough)); err == nil {
		j.ApplicationDeadline = &t
	}
	j.Location, j.Countries = schemaPlacesLocation(p.JobLocation)
	return j, true
}

// ivPosting is the schema.org JobPosting on an Intervieweb posting page. datePosted and
// validThrough are bare dates (yyyy-mm-dd).
type ivPosting struct {
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	DatePosted   string       `json:"datePosted"`
	ValidThrough string       `json:"validThrough"`
	JobLocation  schemaPlaces `json:"jobLocation"`
}

// ivLink is one posting a listing links: its native id and the URL to fetch it at.
type ivLink struct {
	id  string
	url string
}

// ivJobIDPattern captures the numeric id that ends a posting permalink's slug:
// /jobs/<slug>-<id>/<lang>/. The slug may itself contain numbers, so the id is the LAST run
// of digits before the language segment.
var ivJobIDPattern = regexp.MustCompile(`/jobs/[^/?#]*-(\d+)/`)

func ivJobID(u string) string { return firstSubmatch(ivJobIDPattern, u) }

// ivJobLinks returns the postings a listing page links, de-duplicated by id (each card links
// its posting twice) and restricted to the board's own host, first-seen order.
func ivJobLinks(base *url.URL, root *html.Node) []ivLink {
	var links []ivLink
	seen := map[string]bool{}
	for _, u := range jobLinks(base, root, func(href string) bool { return ivJobID(href) != "" }) {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Host != base.Host {
			continue
		}
		id := ivJobID(u)
		if seen[id] {
			continue
		}
		seen[id] = true
		links = append(links, ivLink{id: id, url: u})
	}
	return links
}

// ivMerge appends the links of add whose id is not already in links.
func ivMerge(links, add []ivLink) []ivLink {
	have := make(map[string]bool, len(links))
	for _, l := range links {
		have[l.id] = true
	}
	for _, l := range add {
		if !have[l.id] {
			have[l.id] = true
			links = append(links, l)
		}
	}
	return links
}

var (
	ivPageCountPattern = regexp.MustCompile(`Page\s+\d+\s+of\s+(\d+)`)
	ivSectionPattern   = regexp.MustCompile(`'section'\s*:\s*'([^']+)'`)
)

// ivPageCount reads N from the pager's "Page 1 of N", or 1 when the page has no pager.
func ivPageCount(root *html.Node) int {
	n, err := strconv.Atoi(firstSubmatch(ivPageCountPattern, textContent(root)))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// ivPagerConfig reads the pager's AJAX endpoint (the hidden #url-for-announces input) and the
// section key researchAnnounces() posts, or empty strings when either is missing.
func ivPagerConfig(root *html.Node) (endpoint, section string) {
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case n.Data == "input" && Attr(n, "id") == "url-for-announces":
			endpoint = Attr(n, "value")
		case n.Data == "script" && section == "":
			section = firstSubmatch(ivSectionPattern, textContent(n))
		}
		return true
	})
	return endpoint, section
}
