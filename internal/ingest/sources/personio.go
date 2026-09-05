package sources

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// personio adapts the Personio public XML feed. Each board is its own subdomain and
// publishes every open position in one document, with the body inline across one or
// more jobDescription HTML values — so no per-posting detail request is needed. The
// feed carries no posting URL, so the adapter builds one from the board and position id.
// personioHTTP is the transport personio needs: an XML feed plus HTML detail pages.
type personioHTTP interface {
	XMLGetter
	HTMLGetter
}

type personio struct {
	http personioHTTP
}

// NewPersonio builds the Personio adapter over the given HTTP client.
func NewPersonio(c personioHTTP) Source { return personio{http: c} }

func (personio) Provider() string { return "personio" }

// fullBoardListing: the default-locale feed carries every open position in one document (no
// pagination) — the English-feed and detail-page fallbacks are best-effort and only affect
// body richness, never whether a position is counted. See the fullBoardListing interface for
// the bar.
func (personio) fullBoardListing() {}

// personioPosition is one open position in a board's XML feed. Personio splits the body
// across one or more jobDescription HTML values, concatenated by body().
type personioPosition struct {
	ID           string `xml:"id"`
	Office       string `xml:"office"`
	Name         string `xml:"name"`
	CreatedAt    string `xml:"createdAt"`
	Descriptions []struct {
		Value string `xml:"value"`
	} `xml:"jobDescriptions>jobDescription"`
}

func (pos personioPosition) body() string {
	var b strings.Builder
	for _, d := range pos.Descriptions {
		b.WriteString(d.Value)
	}
	return b.String()
}

func (p personio) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host := fmt.Sprintf("https://%s.jobs.personio.com", e.Board)

	positions, err := p.feed(ctx, host, "")
	if err != nil {
		return nil, fmt.Errorf("personio: fetch board %s: %w", e.Board, err)
	}

	// The default feed is locale-gated: a posting published only in a non-default locale
	// comes back with an empty body. Fetch the English feed once (best-effort) and index
	// it by id, to fill those gaps.
	enBodies := p.englishBodies(ctx, host, positions)

	jobs := make([]Job, 0, len(positions))
	for _, pos := range positions {
		body := pos.body()
		if body == "" {
			body = enBodies[pos.ID]
		}
		url := fmt.Sprintf("%s/job/%s", host, pos.ID)
		description := sanitizeHTML(body)
		// Still empty (the posting is in neither the default nor the English feed): some
		// boards' detail pages server-render the body as a schema.org JobPosting, so fall
		// back to that as a last resort.
		if description == "" {
			if d, ok := p.detailDescription(ctx, url); ok {
				description = d
			}
		}
		jobs = append(jobs, Job{
			ExternalID:  pos.ID,
			URL:         url,
			Title:       pos.Name,
			Company:     e.Company,
			Location:    pos.Office,
			Description: description,
			Remote:      isRemote(pos.Office), // the feed has no remote flag
			PostedAt:    parseRFC3339(pos.CreatedAt),
		})
	}
	return jobs, nil
}

// feed fetches a board's XML feed for the given language ("" for the board default) and
// returns its positions.
func (p personio) feed(ctx context.Context, host, lang string) ([]personioPosition, error) {
	url := host + "/xml"
	if lang != "" {
		url += "?language=" + lang
	}
	var resp struct {
		Positions []personioPosition `xml:"position"`
	}
	if err := p.http.GetXML(ctx, url, &resp); err != nil {
		return nil, err
	}
	return resp.Positions, nil
}

// englishBodies returns id→body from the English feed, fetched only when some position's
// default-feed body is empty. A failed English fetch is non-fatal — the caller still has
// the detail-page fallback — so it returns nil on error.
func (p personio) englishBodies(ctx context.Context, host string, positions []personioPosition) map[string]string {
	missing := false
	for _, pos := range positions {
		if pos.body() == "" {
			missing = true
			break
		}
	}
	if !missing {
		return nil
	}
	en, err := p.feed(ctx, host, "en")
	if err != nil {
		return nil
	}
	bodies := make(map[string]string, len(en))
	for _, pos := range en {
		bodies[pos.ID] = pos.body()
	}
	return bodies
}

// detailDescription fetches a posting's detail page and returns its schema.org JobPosting
// body, sanitized, with ok=false when the page fetch fails or carries no such block.
func (p personio) detailDescription(ctx context.Context, url string) (string, bool) {
	root, err := p.http.GetHTML(ctx, url)
	if err != nil {
		return "", false
	}
	var ld struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(root, &ld) || ld.Description == "" {
		return "", false
	}
	return sanitizeHTML(html.UnescapeString(ld.Description)), true
}
