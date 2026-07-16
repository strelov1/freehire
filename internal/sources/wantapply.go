package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// wantapply adapts the Wantapply job aggregator (wantapply.com). The .com host sits behind a WAF
// that 403s non-browser clients, so this adapter crawls the .cy mirror, which serves identical
// content (same backend — sitemap lastmod matches to the millisecond) without the WAF. Wantapply
// is a directApply aggregator (many employers, no per-tenant board), so the adapter is boardless
// and stays in the source facet, taking each posting's company from the JSON-LD. Its sitemap
// enumerates every vacancy as a single-segment slug page and each page server-renders a schema.org
// JobPosting ld+json — the dataart/successfactors shape (sitemap to enumerate, per-vacancy detail
// fetch). It hydrates detail only for vacancies the catalogue does not already have
// (HydratingSource), and is NOT self-closing: it re-lists all current vacancies each run, so the
// pipeline's unseen-sweep is the close signal (a vacancy that drops out of the sitemap is closed).
type wantapply struct {
	http wantapplyHTTP
}

// wantapplyHTTP is the transport wantapply needs: the XML sitemap plus HTML detail pages.
type wantapplyHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	wantapplyHost       = "https://wantapply.cy"
	wantapplySitemapURL = wantapplyHost + "/sitemap.xml"
	// wantapplyHostname is the sitemap host a vacancy loc must carry (guards against a foreign
	// or malformed URL slipping through as a slug).
	wantapplyHostname = "wantapply.cy"
	// wantapplyDetailWorkers bounds the detail-fetch fan-out below the shared default (8): the
	// .cy mirror throttles a burst of concurrent requests (a prod run at 8 landed only ~32% of
	// the catalogue), while 4 fetches the full sitemap cleanly (~2 min for ~930 vacancies).
	wantapplyDetailWorkers = 4
)

// wantapplyReserved are the single-segment paths that are pages, not vacancies. Multi-segment
// paths (/company/*, /jobs/*) are excluded structurally; these named single-segment pages are not.
var wantapplyReserved = map[string]struct{}{
	"create": {}, "sign-in": {}, "sign-up": {},
	"privacy-policy": {}, "terms-of-service": {},
}

// NewWantapply builds the Wantapply adapter over the given HTTP client.
func NewWantapply(c wantapplyHTTP) Source { return wantapply{http: c} }

func (wantapply) Provider() string { return "wantapply" }

func (wantapply) boardless() {}

func (wantapply) aggregator() {}

// wantapplyVacancy is one candidate vacancy discovered from the sitemap: its slug (the
// ExternalID) and canonical URL.
type wantapplyVacancy struct {
	slug string
	url  string
}

// Fetch is the list-only fallback (used when the pipeline cannot supply a seen set): it fetches
// detail for every current vacancy. FetchNew is the hydrating path ingest prefers.
func (s wantapply) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	vacancies, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, wantapplyDetailWorkers, func(v wantapplyVacancy) (Job, bool) {
		return s.detail(ctx, v)
	}), nil
}

// FetchNew fetches detail only for a vacancy the catalogue does not already have — seen reports
// whether a slug is already ingested. A seen vacancy yields a liveness-refresh job (no detail
// request) so the pipeline refreshes its last-seen/open state WITHOUT rewriting the content
// hydrated when it was new; an unseen vacancy is hydrated from its detail page. Detail fetches
// run under a bounded worker pool, and a single vacancy's detail failure is isolated.
func (s wantapply) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	vacancies, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, wantapplyDetailWorkers, func(v wantapplyVacancy) (Job, bool) {
		if seen(v.slug) {
			// Already ingested: refresh liveness only, no detail request. Just the identity
			// fields the pipeline's touch needs (ExternalID); content is left untouched.
			return Job{ExternalID: v.slug, URL: v.url, SeenRefresh: true}, true
		}
		return s.detail(ctx, v)
	}), nil
}

// crawl reads the sitemap and returns every vacancy candidate (reserved pages, /company/*, and
// /jobs/* excluded) — the shared enumeration behind Fetch and FetchNew.
func (s wantapply) crawl(ctx context.Context) ([]wantapplyVacancy, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := s.http.GetXML(ctx, wantapplySitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("wantapply: sitemap: %w", err)
	}
	var out []wantapplyVacancy
	for _, entry := range sitemap.URLs {
		// Use the sitemap's canonical loc as the URL rather than rebuilding it from the slug —
		// the slug is url.Parse-decoded, so reconstruction could diverge from the real page.
		if slug := wantapplyVacancySlug(entry.Loc); slug != "" {
			out = append(out, wantapplyVacancy{slug: slug, url: entry.Loc})
		}
	}
	return out, nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job, returning ok=false
// when the fetch fails, the page carries no JobPosting (e.g. a closed vacancy's empty page), or
// the posting names no employer — so the caller skips just that vacancy.
func (s wantapply) detail(ctx context.Context, v wantapplyVacancy) (Job, bool) {
	root, err := s.http.GetHTML(ctx, v.url)
	if err != nil {
		return Job{}, false
	}
	var p wantapplyPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	company := strings.TrimSpace(p.HiringOrganization.Name)
	if company == "" {
		return Job{}, false // aggregator: no employer name → cannot normalize
	}
	location := p.location()
	// jobLocationType is the structured work-arrangement signal: TELECOMMUTE means remote. Absent
	// → leave WorkMode empty and fall back to the location text for the remote flag.
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	workMode := ""
	if remote {
		workMode = "remote"
	}
	// employmentType is emitted as an array (["FULL_TIME"]); the first entry is the type.
	employmentType := ""
	if len(p.EmploymentType) > 0 {
		employmentType = p.EmploymentType[0]
	}
	return Job{
		ExternalID:     v.slug,
		URL:            v.url,
		Title:          p.Title,
		Company:        company,
		Location:       location,
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote || isRemote(location),
		WorkMode:       workMode,
		EmploymentType: schemaEmploymentType(employmentType),
		PostedAt:       parseRFC3339(p.DatePosted),
	}, true
}

// wantapplyVacancySlug returns the vacancy slug for a sitemap loc that is a single-segment,
// non-reserved page on the wantapply.cy host, or "" for the root, a reserved static page, a
// multi-segment path (/company/*, /jobs/*), or an unparseable/foreign URL.
func wantapplyVacancySlug(loc string) string {
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil || u.Hostname() != wantapplyHostname {
		return ""
	}
	seg := strings.Trim(u.Path, "/")
	if seg == "" || strings.Contains(seg, "/") {
		return "" // root, or multi-segment (/company/*, /jobs/*)
	}
	if _, reserved := wantapplyReserved[seg]; reserved {
		return ""
	}
	return seg
}

// wantapplyPosting is the schema.org JobPosting decoded from a vacancy page's ld+json.
type wantapplyPosting struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	DatePosted         string   `json:"datePosted"`
	EmploymentType     []string `json:"employmentType"`
	JobLocationType    string   `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation []wantapplyPlace `json:"jobLocation"`
}

type wantapplyPlace struct {
	Address wantapplyAddress `json:"address"`
}

type wantapplyAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressRegion   string `json:"addressRegion"`
	AddressCountry  string `json:"addressCountry"`
}

// location joins each jobLocation place as "City, Region, Country" (skipping empty parts),
// deduped and separated by "; ", so a job open in several places lists them all.
func (p wantapplyPosting) location() string {
	var out []string
	seen := make(map[string]struct{})
	for _, pl := range p.JobLocation {
		s := joinNonEmpty(pl.Address.AddressLocality, pl.Address.AddressRegion, pl.Address.AddressCountry)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return strings.Join(out, "; ")
}
