package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// hireclick adapts HireClick career sites (<board>.hireclick.com). The board is the tenant
// subdomain.
//
// A crawl is two requests before any posting is read. The career site's job board page
// ("/jobboard/jobboard.aspx", where the bare host redirects) renders no postings of its own: it
// carries the tenant's organisation GUID in a hidden input (#hdnOrgGuid), and its script asks
// "/api/Controllers/JobBoard/GetSettingsAndJobs?orgGUID=<guid>" for the list, which it then pages
// through client-side. That endpoint answers the WHOLE board in one response — there is no
// server-side paging parameter, which is what earns fullBoardListing — and its body is a JSON
// STRING holding the JSON document, so it is decoded twice. The GUID cannot be derived from the
// subdomain and the endpoint accepts nothing else, so the page fetch is not optional. A
// subdomain no tenant owns redirects to "/misc/maintenance", which carries no GUID.
//
// The list states each posting's title, URL and city but only a truncated plain-text excerpt of
// the body, so the description comes from the posting's own page ("/jb/<slug>/view/<id>"), which
// carries a schema.org JobPosting ld+json with the full HTML body, the publish date, the
// employment type and a postal address. Every page on the platform is heavy (the tenant logo is
// inlined as base64 several times over and an ASP.NET view state rides along) and is served
// uncompressed and slowly, while the JSON list answers fast. So the adapter rides the long-timeout
// client under one run-wide in-flight cap (registry.go, limitedHireClick in pacer.go; the cap, not
// the detail fan-out below, bounds load), and it hydrates: a posting already in the catalogue is
// refreshed from the list alone.
//
// A posting the platform has taken down redirects to "/jobboard/" rather than answering 404.
//
// No structured work-arrangement field is published (no jobLocationType), so remote falls back
// to the location text.
type hireclick struct {
	http hireclickHTTP
}

// hireclickHTTP is the transport the adapter needs: the board page as a DOM, the list as JSON,
// and the posting pages with their final URL, because a removed posting answers with a redirect.
type hireclickHTTP interface {
	HTMLGetter
	HTMLResolvedGetter
	JSONGetter
}

// NewHireClick builds the HireClick adapter over the given HTTP client.
func NewHireClick(c hireclickHTTP) Source { return hireclick{http: c} }

func (hireclick) Provider() string { return "hireclick" }

// fullBoardListing: the list endpoint is a single unpaginated request returning the board's
// whole job list, and any failure to read it fails the Fetch. See the fullBoardListing
// interface for the bar.
func (hireclick) fullBoardListing() {}

