package sources

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// arbeitsagentur adapts the Bundesagentur für Arbeit (Germany's federal employment agency) job
// board. Its jobsuche-service search API is reachable keyless with the well-known public
// X-API-Key "jobboerse-jobsuche"; postings are enumerated by professional field (berufsfeld),
// carried as the board file entry's board. The search payload has no description, so it is
// fetched from the same service's per-posting jobdetails endpoint (keyed by the base64 of the
// posting's own referenznummer). Most results carry an externeURL (re-listed from other boards)
// and are dropped — only the agency's own first-party postings are kept. Multi-company (company
// per posting), board-based.
type arbeitsagentur struct {
	http arbeitsagenturHTTP
}

// arbeitsagenturHTTP is the transport arbeitsagentur needs: a keyed JSON GET, used for both the
// paginated search and the per-posting detail fetch.
type arbeitsagenturHTTP interface {
	GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error
}

const (
	arbeitsagenturSearchURL = "https://rest.arbeitsagentur.de/jobboerse/jobsuche-service/pc/v6/jobs"
	// arbeitsagenturDetailAPIURL is the JSON detail endpoint, keyed by base64(referenznummer).
	arbeitsagenturDetailAPIURL = "https://rest.arbeitsagentur.de/jobboerse/jobsuche-service/pc/v4/jobdetails/"
	// arbeitsagenturDetailPageURL is the public SSR page a posting's own URL points at — distinct
	// from arbeitsagenturDetailAPIURL, which this adapter reads instead of scraping that page.
	arbeitsagenturDetailPageURL = "https://www.arbeitsagentur.de/jobsuche/jobdetail/"
	arbeitsagenturAPIKey        = "jobboerse-jobsuche"
	arbeitsagenturPageSize      = 100
	// arbeitsagenturWithinDays bounds each crawl to a fresh window, keeping the result set well
	// inside the API's page*size ≈ 10 000 pagination depth cap.
	arbeitsagenturWithinDays = 7
	// arbeitsagenturMaxPages backstops the loop at the depth cap (page*size ≤ 10 000).
	arbeitsagenturMaxPages = 10000 / arbeitsagenturPageSize
)

// NewArbeitsagentur builds the Arbeitsagentur adapter over the given client.
func NewArbeitsagentur(c arbeitsagenturHTTP) Source { return arbeitsagentur{http: c} }

func (arbeitsagentur) Provider() string { return "arbeitsagentur" }

// arbeitsagenturSearch is one search-API page.
type arbeitsagenturSearch struct {
	Ergebnisliste []arbeitsagenturPosting `json:"ergebnisliste"`
	MaxErgebnisse int                     `json:"maxErgebnisse"`
}

// arbeitsagenturPosting is one search result. ExterneURL is present when the posting is
// re-listed from another board; such postings are dropped. Homeofficemoeglich is carried
// directly on the search result — unlike the description, it needs no detail fetch.
type arbeitsagenturPosting struct {
	Referenznummer              string                   `json:"referenznummer"`
	StellenangebotsTitel        string                   `json:"stellenangebotsTitel"`
	Firma                       string                   `json:"firma"`
	Stellenlokationen           []arbeitsagenturLokation `json:"stellenlokationen"`
	DatumErsteVeroeffentlichung string                   `json:"datumErsteVeroeffentlichung"`
	ExterneURL                  string                   `json:"externeURL"`
	Homeofficemoeglich          bool                     `json:"homeofficemoeglich"`
}

// arbeitsagenturLokation is one entry of a posting's (possibly multi-site) location list.
type arbeitsagenturLokation struct {
	Adresse arbeitsagenturAdresse `json:"adresse"`
}

type arbeitsagenturAdresse struct {
	Ort    string `json:"ort"`
	Region string `json:"region"`
	Land   string `json:"land"`
}

// arbeitsagenturDetail is the jobdetails endpoint's response — only the field this adapter needs.
type arbeitsagenturDetail struct {
	StellenangebotsBeschreibung string `json:"stellenangebotsBeschreibung"`
}

// description is the sanitized posting body. The Stellenbeschreibung is plain text (newline
// paragraphs, no markup), so it goes through plainTextToHTML — as djinni/lumenalta do — to rebuild
// paragraph structure rather than collapsing into one block when rendered.
func (d arbeitsagenturDetail) description() string {
	return sanitizeHTML(plainTextToHTML(d.StellenangebotsBeschreibung))
}

func (a arbeitsagentur) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var kept []arbeitsagenturPosting
	for page := 1; page <= arbeitsagenturMaxPages; page++ {
		var resp arbeitsagenturSearch
		if err := a.http.GetJSONWithHeaders(ctx, a.searchURL(e.Board, page), arbeitsagenturHeaders(), &resp); err != nil {
			return nil, fmt.Errorf("arbeitsagentur: search board %q page %d: %w", e.Board, page, err)
		}
		for _, p := range resp.Ergebnisliste {
			if strings.TrimSpace(p.ExterneURL) == "" && p.Referenznummer != "" {
				kept = append(kept, p) // first-party only
			}
		}
		if len(resp.Ergebnisliste) < arbeitsagenturPageSize || page*arbeitsagenturPageSize >= resp.MaxErgebnisse {
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
	d := a.detail(ctx, p.Referenznummer)
	return Job{
		ExternalID:  p.Referenznummer,
		URL:         arbeitsagenturDetailPageURL + p.Referenznummer,
		Title:       strings.TrimSpace(p.StellenangebotsTitel),
		Company:     strings.TrimSpace(p.Firma),
		Location:    arbeitsagenturLocation(p.Stellenlokationen),
		Description: d.description(),
		Remote:      p.Homeofficemoeglich,
		WorkMode:    workModeFromRemote(p.Homeofficemoeglich),
		PostedAt:    arbeitsagenturDate(p.DatumErsteVeroeffentlichung),
	}
}

// detail fetches the posting's description from the jobdetails JSON endpoint, keyed by the
// base64 of its own referenznummer. Any failure (fetch error, bad JSON) yields the zero detail
// rather than an error — the posting is still worth emitting, just without a description.
func (a arbeitsagentur) detail(ctx context.Context, refnr string) arbeitsagenturDetail {
	url := arbeitsagenturDetailAPIURL + base64.StdEncoding.EncodeToString([]byte(refnr))
	var d arbeitsagenturDetail
	if err := a.http.GetJSONWithHeaders(ctx, url, arbeitsagenturHeaders(), &d); err != nil {
		return arbeitsagenturDetail{}
	}
	return d
}

// arbeitsagenturHeaders is the X-API-Key header both the search and detail requests carry.
func arbeitsagenturHeaders() map[string]string {
	return map[string]string{"X-API-Key": arbeitsagenturAPIKey}
}

// arbeitsagenturLocation joins the non-empty parts of a posting's first location entry,
// dropping the literal "null" the API emits for absent fields. A posting may list several
// sites; only the first is surfaced, matching what the old single-location API exposed.
func arbeitsagenturLocation(locs []arbeitsagenturLokation) string {
	if len(locs) == 0 {
		return ""
	}
	o := locs[0].Adresse
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
