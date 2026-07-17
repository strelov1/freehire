package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// avito adapts Avito's career site (career.avito.com): a single-company server-rendered
// site with no per-tenant board id (boardless). Its vacancies are enumerated from the
// public sitemap index — fetch the index, fetch each sub-sitemap, keep the vacancy-detail
// URLs (/vacancies/<category>/<id>/) — and each vacancy page carries a schema.org
// JobPosting ld+json block, so the description comes from a per-job detail fetch
// (bounded-concurrency), like the other sitemap+ld+json adapters (radancy, successfactors).
type avito struct {
	http avitoHTTP
}

// avitoHTTP is the transport avito needs: an XML sitemap plus HTML detail pages.
type avitoHTTP interface {
	XMLGetter
	HTMLGetter
}

const avitoSitemapIndexURL = "https://career.avito.com/sitemap.xml"

// NewAvito builds the Avito adapter over the given HTTP client.
func NewAvito(c avitoHTTP) Source { return avito{http: c} }

func (avito) Provider() string { return "avito" }

// avito is single-company, so its config entries carry no board.
func (avito) boardless() {}

// avitoLoc is one <loc> of either the sitemap index or a sub-sitemap.
type avitoLoc struct {
	Loc string `xml:"loc"`
}

func (a avito) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var index struct {
		Sitemaps []avitoLoc `xml:"sitemap"`
	}
	if err := a.http.GetXML(ctx, avitoSitemapIndexURL, &index); err != nil {
		return nil, fmt.Errorf("avito: sitemap index: %w", err)
	}

	// Keep every vacancy-detail loc across all sub-sitemaps, deduplicated by the numeric
	// vacancy id (the same vacancy can appear under several category paths). A failed
	// sub-sitemap is skipped rather than aborting the whole crawl.
	var urls []string
	seen := make(map[string]bool)
	for _, sm := range index.Sitemaps {
		var sub struct {
			URLs []avitoLoc `xml:"url"`
		}
		if err := a.http.GetXML(ctx, sm.Loc, &sub); err != nil {
			continue
		}
		for _, u := range sub.URLs {
			id := avitoVacancyID(u.Loc)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			urls = append(urls, u.Loc)
		}
	}

	// Each vacancy's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return a.detail(ctx, e, u)
	}), nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job, returning
// ok=false when the URL has no parseable id, the fetch fails, or the page carries no
// JobPosting, so the caller skips just that vacancy.
func (a avito) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := avitoVacancyID(loc)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := a.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p avitoPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	// Avito's ld+json addressLocality reports the HQ city (Москва) even for remote roles;
	// the authoritative display city is the page <title>'s "в городе <city>" suffix. Fall
	// back to addressLocality when that suffix is absent.
	location := firstNonEmpty(avitoTitleCity(root), p.JobLocation.Address.AddressLocality)

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// Avito's JobPosting omits jobLocationType (verified across pages, incl. remote
		// roles), so unlike radancy there is no structured remote enum to read — the title's
		// "Удалённая работа" city is the only signal. isRemote matches the Russian stem "удал".
		Remote: isRemote(location) || isRemote(p.Title),
		// datePosted is RFC3339 with a +03:00 offset.
		PostedAt: parseRFC3339(p.DatePosted),
	}, true
}

// avitoPosting is the schema.org JobPosting decoded from an Avito vacancy page's ld+json.
// jobLocation is a single Place whose address is a single PostalAddress; the identifier
// field holds the category name (not the vacancy id), so it is not read.
type avitoPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// avitoVacancyIDPattern captures the numeric vacancy id from a /vacancies/<category>/<id>
// detail URL. The category segment and a trailing numeric id are both required, so
// category and landing pages (no trailing id) are not mistaken for vacancies.
var avitoVacancyIDPattern = regexp.MustCompile(`/vacancies/[^/]+/(\d+)`)

// avitoVacancyID extracts the numeric vacancy id from a detail URL, or "" when the URL is
// not a vacancy-detail page.
func avitoVacancyID(loc string) string {
	return firstSubmatch(avitoVacancyIDPattern, loc)
}

// avitoTitleCityPattern captures the display city from a page <title>'s
// "… в городе <city>" suffix (e.g. "Удалённая работа", "Москва").
var avitoTitleCityPattern = regexp.MustCompile(`в городе\s+(.+?)\s*$`)

// avitoTitleCity returns the display city parsed from the page <title>'s "в городе"
// suffix, or "" when the page has no such title.
func avitoTitleCity(root *html.Node) string {
	var title string
	walk(root, func(n *html.Node) bool {
		if title != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			title = textContent(n)
			return false
		}
		return true
	})
	return firstSubmatch(avitoTitleCityPattern, title)
}
