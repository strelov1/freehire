package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// ptsecurity adapts Positive Technologies' own careers site (ptsecurity.com/about/vacancy/), a
// single-company source with no board id. The site is a Next.js App Router app with no public API
// (its /api paths answer 403), so both halves are read from what the server renders:
//
//   - the listing's RSC flight carries the vacancy-list payload — every vacancy's title, slug,
//     locations, activity group and technology stack, plus the listing's own pagination total;
//   - a vacancy page carries the body as rendered HTML (the data-testid="article-content"
//     element — the flight holds only the React tree built from it) and a schema.org
//     JobPosting ld+json with the employment type, the remote marker and the office addresses.
//
// The site's TLS certificate chains to the Russian Trusted Root CA, so the adapter is handed
// the client from NewRussianTrustedRootClient rather than the shared one.
type ptsecurity struct {
	http HTMLGetter
}

const (
	// ptsecurityListURL is the listing, paged server-side by ?page=N. The /ru-ru/ path the site
	// links answers 308 to this canonical one.
	ptsecurityListURL    = "https://ptsecurity.com/about/vacancy/?page=%d"
	ptsecurityVacancyURL = "https://ptsecurity.com/about/vacancy/%s/"
	// ptsecurityMaxPages bounds the pagination walk against a payload whose pageCount never stops
	// growing.
	ptsecurityMaxPages = 50
)

// NewPtsecurity builds the Positive Technologies adapter over the given HTTP client.
func NewPtsecurity(c HTMLGetter) Source { return ptsecurity{http: c} }

func (ptsecurity) Provider() string { return "ptsecurity" }

// ptsecurity is single-company, so its config entries carry no board.
func (ptsecurity) boardless() {}

// ptsecurityListing is the vacancy-list component's data prop: the vacancies grouped by the
// site's "activity" (a group with id "_no-activity" holds the ungrouped ones), and the
// listing's pagination.
type ptsecurityListing struct {
	Vacancies  []ptsecurityGroup `json:"vacancies"`
	Pagination *struct {
		Page      int `json:"page"`
		PageCount int `json:"pageCount"`
		Total     int `json:"total"`
	} `json:"pagination"`
}

type ptsecurityGroup struct {
	Vacancies []ptsecurityVacancy `json:"vacancies"`
}

// ptsecurityVacancy is one listing entry. The slug is the vacancy's identity and its URL
// segment — numeric for some vacancies, a transliterated title for others.
type ptsecurityVacancy struct {
	Title           string   `json:"title"`
	Slug            string   `json:"slug"`
	IsRemoteVacancy bool     `json:"isRemoteVacancy"`
	Activity        string   `json:"activity"`
	Locations       []string `json:"locations"`
	Stack           struct {
		Data []struct {
			Attributes struct {
				Title string `json:"title"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"stack"`
}

func (p ptsecurity) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	vacancies, err := p.list(ctx)
	if err != nil {
		return nil, err
	}
	jobs := fetchDetails(vacancies, defaultDetailWorkers, func(v ptsecurityVacancy) (Job, bool) {
		return p.detail(ctx, e, v)
	})
	// A run that listed vacancies and read none of them is a failure, not an empty crawl —
	// see AGENTS.md's "a crawl that reads nothing of what it listed must fail".
	if len(vacancies) > 0 && !anyRead(jobs) {
		return nil, fmt.Errorf("ptsecurity: listed %d vacancies and read none of them", len(vacancies))
	}
	return jobs, nil
}

// list walks the listing's pages and returns every vacancy, deduplicated by slug. The listing
// carries no other id, and the slug is the vacancy's URL segment, so it is unique among live
// vacancies: two entries with one slug can only open one page. Even a generic slug ("vacancy") is
// a real route.
//
// The walk is checked against the listing's own total, counting the entries as delivered
// (before the slug dedup, so a repeated entry cannot fail every crawl): a walk that came up
// short would have the sweep close live vacancies it merely did not deliver, so it fails
// instead. A payload that is missing altogether is a markup change and fails the same way; a
// total of zero is the site's own answer and yields nothing.
func (p ptsecurity) list(ctx context.Context) ([]ptsecurityVacancy, error) {
	var out []ptsecurityVacancy
	seen := map[string]bool{}
	total, delivered := 0, 0
	for page := 1; page <= ptsecurityMaxPages; page++ {
		flight, err := fetchFlight(ctx, p.http, fmt.Sprintf(ptsecurityListURL, page))
		if err != nil {
			return nil, fmt.Errorf("ptsecurity: listing page %d: %w", page, err)
		}
		l, err := ptsecurityListingOf(flight)
		if err != nil {
			return nil, fmt.Errorf("ptsecurity: listing page %d: %w", page, err)
		}
		total = l.Pagination.Total
		for _, g := range l.Vacancies {
			for _, v := range g.Vacancies {
				delivered++
				if v.Slug == "" || seen[v.Slug] {
					continue
				}
				seen[v.Slug] = true
				out = append(out, v)
			}
		}
		if page >= l.Pagination.PageCount {
			break
		}
	}
	if delivered < total {
		return nil, fmt.Errorf("ptsecurity: listing delivered %d of its %d vacancies", delivered, total)
	}
	return out, nil
}

// ptsecurityListingOf decodes the vacancy-list payload out of the listing's flight. The
// component's props carry testId "vacancy-list" ahead of its data object, which is the first
// object after that marker.
func ptsecurityListingOf(flight string) (ptsecurityListing, error) {
	var l ptsecurityListing
	raw, ok := bracketSlice(flight, `"testId":"vacancy-list"`, '{', '}')
	if !ok {
		return l, fmt.Errorf("vacancy-list payload not found")
	}
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		return l, fmt.Errorf("decode vacancy-list payload: %w", err)
	}
	if l.Pagination == nil {
		return l, fmt.Errorf("vacancy-list payload carries no pagination")
	}
	return l, nil
}

// ptsecurityPosting is the vacancy page's schema.org JobPosting. datePosted is present but
// empty on every page seen, so it is still decoded for the day the site fills it.
type ptsecurityPosting struct {
	DatePosted      string `json:"datePosted"`
	EmploymentType  string `json:"employmentType"`
	JobLocationType string `json:"jobLocationType"`
	JobLocation     struct {
		Address ptsecurityAddresses `json:"address"`
	} `json:"jobLocation"`
}

// ptsecurityAddresses decodes jobLocation.address, which this site emits as an ARRAY of
// PostalAddress inside one Place (one per office city) where schema.org's usual shape is a
// single object. Both are accepted.
type ptsecurityAddresses []schemaAddress

func (a *ptsecurityAddresses) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if bytes.Equal(trimmed, []byte("null")) {
		*a = nil
		return nil
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []schemaAddress
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return err
		}
		*a = arr
		return nil
	}
	var one schemaAddress
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return err
	}
	*a = []schemaAddress{one}
	return nil
}

