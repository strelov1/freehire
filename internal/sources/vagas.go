package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// vagas adapts vagas.com.br, a large Brazilian national job board. It has no per-tenant board:
// a fixed set of tech area listings (/vagas-de-<area>) is crawled, paginated by ?pagina=N, and
// each posting's company comes from the vacancy itself, so it is a boardless aggregator. The
// listing pages are server-rendered HTML enumerating /vagas/v<id>/<slug> links; each job page
// carries a schema.org JobPosting ld+json block, so the description and fields come from a
// per-job detail fetch (bounded concurrency), like the other JSON-LD detail adapters.
type vagas struct {
	http HTMLGetter
}

// NewVagas builds the vagas.com.br adapter over the given HTTP client.
func NewVagas(c HTMLGetter) Source { return vagas{http: c} }

func (vagas) Provider() string { return "vagas" }

// vagas is a national board crawled by fixed area listings, not per company.
func (vagas) boardless() {}

// vagas aggregates postings from many companies, so it stays in the source facet.
func (vagas) aggregator() {}

// vagasAreas are the tech-area listing slugs crawled. vagas.com is a general board, so scope is
// held to IT areas; the slugs overlap (a job appears under several), so postings are de-duplicated
// by their native id across areas before the detail fan-out.
var vagasAreas = []string{
	"vagas-de-tecnologia",
	"vagas-de-programador",
	"vagas-de-desenvolvedor",
}

// vagasMaxPages bounds listing pagination so an area that never returns an empty page cannot loop.
const vagasMaxPages = 60

// vagasBase is the scheme+host that relative job hrefs resolve against; parsed once.
var vagasBase = &url.URL{Scheme: "https", Host: "www.vagas.com.br"}

func (v vagas) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	// Collect distinct job URLs across every area listing, keyed by native id so a posting
	// listed under several areas is fetched once.
	seen := make(map[string]bool)
	var urls []string
	for i, area := range vagasAreas {
		areaURLs, err := v.listArea(ctx, area)
		if err != nil {
			if i == 0 {
				return nil, fmt.Errorf("vagas: listing %s: %w", area, err)
			}
			continue // a later area failing still ingests the ones already gathered
		}
		for _, u := range areaURLs {
			id := vagasJobID(u)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			urls = append(urls, u)
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return v.detail(ctx, u)
	}), nil
}

// listArea enumerates one area's job URLs across its paginated listing, stopping on the first
// page that adds no new links (an empty tail page, or a board that repeats a page for any pagina).
func (v vagas) listArea(ctx context.Context, area string) ([]string, error) {
	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= vagasMaxPages; page++ {
		listURL := fmt.Sprintf("https://www.vagas.com.br/%s?pagina=%d", area, page)
		root, err := v.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break // a later page failing ends enumeration with the URLs gathered so far
		}
		newLinks := 0
		for _, link := range jobLinks(vagasBase, root, func(href string) bool { return vagasJobID(href) != "" }) {
			if !seen[link] {
				seen[link] = true
				urls = append(urls, link)
				newLinks++
			}
		}
		if newLinks == 0 {
			break
		}
	}
	return urls, nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, or has no id, so the caller skips just that posting.
func (v vagas) detail(ctx context.Context, jobURL string) (Job, bool) {
	id := vagasJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := v.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p vagasPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := joinNonEmpty(p.JobLocation.Address.AddressLocality,
		p.JobLocation.Address.AddressRegion, p.JobLocation.Address.AddressCountry)
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// vagasJobIDPattern captures the native posting id from a /vagas/v<id>/<slug> URL.
var vagasJobIDPattern = regexp.MustCompile(`/vagas/v(\d+)`)

// vagasJobID extracts the native posting id from a job URL, or "" when the URL is not a job link.
func vagasJobID(u string) string {
	if m := vagasJobIDPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// vagasPosting is the schema.org JobPosting decoded from a vagas.com job page. identifier.value
// is the native id but is redundant with the URL's v<id>, so only the display fields are read.
type vagasPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}