func (s hireclick) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p hireclickPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew is the hydrating crawl: it lists the whole board, but fetches a posting's page only
// for an id the catalogue does not already have. A seen posting is emitted as a liveness refresh
// carrying what the list states (identity and title), with no detail request and no content
// rewrite.
func (s hireclick) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p hireclickPosting) (Job, bool) {
		if seen(p.id) {
			return Job{
				ExternalID:  p.id,
				URL:         p.url,
				Title:       p.title,
				Company:     e.Company,
				SeenRefresh: true,
			}, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// hireclickPosting is one entry of the board's list: the posting's id, its title and its page.
type hireclickPosting struct {
	id    string
	title string
	url   string
}

// hireclickList is the document the list endpoint's JSON string decodes to. Settings (the
// tenant's "General Application" pseudo-posting among them) is not a vacancy and is not read.
type hireclickList struct {
	JobList []struct {
		JobTitle string `json:"JobTitle"`
		JobURL   string `json:"JobURL"`
	} `json:"JobList"`
}

// list reads the board's organisation GUID from its job board page, then the whole job list.
func (s hireclick) list(ctx context.Context, e CompanyEntry) ([]hireclickPosting, error) {
	origin := fmt.Sprintf("https://%s.hireclick.com", e.Board)
	root, err := s.http.GetHTML(ctx, origin+"/jobboard/jobboard.aspx")
	if err != nil {
		return nil, fmt.Errorf("hireclick: board page %s: %w", e.Board, err)
	}
	guid := hireclickOrgGUID(root)
	if guid == "" {
		return nil, fmt.Errorf("hireclick: board %s: no organisation id on the board page (no such tenant)", e.Board)
	}

	var raw string
	listURL := origin + "/api/Controllers/JobBoard/GetSettingsAndJobs?orgGUID=" + url.QueryEscape(guid)
	if err := s.http.GetJSON(ctx, listURL, &raw); err != nil {
		return nil, fmt.Errorf("hireclick: job list %s: %w", e.Board, err)
	}
	var doc hireclickList
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("hireclick: job list %s: %w", e.Board, err)
	}

	out := make([]hireclickPosting, 0, len(doc.JobList))
	emitted := map[string]bool{}
	for _, j := range doc.JobList {
		id := hireclickJobID(j.JobURL)
		if id == "" || emitted[id] {
			continue // no native id → would collide on the dedup key
		}
		emitted[id] = true
		out = append(out, hireclickPosting{id: id, title: strings.TrimSpace(j.JobTitle), url: j.JobURL})
	}
	return out, nil
}

// hireclickOrgGUID returns the value of the board page's #hdnOrgGuid hidden input, or "".
func hireclickOrgGUID(root *html.Node) string {
	var guid string
	walk(root, func(n *html.Node) bool {
		if guid != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "input" && Attr(n, "id") == "hdnOrgGuid" {
			guid = strings.TrimSpace(Attr(n, "value"))
		}
		return true
	})
	return guid
}

// hireclickJobIDPattern captures the numeric posting id from a "/jb/<slug>/view/<id>" URL. The
// id alone selects the posting (the slug is decorative), and it is unique platform-wide.
var hireclickJobIDPattern = regexp.MustCompile(`/jb/[^/]+/view/(\d+)`)

// hireclickJobID extracts the native posting id from a posting URL, or "" when absent.
func hireclickJobID(u string) string {
	return firstSubmatch(hireclickJobIDPattern, u)
}

// detail fetches one posting page and maps its JobPosting ld+json to a Job. A posting the
// platform has taken down redirects off its "/view/<id>" page and is dropped, as is a 404/410;
// any other failure — including a page that answered without a JobPosting — comes back as an
// unreadableDetail marker, since the page is the only source of the posting's body.
func (s hireclick) detail(ctx context.Context, e CompanyEntry, p hireclickPosting) (Job, bool) {
	root, final, err := s.http.GetHTMLResolved(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		return Job{}, false
	}
	if hireclickJobID(final) != p.id {
		return Job{}, false // redirected to the job board: the posting is gone
	}
	var lp hireclickLD
	if !ldJobPosting(root, &lp) || strings.TrimSpace(lp.Title) == "" {
		return unreadableDetail(p.id, p.url, e.Company), true
	}

	// Trimmed part by part: the locality arrives padded ("WILLISTON ") on some tenants.
	addr := lp.JobLocation.Address
	country := strings.TrimSpace(addr.AddressCountry)
	location := joinNonEmpty(strings.TrimSpace(addr.AddressLocality), strings.TrimSpace(addr.AddressRegion), country)
	var employmentType string
	if len(lp.EmploymentType) > 0 {
		employmentType = schemaEmploymentType(lp.EmploymentType[0])
	}
	return Job{
		ExternalID:  p.id,
		URL:         p.url,
		Title:       strings.TrimSpace(lp.Title),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(lp.Description),
		// No structured work-arrangement field exists, so the location text is the only remote
		// signal, and WorkMode stays empty for the pipeline's own location parse.
		Remote:         isRemote(location),
		PostedAt:       parseLayout("1/2/2006 3:04:05 PM", strings.TrimSpace(lp.DatePosted)),
		EmploymentType: employmentType,
		Countries:      countryFromCode(country),
	}, true
}

// hireclickLD is the schema.org JobPosting a HireClick posting page carries. datePosted is
// written in the US locale ("9/16/2026 10:58:32 AM") with no zone; employmentType is an array
// of one value spelled "Full_Time"/"Part_Time", which schemaEmploymentType folds. The employer
// is the board's configured company: the ld+json's hiringOrganization is the tenant account's
// own legal name, often upper-cased ("WILLIAMS COUNTY"), and a HireClick tenant is one employer.
type hireclickLD struct {
	Title          string      `json:"title"`
	Description    string      `json:"description"`
	DatePosted     string      `json:"datePosted"`
	EmploymentType []string    `json:"employmentType"`
	JobLocation    schemaPlace `json:"jobLocation"`
}
