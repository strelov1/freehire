package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// potok adapts the public career pages Potok (potok.io), a Russian ATS, hosts per tenant at
// <board>.potok.io/open/jobs/. The page is server-rendered Lever-clone markup and lists the
// tenant's WHOLE open set in one response — no pagination. Each row carries id, title, city and
// the group it is filed under, but no body: that lives only on the posting page /open/jobs/<id>, which has no JSON-LD, so this is a
// HydratingSource that fetches a posting page only for a posting the catalogue lacks.
//
// Neither page states a salary or a publish date. The posting page adds one structured field the
// listing lacks — the hh-style schedule picklist ("Полный день", "Гибкий график", "Удаленная
// работа") — and only its remote value says anything about the work arrangement.
type potok struct {
	http HTMLGetter
}

const (
	potokListURL = "https://%s.potok.io/open/jobs/"
	potokJobURL  = "https://%s.potok.io/open/jobs/%s"
	// potokRemoteSchedule is the schedule picklist value that states a remote arrangement.
	potokRemoteSchedule = "Удаленная работа"
)

// potokJobIDPattern captures the numeric posting id from a /open/jobs/<id> href.
var potokJobIDPattern = regexp.MustCompile(`/open/jobs/(\d+)$`)

// NewPotok builds the Potok adapter over the given HTTP client.
func NewPotok(c HTMLGetter) Source { return potok{http: c} }

func (potok) Provider() string { return "potok" }

// fullBoardListing: the listing is a single unpaginated page carrying every open posting, and a
// page that is not that listing fails the crawl. See the fullBoardListing interface for the bar.
func (potok) FullBoardListing() {}

// potokPosting is one listing row: everything the platform states about a posting bar its body
// and schedule. team is the group the tenant files it under — a department on most tenants, the
// resident company on a hub tenant (tehnopark-skolkovo).
type potokPosting struct {
	id       string
	title    string
	location string
	team     string
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (s potok) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p potokPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have. A seen
// posting yields its list-only job flagged SeenRefresh, so the pipeline refreshes liveness without
// a request or a rewrite of the body hydrated when it was new.
func (s potok) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p potokPosting) (Job, bool) {
		if seen(p.id) {
			j := p.job(e)
			j.SeenRefresh = true
			return j, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// list fetches a tenant's career page and returns every posting it lists. A tenant with no open
// posting still renders the postings wrapper, empty; a 200 WITHOUT it is not the career page (the
// recruiter SPA, a changed template) and fails the board rather than reading as an empty one,
// which the board-scoped sweep would act on.
func (s potok) list(ctx context.Context, e CompanyEntry) ([]potokPosting, error) {
	root, err := s.http.GetHTML(ctx, fmt.Sprintf(potokListURL, e.Board))
	if err != nil {
		return nil, fmt.Errorf("potok: list board %s: %w", e.Board, err)
	}
	wrapper := firstByClass(root, "postings-wrapper")
	if wrapper == nil {
		return nil, fmt.Errorf("potok: board %s: page carries no postings listing", e.Board)
	}
	var out []potokPosting
	walk(wrapper, func(group *html.Node) bool {
		if group.Type != html.ElementNode || !HasClass(group, "postings-group") {
			return true
		}
		var team string
		if h := firstByClass(group, "posting-category-title"); h != nil {
			team = potokText(h)
		}
		walk(group, func(n *html.Node) bool {
			if n.Type != html.ElementNode || !HasClass(n, "posting") {
				return true
			}
			if p, ok := potokRow(n, team); ok {
				out = append(out, p)
			}
			return false
		})
		return false
	})
	return out, nil
}

// potokRow reads one div.posting. A row whose title link carries no posting id is skipped.
func potokRow(n *html.Node, team string) (potokPosting, bool) {
	link := firstByClass(n, "posting-title")
	if link == nil {
		return potokPosting{}, false
	}
	id := firstSubmatch(potokJobIDPattern, Attr(link, "href"))
	if id == "" {
		return potokPosting{}, false
	}
	p := potokPosting{id: id, team: team}
	if h := firstByTag(link, "h5"); h != nil {
		p.title = potokText(h)
	}
	if loc := firstByClass(link, "sort-by-location"); loc != nil {
		p.location = potokText(loc)
	}
	return p, p.title != ""
}

// detail fetches one posting page and maps it with its listing row. A failed request or a page
// carrying no body drops the posting: the listing, not the posting page, is what proves a
// posting exists, and an unseen posting that is dropped is simply offered again next crawl.
func (s potok) detail(ctx context.Context, e CompanyEntry, p potokPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, fmt.Sprintf(potokJobURL, e.Board, p.id))
	if err != nil {
		return Job{}, false
	}
	body := firstByClass(root, "plain-list")
	if body == nil {
		return Job{}, false
	}
	j := p.job(e)
	j.Description = sanitizeHTML(innerHTML(body))
	if potokSchedule(root) == potokRemoteSchedule {
		j.WorkMode = "remote"
		j.Remote = true
	}
	return j, true
}

// job maps a listing row onto a body-less Job. The employer is the configured company, except on
// a hub board, whose groups are the resident companies themselves.
func (p potokPosting) job(e CompanyEntry) Job {
	company := e.Company
	if e.Hub && p.team != "" {
		company = p.team
	}
	return Job{
		ExternalID: p.id,
		URL:        fmt.Sprintf(potokJobURL, e.Board, p.id),
		Title:      p.title,
		Company:    company,
		Location:   p.location,
		Remote:     isRemote(p.location),
		PostedAt:   nil, // neither page states one
	}
}

// potokSchedule reads the schedule from the posting page's headline. The platform renders the
// city and the schedule as two "sort-by-time" categories, in that order, followed by the team
// under "sort-by-team"; the second "sort-by-time" is the schedule (empty when unset).
func potokSchedule(root *html.Node) string {
	headline := firstByClass(root, "posting-headline")
	if headline == nil {
		return ""
	}
	var times []string
	walk(headline, func(n *html.Node) bool {
		if n.Type == html.ElementNode && HasClass(n, "posting-category") && HasClass(n, "sort-by-time") {
			times = append(times, potokText(n))
			return false
		}
		return true
	})
	if len(times) < 2 {
		return ""
	}
	return times[1]
}

// potokText is textContent with runs of whitespace collapsed: the pages indent their text across
// several source lines, and some tenants double-space titles.
func potokText(n *html.Node) string {
	return strings.Join(strings.Fields(textContent(n)), " ")
}
