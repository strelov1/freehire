package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// habrCareer adapts Habr Career (career.habr.com), a Russian-language IT job board. Its public,
// keyless listing API (/api/frontend/vacancies) returns a paginated list where every item
// carries its own employer, so one paged crawl assembles every Job — but the list omits the
// description, so each vacancy is enriched from its detail page's JobPosting ld+json (the same
// block the linksource adapter reads; see ParseHabrPosting). Unlike a single-company board the
// employer comes from the listing item, so its boardless config entry's company is only a
// validation placeholder.
//
// Coverage ceiling: the API reports ~974 vacancies but paginates only to ~748; the cap sits on
// the result-set itself (every sort ordering, the s[]/qid filters, and the RSS feed return the
// same ids), so the unreachable remainder needs an authenticated session and is out of scope.
type habrCareer struct {
	http habrCareerHTTP
}

// habrCareerHTTP is the transport habr_career needs: a JSON listing (with custom headers) plus
// HTML detail pages.
type habrCareerHTTP interface {
	HeaderJSONGetter
	HTMLGetter
}

const (
	habrListURL    = "https://career.habr.com/api/frontend/vacancies?type=all&sort=date&page=%d"
	habrVacancyURL = "https://career.habr.com/vacancies/%d"
	// habrMaxPages bounds pagination so a wrong or missing meta.totalPages cannot loop. The API
	// caps the listing at ~30 pages (the ~748 reachable vacancies of the package-doc ceiling), so
	// 50 is a safe headroom over that cap.
	habrMaxPages = 50
)

// habrHeaders mirror what a browser sends to the JSON API; Habr serves the listing without them
// but the public Career.habr-parser project sends them and they reduce block risk.
var habrHeaders = map[string]string{
	"Accept":  "application/json",
	"Referer": "https://career.habr.com/vacancies",
}

// NewHabrCareer builds the Habr Career adapter over the given HTTP client.
func NewHabrCareer(c habrCareerHTTP) Source { return habrCareer{http: c} }

func (habrCareer) Provider() string { return "habr_career" }

// habr_career has one global listing, so its config entries carry no board.
func (habrCareer) boardless() {}

// habr_career aggregates postings from many companies, so it stays in the source facet.
func (habrCareer) aggregator() {}

// habrListResponse is one /api/frontend/vacancies page: List is the page, Meta.TotalPages bounds
// pagination.
type habrListResponse struct {
	List []habrVacancy `json:"list"`
	Meta struct {
		CurrentPage int `json:"currentPage"`
		TotalPages  int `json:"totalPages"`
	} `json:"meta"`
}

// habrVacancy is one listing item. Company nests the employer's own title (the board lists many
// employers); the description is not here — it lives on the detail page (see description).
type habrVacancy struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	RemoteWork    bool   `json:"remoteWork"`
	PublishedDate struct {
		Date string `json:"date"`
	} `json:"publishedDate"`
	Company struct {
		Title string `json:"title"`
	} `json:"company"`
	Locations []habrLocation `json:"locations"`
}

// habrLocation is one of a vacancy's listed locations.
type habrLocation struct {
	Title string `json:"title"`
}

func (h habrCareer) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var vacancies []habrVacancy
	for page := 1; page <= habrMaxPages; page++ {
		var resp habrListResponse
		if err := h.http.GetJSONWithHeaders(ctx, fmt.Sprintf(habrListURL, page), habrHeaders, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("habr_career: list page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the vacancies gathered so far
		}
		if len(resp.List) == 0 {
			break
		}
		vacancies = append(vacancies, resp.List...)
		if resp.Meta.TotalPages > 0 && page >= resp.Meta.TotalPages {
			break
		}
	}

	// A vacancy is always yielded; only its description depends on the detail fetch, so the
	// detail closure never returns ok=false (a missing description must not drop the vacancy).
	return fetchDetails(vacancies, defaultDetailWorkers, func(v habrVacancy) (Job, bool) {
		return h.toJob(ctx, v), true
	}), nil
}

