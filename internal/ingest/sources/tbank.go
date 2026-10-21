package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// tbank adapts T-Bank's careers site, a single-company source with no per-tenant board id
// (boardless). The listing is the careers sitemaps — one shard per section (it_N, back_office_N,
// publisher_N), each vacancy listed once per city it is open in — and each vacancy's description
// comes from its own POST to the careers API, fanned out like the other detail-fetching adapters.
//
// The listing used to be the API's getVacancies. On 2026-10 T-Bank moved it behind a server-built
// GraphQL filter (`filters.generatedGraphQL`) that no plain request supplies, and the endpoint went
// on answering "OK" with zero vacancies, so the provider looked healthy while collecting nothing.
// The sitemaps list the same vacancies (716 unique on 2026-10-21) with the id the description
// endpoint takes and the section that names its category.
type tbank struct {
	http tbankHTTP
}

// tbankHTTP is the transport tbank needs: the sitemaps and the description POST.
type tbankHTTP interface {
	XMLGetter
	JSONPoster
}

const (
	tbankSitemapIndexURL = "https://www.tbank.ru/storage/career/sitemaps/sitemap.xml"
	tbankDetailURL       = "https://www.tbank.ru/pfpjobs/papi/getVacancyDescription"
	tbankSource          = "publisher"
	// tbankTimeout bounds one request. The sitemaps are slow to serve — measured 2026-10-21,
	// back_office_0.xml took 33 s for 188 KB and it_0.xml 12.5 s for 3.9 MB.
	tbankTimeout = 90 * time.Second
	// tbankMaxCitiesNamed bounds how many of a vacancy's cities its location names; a
	// representative vacancy is listed in up to ~290.
	tbankMaxCitiesNamed = 3
)

// tbankShardPattern selects the vacancy shards of the sitemap index; the others (all.xml,
// catalog.xml) are site and catalogue pages.
var tbankShardPattern = regexp.MustCompile(`/(it|back_office|publisher)_\d+\.xml$`)

// tbankVacancyPattern reads a vacancy URL: /career/<section>/vacancy/<city>/<seo-slug>/<id>/.
var tbankVacancyPattern = regexp.MustCompile(`^https://www\.tbank\.ru/career/([a-z-]+)/vacancy/([^/]+)/[^/]+/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/$`)

// tbankSectionCategory maps a URL section to the category the description endpoint takes.
var tbankSectionCategory = map[string]string{
	"it":          "tcareer_it",
	"back-office": "tcareer_back_office",
	"service":     "tcareer_work_with_clients",
}

// tbankDetailRequest is the getVacancyDescription POST body, keyed by a vacancy's id and its
// category.
type tbankDetailRequest struct {
	Source  string `json:"source"`
	URLSlug string `json:"urlSlug"`
	Options struct {
		Category string `json:"category"`
	} `json:"options"`
}

// tbankVacancy is one vacancy as the sitemaps list it: its id, category, first listed URL and
// every city it is open in, in listing order.
type tbankVacancy struct {
	id       string
	category string
	url      string
	cities   []string
}

// NewTBank builds the T-Bank adapter over the given HTTP client.
func NewTBank(c tbankHTTP) Source { return tbank{http: c} }

func (tbank) Provider() string { return "tbank" }

// tbank is single-company, so its config entries carry no board.
func (tbank) boardless() {}

func (b tbank) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	vacancies, err := b.list(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v tbankVacancy) (Job, bool) {
		return b.detail(ctx, e, v)
	}), nil
}