// detail reads one vacancy page. A 404/410 is the site's word that the vacancy is gone and
// drops it; any other failure, or a 200 page without its article, yields an unreadable marker,
// since the page is the vacancy's only body and a re-templated page must not read as a mass
// closure. (An unknown slug answers 403 here rather than 404, so a closed vacancy raced by the
// crawl surfaces as a marker — the cautious reading.)
func (p ptsecurity) detail(ctx context.Context, e CompanyEntry, v ptsecurityVacancy) (Job, bool) {
	url := fmt.Sprintf(ptsecurityVacancyURL, v.Slug)
	root, err := p.http.GetHTML(ctx, url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(v.Slug, url, e.Company), true
		}
		return Job{}, false
	}
	article := ptsecurityArticle(root)
	if article == nil {
		return unreadableDetail(v.Slug, url, e.Company), true
	}
	var ld ptsecurityPosting
	ldJobPosting(root, &ld) // optional: a page without it still has its body

	location := distinctJoin(v.Locations, ", ", func(s string) string { return s })
	remoteMarked := strings.EqualFold(ld.JobLocationType, "TELECOMMUTE")
	var countries []string
	for _, a := range ld.JobLocation.Address {
		countries = append(countries, a.AddressCountry)
	}
	stack := make([]string, 0, len(v.Stack.Data))
	for _, s := range v.Stack.Data {
		stack = append(stack, ptsecurityLatinC(s.Attributes.Title))
	}

	return Job{
		ExternalID:  v.Slug,
		URL:         url,
		Title:       strings.TrimSpace(v.Title),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(innerHTML(article)),
		Remote:      v.IsRemoteVacancy || remoteMarked || isRemote(location),
		// jobLocationType is the structured marker, set on exactly the vacancies whose
		// locations include "Удаленная работа". Its absence is not proof of an office role.
		WorkMode:       workModeFromRemote(v.IsRemoteVacancy || remoteMarked),
		EmploymentType: schemaEmploymentType(strings.ReplaceAll(ld.EmploymentType, "-", "_")),
		Category:       ptsecurityCategories[v.Activity],
		Skills:         skilltag.Canonicalize(stack),
		Countries:      countriesFromCodes(countries),
		PostedAt:       parseDate(ld.DatePosted),
	}, true
}

// ptsecurityCategories maps the site's activity groups onto freehire categories. Only groups
// whose every vacancy is the category are mapped: "Продажи/Поддержка продаж" mixes sales with
// presale ENGINEERS, so it is left to the title dictionary.
var ptsecurityCategories = map[string]string{
	"Хакеры": "security",
	"Информационная безопасность": "security",
}

// ptsecurityLatinC rewrites a leading Cyrillic "С" to the Latin "C" it stands for. The site
// spells its C-family stack entries with the Cyrillic letter ("С++", "С#"), which no skill
// dictionary keys.
func ptsecurityLatinC(s string) string {
	if rest, ok := strings.CutPrefix(strings.TrimSpace(s), "С"); ok {
		return "C" + rest
	}
	return s
}

// ptsecurityArticle returns the element holding the vacancy body, or nil when the page has
// none.
func ptsecurityArticle(root *html.Node) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == html.ElementNode && Attr(n, "data-testid") == "article-content" {
			found = n
			return false
		}
		return true
	})
	return found
}

// anyRead reports whether at least one job carries a body this crawl actually read, as opposed
// to an unreadable marker.
func anyRead(jobs []Job) bool {
	for _, j := range jobs {
		if !j.Unreadable {
			return true
		}
	}
	return false
}
