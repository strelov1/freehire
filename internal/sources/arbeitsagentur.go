package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// arbeitsagentur adapts the Bundesagentur für Arbeit (Germany's federal employment agency) job
// board. Its jobsuche-service search API is reachable keyless with the well-known public
// X-API-Key "jobboerse-jobsuche"; postings are enumerated by professional field (berufsfeld),
// carried as the board file entry's board. The search payload has no description and the detail
// API is 403, so the description is scraped from the server-rendered public jobdetail page. Most
// results carry an externeUrl (re-listed from other boards) and are dropped — only the agency's
// own first-party postings are kept. Multi-company (company per posting), board-based.
type arbeitsagentur struct {
	http arbeitsagenturHTTP
}

// arbeitsagenturHTTP is the two-stage transport: a keyed JSON search and an SSR detail page fetch.
type arbeitsagenturHTTP interface {
	GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error
	GetHTML(ctx context.Context, url string) (*html.Node, error)
}

const (
	arbeitsagenturSearchURL = "https://rest.arbeitsagentur.de/jobboerse/jobsuche-service/pc/v4/jobs"
	arbeitsagenturDetailURL = "https://www.arbeitsagentur.de/jobsuche/jobdetail/"
	arbeitsagenturAPIKey    = "jobboerse-jobsuche"
	arbeitsagenturPageSize  = 100
	// arbeitsagenturWithinDays bounds each crawl to a fresh window, keeping the result set well
	// inside the API's page*size ≈ 10 000 pagination depth cap.
	arbeitsagenturWithinDays = 7
	// arbeitsagenturMaxPages backstops the loop at the depth cap (page*size ≤ 10 000).
	arbeitsagenturMaxPages = 10000 / arbeitsagenturPageSize
)

// NewArbeitsagentur builds the Arbeitsagentur adapter over the given two-stage client.
func NewArbeitsagentur(c arbeitsagenturHTTP) Source { return arbeitsagentur{http: c} }

func (arbeitsagentur) Provider() string { return "arbeitsagentur" }

// arbeitsagenturSearch is one search-API page.
type arbeitsagenturSearch struct {
	Stellenangebote []arbeitsagenturPosting `json:"stellenangebote"`
	MaxErgebnisse   int                     `json:"maxErgebnisse"`
}

// arbeitsagenturPosting is one search result. externeUrl is present when the posting is re-listed
// from another board; such postings are dropped.
type arbeitsagenturPosting struct {
	Refnr       string            `json:"refnr"`
	Titel       string            `json:"titel"`
	Arbeitgeber string            `json:"arbeitgeber"`
	Arbeitsort  arbeitsagenturOrt `json:"arbeitsort"`
	Datum       string            `json:"aktuelleVeroeffentlichungsdatum"`
	ExterneURL  string            `json:"externeUrl"`
}

type arbeitsagenturOrt struct {
	Ort    string `json:"ort"`
	Region string `json:"region"`
	Land   string `json:"land"`
}

// arbeitsagenturDetail decodes the description out of the jobdetail page's ng-state JSON blob.
type arbeitsagenturDetail struct {
	Jobdetail struct {
		Beschreibung string `json:"stellenangebotsBeschreibung"`
	} `json:"jobdetail"`
}

func (a arbeitsagentur) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	headers := map[string]string{"X-API-Key": arbeitsagenturAPIKey}
	var kept []arbeitsagenturPosting
	for page := 1; page <= arbeitsagenturMaxPages; page++ {
		var resp arbeitsagenturSearch
		if err := a.http.GetJSONWithHeaders(ctx, a.searchURL(e.Board, page), headers, &resp); err != nil {
			return nil, fmt.Errorf("arbeitsagentur: search board %q page %d: %w", e.Board, page, err)
		}
		for _, p := range resp.Stellenangebote {
			if strings.TrimSpace(p.ExterneURL) == "" && p.Refnr != "" {
				kept = append(kept, p) // first-party only
			}
		}
		if len(resp.Stellenangebote) < arbeitsagenturPageSize || page*arbeitsagenturPageSize >= resp.MaxErgebnisse {
			break
		}
	}
	// A missing/failed description does not drop the posting, so the mapper always keeps it.
	return fetchDetails(kept, defaultDetailWorkers, func(p arbeitsagenturPosting) (Job, bool) {
		return a.toJob(ctx, p), true
	}), nil
}

// searchURL builds a berufsfeld search request bounded to the recent-publish window.
func (arbeitsagentur) searchURL(berufsfeld string, page int) string {
	q := url.Values{}
	q.Set("berufsfeld", berufsfeld)
	q.Set("size", strconv.Itoa(arbeitsagenturPageSize))
	q.Set("page", strconv.Itoa(page))
	q.Set("veroeffentlichtseit", strconv.Itoa(arbeitsagenturWithinDays))
	return arbeitsagenturSearchURL + "?" + q.Encode()
}

func (a arbeitsagentur) toJob(ctx context.Context, p arbeitsagenturPosting) Job {
	detailURL := arbeitsagenturDetailURL + p.Refnr
	return Job{
		ExternalID:  p.Refnr,
		URL:         detailURL,
		Title:       strings.TrimSpace(p.Titel),
		Company:     strings.TrimSpace(p.Arbeitgeber),
		Location:    arbeitsagenturLocation(p.Arbeitsort),
		Description: a.description(ctx, detailURL),
		PostedAt:    arbeitsagenturDate(p.Datum),
	}
}

// description scrapes the SSR jobdetail page's ng-state JSON for the Stellenbeschreibung. Any
// failure (fetch error, no ng-state block, bad JSON) yields an empty description rather than an
// error — the posting is still worth emitting. The Stellenbeschreibung is plain text (newline
// paragraphs, no markup), so it goes through plainTextToHTML — as djinni/lumenalta do — to rebuild
// paragraph structure rather than collapsing into one block when rendered.
func (a arbeitsagentur) description(ctx context.Context, detailURL string) string {
	root, err := a.http.GetHTML(ctx, detailURL)
	if err != nil {
		return ""
	}
	blob := scriptTextByID(root, "ng-state")
	if blob == "" {
		return ""
	}
	var d arbeitsagenturDetail
	if err := json.Unmarshal([]byte(blob), &d); err != nil {
		return ""
	}
	return sanitizeHTML(plainTextToHTML(d.Jobdetail.Beschreibung))
}

// arbeitsagenturLocation joins the non-empty parts of an arbeitsort, dropping the literal "null"
// the API emits for absent fields.
func arbeitsagenturLocation(o arbeitsagenturOrt) string {
	parts := make([]string, 0, 3)
	for _, s := range []string{o.Ort, o.Region, o.Land} {
		if s = strings.TrimSpace(s); s != "" && s != "null" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// arbeitsagenturDate parses the "2006-01-02" publish date.
func arbeitsagenturDate(s string) *time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &t
}

// scriptTextByID returns the text of the first <script> element with the given id, or "".
func scriptTextByID(root *html.Node, id string) string {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if a.Key == "id" && a.Val == id {
					found = n
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		return ""
	}
	return textContent(found)
}
