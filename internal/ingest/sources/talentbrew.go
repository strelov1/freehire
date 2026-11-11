package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"
)

// talentbrew adapts Radancy's TalentBrew career sites. They are white-labelled onto each
// employer's own host (careers.aldi.us, jobs.chsinc.com, careers.bat.com/en), so the board is
// the careers HOST, plus the locale path prefix when the site is served under one: "careers.aldi.us"
// or "careers.bat.com/en". The listing is then https://<board>/search-jobs and its results
// endpoint https://<board>/search-jobs/results — no other tenant identity is needed.
//
// The results endpoint answers a GET with {"results": "<html fragment>"}: the fragment holds a
// <section id="search-results"> whose data-total-pages says how many pages RecordsPerPage cuts
// the board into, and one <a href=… data-job-id=…> per posting (a page of 100 was honoured on
// the sampled boards). The listing carries no description, so the adapter hydrates
// (HydratingSource): a posting page — whose schema.org JobPosting ld+json block states the
// title, description, date and locations — is fetched only for a posting the catalogue does not
// already have. The boards are large (ALDI US lists ~1,550 postings), so that is the difference
// between a request per posting and a request per NEW posting on every crawl.
//
// The results endpoint is NOT answered without the facet parameters the site's own script sends:
// a bare CurrentPage/RecordsPerPage query returns an empty fragment with hasJobs=true.
type talentbrew struct {
	http talentbrewGetter
}

type talentbrewGetter interface {
	JSONGetter
	HTMLGetter
}

// NewTalentbrew builds the talentbrew adapter over the given HTTP client.
func NewTalentbrew(c talentbrewGetter) Source { return talentbrew{http: c} }

func (talentbrew) Provider() string { return "talentbrew" }

const (
	// tbPageSize is the RecordsPerPage a listing request asks for.
	tbPageSize = 100
	// tbMaxPages bounds the page walk, so a board whose data-total-pages never converges cannot
	// loop forever. 100 pages × tbPageSize is far past the largest board sampled (~1,550).
	tbMaxPages = 100
	// tbResultsQuery is the facet parameter set the site's own search script sends with every
	// request, empty. Without it the endpoint answers an empty fragment.
	tbResultsQuery = "ActiveFacetID=0&TotalContentResults=&Distance=50&RadiusUnitType=0&Keywords=&Location=" +
		"&ShowRadius=False&IsPagination=False&CustomFacetName=&FacetTerm=&FacetType=0" +
		"&SearchResultsModuleName=Search+Results&SortCriteria=0&SortDirection=0&SearchType=5&PostalCode=" +
		"&ResultsType=0&fc=&fl=&fcf=&afc=&afl=&afcf="
)

