package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// geekhunter adapts GeekHunter career boards, a Brazilian tech-recruitment ATS. The board is
// the company's GeekHunter slug (e.g. "omega-gestao-educacional-ltda" for
// geekhunter.com/pt/omega-gestao-educacional-ltda/jobs). The listing page server-renders a
// schema.org ItemList ld+json block naming every open posting's URL, and each posting's own
// page carries a full schema.org JobPosting ld+json block — so both the listing and the detail
// come from parsing embedded ld+json rather than a JSON API, like the other ld+json adapters.
type geekhunterHTTP interface {
	HTMLGetter
}

type geekhunter struct {
	http geekhunterHTTP
}

// NewGeekHunter builds the GeekHunter adapter over the given HTTP client.
func NewGeekHunter(c geekhunterHTTP) Source { return geekhunter{http: c} }

func (geekhunter) Provider() string { return "geekhunter" }

const geekhunterBase = "https://www.geekhunter.com/pt"

// fullBoardListing: the board's /jobs page inlines every open posting in one ItemList ld+json
// block (verified live against boards up to 10 postings, matching numberOfItems exactly; no
// pagination observed), so a listing failure aborts the whole Fetch rather than silently
// returning a partial board.
func (geekhunter) fullBoardListing() {}

func (s geekhunter) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	root, err := s.http.GetHTML(ctx, fmt.Sprintf("%s/%s/jobs", geekhunterBase, e.Board))
	if err != nil {
		return nil, fmt.Errorf("geekhunter: list board %s: %w", e.Board, err)
	}
	locs := geekhunterListingURLs(root)
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one posting's page and maps its JobPosting ld+json block to a Job. A page the
// platform reports gone is dropped; one this crawl merely could not read comes back as an
// unreadableDetail marker, since the page is this adapter's only source for the posting.
func (s geekhunter) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := geekhunterJobID(loc)
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, loc, e.Company), true
		}
		return Job{}, false
	}
	var p geekhunterPosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, loc, e.Company), true
	}

	location := ""
	if len(p.JobLocation) > 0 {
		location = p.JobLocation[0].Address.Location()
	}
	// GeekHunter carries no structured remote flag: a fully remote posting simply omits
	// jobLocation entirely (verified live against a "REMOTO" posting, which had no jobLocation
	// at all), so that absence is the structured signal here. isRemote(location) only ever
	// fires for the rare posting whose location string itself mentions "remote" in English —
	// GeekHunter's own wording is Portuguese ("remoto"), which it does not match.
	remote := len(p.JobLocation) == 0

	var employmentType string
	if len(p.EmploymentType) > 0 {
		employmentType = schemaEmploymentType(p.EmploymentType[0])
	}

	return Job{
		ExternalID:     id,
		URL:            loc,
		Title:          p.Title,
		Company:        firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:       location,
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote || isRemote(location),
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: employmentType,
		// GeekHunter emits datePosted as a bare date ("2026-04-02"), not a full timestamp.
		PostedAt: parseRFC3339OrDate(p.DatePosted),
	}, true
}

// geekhunterPosting is the schema.org JobPosting decoded from a GeekHunter job page's
// application/ld+json block.
type geekhunterPosting struct {
	Title              string       `json:"title"`
	Description        string       `json:"description"`
	DatePosted         string       `json:"datePosted"`
	EmploymentType     []string     `json:"employmentType"`
	JobLocation        schemaPlaces `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// geekhunterListingURLs decodes the board listing page's ItemList ld+json block
// (<script id="itemList">) into its posting URLs, in listing order. Returns nil when the block
// is absent or carries no items — an empty listing, not a parse failure, since a board can
// genuinely have zero open postings right now.
func geekhunterListingURLs(root *html.Node) []string {
	raw := scriptTextByID(root, "itemList")
	if raw == "" {
		return nil
	}
	var list struct {
		ItemListElement []struct {
			URL string `json:"url"`
		} `json:"itemListElement"`
	}
	if json.Unmarshal([]byte(raw), &list) != nil {
		return nil
	}
	urls := make([]string, 0, len(list.ItemListElement))
	for _, item := range list.ItemListElement {
		if item.URL != "" {
			urls = append(urls, item.URL)
		}
	}
	return urls
}

// geekhunterJobIDPattern captures the posting slug from a canonical /pt/<company>/jobs/<slug>
// URL — the last path segment, GeekHunter's own stable id for the posting. The page's own
// ld+json identifier is only available after a successful detail fetch, so it cannot serve as
// the marker id for a posting whose page failed to load; the URL-derived slug is stable across
// both outcomes.
var geekhunterJobIDPattern = regexp.MustCompile(`/jobs/([^/?#]+)$`)

// geekhunterJobID extracts the posting slug from a job page URL, or "" when the URL carries
// none (defensive; every URL the ItemList yields is a /jobs/<slug> posting link).
func geekhunterJobID(loc string) string {
	return firstSubmatch(geekhunterJobIDPattern, loc)
}