// toJob maps a listing item to a Job. ExternalID/URL match the linksource adapter exactly so the
// same vacancy crawled here and followed from a Telegram link dedups into one row. The posted
// date comes from the listing's publishedDate.date (an RFC 3339 timestamp), not the detail
// page's basic-date element (which is the page render time).
func (h habrCareer) toJob(ctx context.Context, v habrVacancy) Job {
	mode := ""
	if v.RemoteWork {
		mode = "remote"
	}
	return Job{
		ExternalID:  strconv.Itoa(v.ID),
		URL:         fmt.Sprintf(habrVacancyURL, v.ID),
		Title:       v.Title,
		Company:     v.Company.Title,
		Location:    distinctJoin(v.Locations, ", ", func(l habrLocation) string { return l.Title }),
		Description: h.description(ctx, v.ID),
		Remote:      v.RemoteWork,
		WorkMode:    mode,
		PostedAt:    parseRFC3339(v.PublishedDate.Date),
	}
}

// description fetches the vacancy detail page and returns its JobPosting description, sanitized.
// A failed fetch or a page without a JobPosting yields an empty description (the vacancy is still
// kept) rather than dropping the vacancy.
func (h habrCareer) description(ctx context.Context, id int) string {
	node, err := h.http.GetHTML(ctx, fmt.Sprintf(habrVacancyURL, id))
	if err != nil {
		return ""
	}
	p, ok := ParseHabrPosting(node)
	if !ok {
		return ""
	}
	return sanitizeHTML(p.Description)
}

// HabrPosting is the schema.org JobPosting a Habr Career vacancy page carries, flattened into the
// fields both the board adapter (internal/sources) and the link-following adapter
// (internal/linksource) need, so the two parse a Habr detail page identically. Description is raw
// HTML; callers sanitize it.
type HabrPosting struct {
	Title       string
	Description string
	Company     string
	Location    string
	Identifier  string
}

// ParseHabrPosting reads the JobPosting ld+json from a Habr vacancy page into a HabrPosting,
// returning ok=false when the page carries no JobPosting block.
func ParseHabrPosting(root *html.Node) (HabrPosting, bool) {
	var raw habrRawPosting
	if !ldJobPosting(root, &raw) {
		return HabrPosting{}, false
	}
	p := HabrPosting{
		Title:       raw.Title,
		Description: raw.Description,
		Company:     raw.HiringOrganization.Name,
		Identifier:  raw.Identifier.Value,
	}
	if len(raw.JobLocation) > 0 {
		p.Location = raw.JobLocation[0].Address.Text
	}
	return p, true
}

// habrRawPosting selects the JobPosting fields Habr publishes on a vacancy page. datePosted is
// deliberately absent: it is unreliable (it runs ahead of the real publish date), so callers
// read the posting date from elsewhere (the board adapter from the listing, the linksource
// adapter from the page's <time> element).
type habrRawPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Identifier  struct {
		Value string `json:"value"`
	} `json:"identifier"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation []struct {
		Address habrAddress `json:"address"`
	} `json:"jobLocation"`
}

// habrAddress reads jobLocation.address, which Habr emits as a bare string but schema.org types
// as a PostalAddress object — so it accepts either shape.
type habrAddress struct{ Text string }

func (a *habrAddress) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		a.Text = s
		return nil
	}
	var o struct {
		AddressLocality string `json:"addressLocality"`
		AddressCountry  string `json:"addressCountry"`
	}
	if json.Unmarshal(b, &o) == nil {
		a.Text = strings.TrimSpace(o.AddressLocality + ", " + o.AddressCountry)
		a.Text = strings.TrimPrefix(a.Text, ", ")
		a.Text = strings.TrimSuffix(a.Text, ", ")
	}
	return nil
}