// list reads every vacancy shard and returns each vacancy once. A shard that cannot be read
// fails the listing — tbank is swept, so a missing shard would close every vacancy in it — and
// so does a listing with no vacancy at all, which is never the truth for this employer and is
// what the old API's silent "OK, zero" looked like.
func (b tbank) list(ctx context.Context) ([]tbankVacancy, error) {
	index, err := getSitemap(ctx, b.http, tbankSitemapIndexURL)
	if err != nil {
		return nil, fmt.Errorf("tbank: sitemap index: %w", err)
	}
	byID := map[string]*tbankVacancy{}
	var order []string
	for _, s := range index.Sitemaps {
		if !tbankShardPattern.MatchString(s.Loc) {
			continue
		}
		shard, err := getSitemap(ctx, b.http, s.Loc)
		if err != nil {
			return nil, fmt.Errorf("tbank: shard %s: %w", s.Loc, err)
		}
		for _, u := range shard.URLs {
			m := tbankVacancyPattern.FindStringSubmatch(strings.TrimSpace(u.Loc))
			if m == nil {
				continue
			}
			category, ok := tbankSectionCategory[m[1]]
			if !ok {
				continue
			}
			v, seen := byID[m[3]]
			if !seen {
				v = &tbankVacancy{id: m[3], category: category, url: m[0]}
				byID[m[3]] = v
				order = append(order, m[3])
			}
			v.cities = append(v.cities, m[2])
		}
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("tbank: the sitemaps list no vacancies")
	}
	out := make([]tbankVacancy, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// tbankCapitals lead a location when a vacancy is open there, so a nationwide vacancy listed
// alphabetically in ~290 cities does not read as "Abakan, Almetyevsk, …".
var tbankCapitals = []string{"moscow", "saint-petersburg"}

// tbankIDSlug matches a city segment that is an id rather than a city (the sitemaps carry a few).
var tbankIDSlug = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-`)

// tbankLocation names a vacancy's cities from their URL slugs: the capitals first, then the rest
// in listing order, the first few by name and a count of the others.
func tbankLocation(cities []string) string {
	present := map[string]bool{}
	var rest []string
	for _, c := range cities {
		if present[c] || c == "all" || tbankIDSlug.MatchString(c) {
			continue
		}
		present[c] = true
		rest = append(rest, c)
	}
	var named []string
	for _, c := range tbankCapitals {
		if present[c] {
			named = append(named, c)
		}
	}
	for _, c := range rest {
		if !slices.Contains(tbankCapitals, c) {
			named = append(named, c)
		}
	}
	if len(named) == 0 {
		return ""
	}
	show := named
	if len(show) > tbankMaxCitiesNamed {
		show = show[:tbankMaxCitiesNamed]
	}
	parts := make([]string, len(show))
	for i, c := range show {
		parts[i] = tbankCityName(c)
	}
	loc := strings.Join(parts, ", ")
	if more := len(named) - len(show); more > 0 {
		loc += fmt.Sprintf(" and %d more", more)
	}
	return loc
}

// tbankCityName turns a city slug ("nizhniy-novgorod") into words ("Nizhniy Novgorod").
func tbankCityName(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// detail fetches one vacancy's description and maps it to a Job, returning ok=false when the
// request fails or the vacancy is no longer published, so the caller skips just that vacancy.
func (b tbank) detail(ctx context.Context, e CompanyEntry, v tbankVacancy) (Job, bool) {
	req := tbankDetailRequest{Source: tbankSource, URLSlug: v.id}
	req.Options.Category = v.category

	var resp struct {
		Payload struct {
			Title       string       `json:"title"`
			Status      string       `json:"status"`
			Tags        []tbankTag   `json:"tags"`
			Description []tbankBlock `json:"description"`
		} `json:"payload"`
	}
	if err := b.http.PostJSON(ctx, tbankDetailURL, req, &resp); err != nil {
		return Job{}, false
	}
	p := resp.Payload
	if p.Status != "published" || strings.TrimSpace(p.Title) == "" {
		return Job{}, false
	}
	tags := make([]string, len(p.Tags))
	for i, t := range p.Tags {
		tags[i] = t.Text
	}
	return Job{
		ExternalID:  v.id,
		URL:         v.url,
		Title:       strings.TrimSpace(p.Title),
		Company:     e.Company,
		Location:    tbankLocation(v.cities),
		Description: sanitizeHTML(tbankAssembleBlocks(p.Description)),
		Remote:      isRemote(strings.Join(tags, " ")),
	}, true
}

// tbankTag is one of a vacancy's tags (work format, level, schedule).
type tbankTag struct {
	Text string `json:"text"`
}

// tbankBlock is one description block. Content is polymorphic: either an HTML string or an
// array of {title, description} items, so it is held raw and decoded by tbankAssembleBlocks.
type tbankBlock struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
}

// tbankContentItem is one entry of a block whose content is an array.
type tbankContentItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// tbankAssembleBlocks walks the mixed description[] and concatenates it into one HTML string:
// each block's title becomes an <h3>; a string content is appended as-is; an array content's
// items become a <p> title (when present) followed by their description.
func tbankAssembleBlocks(blocks []tbankBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Title != "" {
			b.WriteString("<h3>" + blk.Title + "</h3>")
		}
		if len(blk.Content) == 0 {
			continue
		}
		// Content is either a JSON string (raw HTML) or an array of {title, description}.
		var s string
		if json.Unmarshal(blk.Content, &s) == nil {
			b.WriteString(s)
			continue
		}
		var items []tbankContentItem
		if json.Unmarshal(blk.Content, &items) == nil {
			for _, it := range items {
				if it.Title != "" {
					b.WriteString("<h3>" + it.Title + "</h3>")
				}
				if it.Description != "" {
					b.WriteString("<p>" + it.Description + "</p>")
				}
			}
		}
	}
	return b.String()
}
