package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
)

// gupyJobsURL is Gupy's public job-publication API — the one every career page
// (<subdomain>.gupy.io) renders from. Gupy is the dominant Brazilian ATS (Creditas, Afya,
// Cogna, Omie, …). Queried with ?subdomain=<board> it lists that career page's published
// postings; /<id> is one posting's sectioned detail.
//
// The board is the career-page SUBDOMAIN, not the numeric companyId it used to be: the
// companyId-keyed listing (employability-portal.gupy.io/api/v1/jobs) began answering an nginx
// 404 to every query on 2026-09-30, and nothing public lists by companyId any more (the
// portal's own /api/job-search/jobs ignores a companyId parameter). One company may own
// several career pages — an internal-mobility page, a confidential one — and each is its own
// board. A one-time rekey moved the catalog and the stored postings over.
const gupyJobsURL = "https://private-api.gupy.io/job-publication/public/jobs"

// gupy adapts Gupy career pages. The listing carries title, address and workplace type but
// no body, so each posting's description comes from its detail — the posting's only source,
// read once: as a HydratingSource it fetches the detail only for postings not yet stored.
type gupy struct {
	http JSONGetter
}

// NewGupy builds the Gupy adapter over the given HTTP client.
func NewGupy(c JSONGetter) Source { return gupy{http: c} }

func (gupy) Provider() string { return "gupy" }

// fullBoardListing: the career-page listing is one unpaginated response carrying every
// published posting (measured 2026-10-19: 3,486 distinct postings on restaurantemc in one
// answer), and a failed listing fails the Fetch. See the fullBoardListing interface for the bar.
func (gupy) fullBoardListing() {}

// gupyPosting is one entry of a career page's listing.
type gupyPosting struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	AddressCity           string `json:"addressCity"`
	AddressStateShortName string `json:"addressStateShortName"`
	AddressCountry        string `json:"addressCountry"`
	RemoteWorking         bool   `json:"remoteWorking"`
	WorkplaceType         string `json:"workplaceType"`
}

func (g gupy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := g.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p gupyPosting) (Job, bool) {
		return g.detail(ctx, e, p)
	}), nil
}

// FetchNew is Fetch that spends a detail request only on postings the catalogue does not hold.
// The detail is this adapter's whole cost: re-reading every stored body on every run was ~72k
// detail requests across 3,324 career pages (2026-10-19), which no run could finish. A seen
// posting is re-listed as a SeenRefresh carrying the listing's identity — the listing itself
// proves the posting is still published — so its liveness refreshes and its hydrated body and
// facets are kept.
func (g gupy) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := g.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p gupyPosting) (Job, bool) {
		if id := strconv.FormatInt(p.ID, 10); seen(id) {
			return Job{
				ExternalID:  id,
				Title:       strings.TrimSpace(p.Name),
				Company:     e.Company,
				SeenRefresh: true,
			}, true
		}
		return g.detail(ctx, e, p)
	}), nil
}

// list reads a career page's whole listing, one unpaginated answer. A failure fails the board
// — never an empty board, which would license a sweep.
func (g gupy) list(ctx context.Context, board string) ([]gupyPosting, error) {
	var resp struct {
		Data []gupyPosting `json:"data"`
	}
	listURL := gupyJobsURL + "?subdomain=" + url.QueryEscape(board)
	if err := g.http.GetJSON(ctx, listURL, &resp); err != nil {
		return nil, fmt.Errorf("gupy: list career page %s: %w", board, err)
	}
	return resp.Data, nil
}

// gupyDetail is a posting's structured detail. Gupy splits a job into fixed sections;
// concatenating them in board order reproduces the full description with its markup.
type gupyDetail struct {
	Description         string `json:"description"`         // intro / "about the role"
	Responsibilities    string `json:"responsibilities"`    // Responsabilidades e atribuições
	Prerequisites       string `json:"prerequisites"`       // Requisitos e qualificações
	RelevantExperiences string `json:"relevantExperiences"` // Informações adicionais / benefits
	PublishedAt         string `json:"publishedAt"`
}

// detail fetches one posting's detail and maps it, with the listing's fields, to a Job. A 404
// is Gupy's own "Vaga inexistente" and drops the posting; any other failure, or a detail with
// no body text, is an unreadableDetail marker — the detail is the only source of the body,
// and a dropped posting is indistinguishable from one taken down.
func (g gupy) detail(ctx context.Context, e CompanyEntry, p gupyPosting) (Job, bool) {
	id := strconv.FormatInt(p.ID, 10)
	jobURL := fmt.Sprintf("https://%s.gupy.io/jobs/%s", e.Board, id)

	var d gupyDetail
	if err := g.http.GetJSON(ctx, gupyJobsURL+"/"+id, &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, jobURL, e.Company), true
		}
		return Job{}, false
	}
	body := gupySections(d)
	if body == "" {
		return unreadableDetail(id, jobURL, e.Company), true
	}
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       strings.TrimSpace(p.Name),
		Company:     e.Company,
		Location:    joinNonEmpty(p.AddressCity, p.AddressStateShortName, p.AddressCountry),
		Description: sanitizeHTML(body),
		Remote:      p.RemoteWorking,
		WorkMode:    workplaceTypeMode(p.WorkplaceType),
		PostedAt:    parseRFC3339(d.PublishedAt),
	}, true
}

// gupySections concatenates the detail's section fragments in display order, skipping any
// that carry no real text (Gupy commonly emits a "<p>.</p>" placeholder intro). It returns
// "" when every section is empty.
func gupySections(d gupyDetail) string {
	var b strings.Builder
	for _, section := range []string{d.Description, d.Responsibilities, d.Prerequisites, d.RelevantExperiences} {
		if gupyHasText(section) {
			b.WriteString(section)
		}
	}
	return b.String()
}

// gupyTextPolicy strips all markup, leaving only text — used to tell a real section from a
// placeholder like "<p>.</p>" or "<p>&nbsp;</p>". Compiled once, safe for concurrent use.
var gupyTextPolicy = bluemonday.StrictPolicy()

// gupyHasText reports whether an HTML fragment contains any letter or digit once tags and
// entities are stripped, so whitespace/punctuation-only sections are treated as empty.
func gupyHasText(fragment string) bool {
	for _, r := range html.UnescapeString(gupyTextPolicy.Sanitize(fragment)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
