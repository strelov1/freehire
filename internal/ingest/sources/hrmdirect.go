package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"golang.org/x/net/html"
)

// hrmdirect adapts HRM Direct career sites, which ClearCompany now serves. The board is the
// tenant slug, the one the old "<board>.hrmdirect.com" career site was named by.
//
// Around 2026-10-21 every HRM Direct tenant moved onto ClearCompany: "<board>.hrmdirect.com"
// now answers every path, the old listing and job pages included, with a 302 to
// "apply.clearcompany.com/<board>/redirect", which lands on the hosted career page
// "apply.clearcompany.com/<board>" (measured 2026-10-30 over 25 tenants sampled from the
// catalogue: 24 redirected to a live page, one to the platform's offline page). That page draws
// its postings with a widget script, and the old adapter, which parsed the server-rendered
// listing, read a 200 with no posting link in it as an empty board — so every crawl succeeded
// and listed nothing.
//
// The hosted page renders no posting. Its widget script carries the tenant's career-site id
// (data-site-id), and the widget reads the postings from ClearCompany's public career-site API
// by that id: "careers-api.clearcompany.com/v1/<site id>?pageIndex=N&pageSize=M", a JSON page
// with each posting's title, HTML body, structured locations and posted date, plus the board's
// total. So a crawl is the career page, to learn the id, and the API — and no per-posting
// detail request, since the listing already carries the body. An offline tenant's page answers
// 404, as does the API for an unknown site id, so neither reads as an empty board.
//
// The whole board is ONE API request, because the API's paging cannot be walked. It orders by
// posted date with no tie-break and takes no sort parameter, and each page request orders the
// ties afresh, so a walk lists some postings twice and skips others: measured 2026-10-30 on a
// 7,105-posting board, single walks of 100-posting pages returned 7,064 to 7,080 distinct
// postings, and three walks unioned still reached only 7,094 after nine minutes. A single page
// sized past the total is one query and returns the board whole — the same board answered all
// 7,105, distinct, in 141 s and 41 MB — so the request asks for hrmdirectMaxPostings and is read
// through the streaming client, whose timeout is sized for exactly that.
type hrmdirect struct {
	http hrmdirectHTTP
}

// hrmdirectHTTP is the transport this adapter needs: the career page as HTML, the posting API
// as a stream (a large board's answer outlasts the ordinary client's timeout).
type hrmdirectHTTP interface {
	HTMLGetter
	StreamGetter
}

// NewHRMDirect builds the HRM Direct adapter over the given HTTP client.
func NewHRMDirect(c hrmdirectHTTP) Source { return hrmdirect{http: c} }

func (hrmdirect) Provider() string { return "hrmdirect" }

// fullBoardListing: the board is one request whose distinct postings must reach the total the
// same answer states, or the crawl fails. See the fullBoardListing interface for the bar.
func (hrmdirect) fullBoardListing() {}

// hrmdirectMaxPostings is the page size the single listing request asks for. It is a cap, not a
// cost: the API answers what the board holds. The largest board in the catalogue held 7,105
// postings on 2026-10-30; a board past the cap fails its crawl rather than being truncated.
const hrmdirectMaxPostings = 20000

// hrmdirectPosting is one posting as the career-site API lists it.
type hrmdirectPosting struct {
	ID          string `json:"id"`
	Title       string `json:"positionTitle"`
	Description string `json:"description"`
	PostedDate  string `json:"postedDate"`
	// Location is the platform's own one-line rendering of the locations below ("Remote (US)",
	// "Bronx NY, New York NY").
	Location  string `json:"location"`
	Locations []struct {
		Country  string `json:"country"`
		IsRemote bool   `json:"isRemote"`
	} `json:"locations"`
}

func (s hrmdirect) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	siteID, err := s.siteID(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.list(ctx, e.Board, siteID)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, hrmdirectJob(e, p))
	}
	return jobs, nil
}

// siteID reads the career-site id from the tenant's hosted career page, failing when the page
// names none: without it nothing can be listed, and an empty answer would read as an emptied
// board.
func (s hrmdirect) siteID(ctx context.Context, board string) (string, error) {
	root, err := s.http.GetHTML(ctx, "https://apply.clearcompany.com/"+url.PathEscape(board))
	if err != nil {
		return "", fmt.Errorf("hrmdirect: board %s: career page: %w", board, err)
	}
	var id string
	walk(root, func(n *html.Node) bool {
		if id != "" {
			return false
		}
		if n.Type == html.ElementNode {
			id = strings.TrimSpace(Attr(n, "data-site-id"))
		}
		return id == ""
	})
	if id == "" {
		return "", fmt.Errorf("hrmdirect: board %s: the career page names no career site", board)
	}
	return id, nil
}

// list reads the board's postings in one API request, distinct by id in the API's order, and
// fails unless they reach the total the same answer states.
func (s hrmdirect) list(ctx context.Context, board, siteID string) ([]hrmdirectPosting, error) {
	var resp struct {
		// A pointer, so an answer without the collection is told apart from an empty one.
		Results    *[]hrmdirectPosting `json:"results"`
		TotalCount int                 `json:"totalCount"`
	}
	u := fmt.Sprintf("https://careers-api.clearcompany.com/v1/%s?pageIndex=0&pageSize=%d",
		url.PathEscape(siteID), hrmdirectMaxPostings)
	err := s.http.GetStream(ctx, u, "application/json", func(r io.Reader) error {
		return json.NewDecoder(r).Decode(&resp)
	})
	if err != nil {
		return nil, fmt.Errorf("hrmdirect: board %s: listing: %w", board, err)
	}
	if resp.Results == nil {
		return nil, errListingAbsent("hrmdirect", board, "results")
	}
	var postings []hrmdirectPosting
	seen := map[string]bool{}
	for _, p := range *resp.Results {
		id := strings.ToLower(p.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		postings = append(postings, p)
	}
	if len(postings) < resp.TotalCount {
		return nil, fmt.Errorf("hrmdirect: board %s: listed %d of the %d postings the source states",
			board, len(postings), resp.TotalCount)
	}
	return postings, nil
}

// hrmdirectJob maps one API posting to a Job.
func hrmdirectJob(e CompanyEntry, p hrmdirectPosting) Job {
	id := strings.ToLower(p.ID)
	// A posting open at a remote location as well as an office is open to remote applicants, so
	// any remote location marks it. The flag is the platform's own, never the location text.
	remote := false
	var countries []string
	for _, l := range p.Locations {
		remote = remote || l.IsRemote
		if c := location.NormalizeCountry(l.Country); c != "" && !slices.Contains(countries, c) {
			countries = append(countries, c)
		}
	}
	return Job{
		ExternalID: id,
		// The widget's own permalink for a posting: the career page with the lower-cased id.
		URL:         fmt.Sprintf("https://apply.clearcompany.com/%s?jobId=%s", url.PathEscape(e.Board), url.QueryEscape(id)),
		Title:       strings.TrimSpace(p.Title),
		Company:     e.Company,
		Location:    strings.TrimSpace(p.Location),
		Description: sanitizeHTML(p.Description),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		Countries:   countries,
		PostedAt:    parseLayout("2006-01-02T15:04:05", p.PostedDate),
	}
}
