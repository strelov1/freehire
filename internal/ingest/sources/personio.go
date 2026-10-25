package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// personio adapts the Personio public career site. Each board is its own subdomain and
// publishes every open position in one document, with the body inline — so no
// per-posting detail request is needed. That document is the XML feed (/xml) when the
// tenant has it switched on, and search.json when it does not: many tenants disable the
// feed, which then answers 404, while search.json keeps listing the same positions under
// the same ids (measured 2026-10-25: of 146 German tenants, 24 had jobs over XML and 144
// over search.json). Neither carries a posting URL, so the adapter builds one from the
// board and position id — identical on both paths, so a tenant that toggles its feed keeps
// every posting's (source, external_id).
//
// personioHTTP is the transport personio needs: the XML feed, the search.json fallback,
// plus HTML detail pages.
type personioHTTP interface {
	XMLGetter
	JSONGetter
	HTMLGetter
}

type personio struct {
	http personioHTTP
}

// NewPersonio builds the Personio adapter over the given HTTP client.
func NewPersonio(c personioHTTP) Source { return personio{http: c} }

func (personio) Provider() string { return "personio" }

// fullBoardListing: the default-locale feed — or, with the feed switched off, the German
// search.json — carries every open position in one document (no pagination), and its
// failure fails the Fetch. The English and detail-page fallbacks are best-effort and only
// affect body richness, never whether a position is counted. See the fullBoardListing
// interface for the bar.
func (personio) fullBoardListing() {}

// personioPosition is one open position in a board's XML feed (search() builds the same
// shape from search.json). Personio splits the body across one or more jobDescription HTML
// values, concatenated by body().
type personioPosition struct {
	ID           string                `xml:"id"`
	Office       string                `xml:"office"`
	Name         string                `xml:"name"`
	CreatedAt    string                `xml:"createdAt"`
	Descriptions []personioDescription `xml:"jobDescriptions>jobDescription"`
}

type personioDescription struct {
	Value string `xml:"value"`
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
	var enBodies map[string]string
	switch {
	case err == nil:
		// The default feed is locale-gated: a posting published only in a non-default
		// locale comes back with an empty body. Fetch the English feed once (best-effort)
		// and index it by id, to fill those gaps.
		enBodies = p.englishBodies(ctx, host, positions)
	case feedDisabled(err):
		positions, enBodies, err = p.search(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("personio: fetch board %s (xml feed off): %w", e.Board, err)
		}
	default:
		return nil, fmt.Errorf("personio: fetch board %s: %w", e.Board, err)
	}

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

// feedDisabled reports whether the XML feed request failed because the tenant has the
// feed switched off, which Personio answers with a 404 page. Only that case falls back to
// search.json: a 5xx or a timeout is a failure of the board's listing and must surface as
// one rather than be answered by a second listing.
func feedDisabled(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

// personioSearchJob is one position in a board's search.json. description is empty
// unless the request names a locale the posting is published in.
type personioSearchJob struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Office      string `json:"office"`
	Description string `json:"description"`
}

// search lists a board through search.json, for a tenant whose XML feed is switched off.
// The bare endpoint returns no body at all, so it asks for German — the locale of most
// Personio tenants (1394 of 1440 bodies in the 2026-10-25 measurement) — and, best-effort,
// for English when some posting came back without one. It returns the positions in the
// shape the XML feed yields, plus the English bodies keyed by id.
func (p personio) search(ctx context.Context, host string) ([]personioPosition, map[string]string, error) {
	de, err := p.searchLocale(ctx, host, "de")
	if err != nil {
		return nil, nil, err
	}
	positions := make([]personioPosition, 0, len(de))
	missing := false
	for _, j := range de {
		pos := personioPosition{ID: strconv.FormatInt(j.ID, 10), Office: j.Office, Name: j.Name}
		if j.Description != "" {
			pos.Descriptions = []personioDescription{{Value: j.Description}}
		} else {
			missing = true
		}
		positions = append(positions, pos)
	}
	if !missing {
		return positions, nil, nil
	}
	en, err := p.searchLocale(ctx, host, "en")
	if err != nil {
		return positions, nil, nil
	}
	bodies := make(map[string]string, len(en))
	for _, j := range en {
		bodies[strconv.FormatInt(j.ID, 10)] = j.Description
	}
	return positions, bodies, nil
}

func (p personio) searchLocale(ctx context.Context, host, lang string) ([]personioSearchJob, error) {
	var jobs []personioSearchJob
	if err := p.http.GetJSON(ctx, host+"/search.json?language="+lang, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}
