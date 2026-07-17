package sources

import (
	"context"
	"fmt"

	"golang.org/x/net/html"
)

// breezy adapts Breezy career boards. The board is the company's Breezy subdomain (e.g.
// "agbo" for agbo.breezy.hr). The /json listing is fully structured (title, location,
// remote flag, posted date) but omits the description; each position's page is
// server-rendered HTML carrying a schema.org JobPosting ld+json block, so the description
// comes from a per-position detail fetch (bounded-concurrency), like the other detail
// adapters.
// breezyHTTP is the transport breezy needs: a JSON listing plus HTML detail pages.
type breezyHTTP interface {
	JSONGetter
	HTMLGetter
}

type breezy struct {
	http breezyHTTP
}

// NewBreezy builds the Breezy adapter over the given HTTP client.
func NewBreezy(c breezyHTTP) Source { return breezy{http: c} }

func (breezy) Provider() string { return "breezy" }

// breezyPosting is one item from the /json listing. The description is not here — it
// lives on the position page (see detail).
type breezyPosting struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Location struct {
		City    string `json:"city"`
		Country struct {
			Name string `json:"name"`
		} `json:"country"`
		IsRemote bool `json:"is_remote"`
	} `json:"location"`
	PublishedDate string `json:"published_date"`
}

func (b breezy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("https://%s.breezy.hr/json", e.Board)

	var postings []breezyPosting
	if err := b.http.GetJSON(ctx, url, &postings); err != nil {
		return nil, fmt.Errorf("breezy: list board %s: %w", e.Board, err)
	}

	return fetchDetails(postings, defaultDetailWorkers, func(p breezyPosting) (Job, bool) {
		return b.detail(ctx, e, p)
	}), nil
}

// detail fetches one position page for its JobPosting description, mapping the listing
// fields plus that description to a Job. It returns ok=false when the page fetch fails or
// carries no description, so the caller drops just that posting (a description-less job is
// useless to enrichment).
func (b breezy) detail(ctx context.Context, e CompanyEntry, p breezyPosting) (Job, bool) {
	root, err := b.http.GetHTML(ctx, p.URL)
	if err != nil {
		return Job{}, false
	}
	desc, ok := breezyDescription(root)
	if !ok {
		return Job{}, false
	}

	location := joinNonEmpty(p.Location.City, p.Location.Country.Name)
	return Job{
		ExternalID:  p.ID,
		URL:         p.URL,
		Title:       p.Name,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(desc)),
		// is_remote is the authoritative structured signal; isRemote(location) is only a
		// fallback (never the title, which false-positives on "Remote …" role names).
		Remote:   p.Location.IsRemote || isRemote(location),
		WorkMode: workModeFromRemote(p.Location.IsRemote),
		PostedAt: parseRFC3339(p.PublishedDate),
	}, true
}

// breezyDescription returns the description from the page's JobPosting ld+json, with
// ok=false when no such block is present or its description is empty.
func breezyDescription(root *html.Node) (string, bool) {
	var p struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(root, &p) || p.Description == "" {
		return "", false
	}
	return p.Description, true
}