// Fetch is the list-only fallback HydratingSource specifies for a caller that cannot supply a
// seen set; it hydrates every posting. FetchNew is the path cmd/ingest drives.
func (s talentbrew) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the whole listing but fetches a posting page only for an id the catalogue does
// not already have. A seen posting is emitted as a liveness refresh (identity only).
func (s talentbrew) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	base, err := tbBase(e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.listing(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("talentbrew: listing %s: %w", e.Board, err)
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(postings, defaultDetailWorkers, func(p tbPosting) (Job, bool) {
		if seen(p.id) {
			return Job{ExternalID: p.id, URL: p.url, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		j, ok := s.detail(ctx, e, p)
		if ok && !j.Unreadable {
			read.Add(1)
		}
		return j, ok
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("talentbrew: %s: listed %d new postings and read none of their details", e.Board, wanted.Load())
	}
	return jobs, nil
}

// tbBase validates a board identity ("host" or "host/locale") and returns the URL its listing
// paths hang off, without a trailing slash.
func tbBase(board string) (*url.URL, error) {
	if board == "" || strings.ContainsAny(board, " \t?#@:") || strings.HasPrefix(board, "/") {
		return nil, fmt.Errorf("talentbrew: board %q: want a careers host, optionally with a locale path (careers.bat.com/en)", board)
	}
	u, err := url.Parse("https://" + strings.TrimRight(board, "/"))
	if err != nil || u.Host == "" || !strings.Contains(u.Host, ".") {
		return nil, fmt.Errorf("talentbrew: board %q: not a careers host", board)
	}
	return u, nil
}

// tbPosting is one posting the listing names: its native id and the page to fetch it at.
type tbPosting struct {
	id  string
	url string
}

// listing returns postings only after every declared page was read. A partial listing
// cannot authorize the caller's stale-job sweep, even when earlier pages were positive.
func (s talentbrew) listing(ctx context.Context, base *url.URL) ([]tbPosting, error) {
	var out []tbPosting
	seen := map[string]bool{}
	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		var resp struct {
			Results string `json:"results"`
		}
		u := fmt.Sprintf("%s/search-jobs/results?CurrentPage=%d&RecordsPerPage=%d&%s", base, page, tbPageSize, tbResultsQuery)
		if err := s.http.GetJSON(ctx, u, &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		root, err := html.Parse(strings.NewReader(resp.Results))
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		section, declared, err := tbResults(root)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if page == 1 {
			totalPages = declared
		} else if declared != totalPages {
			return nil, fmt.Errorf("page %d: page count changed from %d to %d", page, totalPages, declared)
		}
		added := 0
		walk(section, func(n *html.Node) bool {
			if n.Type != html.ElementNode || n.Data != "a" {
				return true
			}
			id, href := strings.TrimSpace(Attr(n, "data-job-id")), Attr(n, "href")
			if id == "" || href == "" || seen[id] {
				return true
			}
			ref, err := url.Parse(href)
			if err != nil {
				return true
			}
			// A posting linked on another host is another board's; the board IS a host here.
			abs := base.ResolveReference(ref)
			if abs.Host != base.Host {
				return true
			}
			seen[id] = true
			out = append(out, tbPosting{id: id, url: abs.String()})
			added++
			return true
		})
		if added == 0 && totalPages > 1 {
			return nil, fmt.Errorf("page %d of %d: no new postings", page, totalPages)
		}
	}
	return out, nil
}

// tbResults requires the listing's pagination declaration. An arbitrary HTML response
// or missing metadata is not a successfully completed one-page board.
func tbResults(root *html.Node) (*html.Node, int, error) {
	var section *html.Node
	count := 0
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "section" && Attr(n, "id") == "search-results" {
			section = n
			count++
		}
		return true
	})
	if count != 1 {
		return nil, 0, fmt.Errorf("want one search-results section, got %d", count)
	}
	pages, err := strconv.Atoi(Attr(section, "data-total-pages"))
	if err != nil || pages < 1 || pages > tbMaxPages {
		return nil, 0, fmt.Errorf("invalid data-total-pages %q (want 1..%d)", Attr(section, "data-total-pages"), tbMaxPages)
	}
	return section, pages, nil
}

// tbLD is the JobPosting ld+json a TalentBrew posting page carries. jobLocation is an array.
type tbLD struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	DatePosted  string       `json:"datePosted"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// detail fetches one posting page. A page the platform says is gone (404/410) is dropped
// (ok=false); one that cannot be read, or carries no JobPosting, is an unreadable marker, so a
// transient failure is never filed as the posting's absence.
func (s talentbrew) detail(ctx context.Context, e CompanyEntry, p tbPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		return Job{}, false // 404/410: the posting is gone
	}
	var ld tbLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Title) == "" {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	j := Job{
		ExternalID:  p.id,
		URL:         p.url,
		Title:       strings.TrimSpace(ld.Title),
		Company:     e.Company,
		Description: sanitizeHTML(ld.Description),
		PostedAt:    parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
	}
	j.Location, j.Countries = schemaPlacesLocation(ld.JobLocation)
	j.Remote = isRemote(j.Location)
	j.WorkMode = workModeFromRemote(j.Remote)
	return j, true
}
