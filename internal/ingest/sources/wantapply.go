package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"
)

// wantapply adapts the Wantapply job aggregator (wantapply.com). The .com host sits behind a WAF
// that 403s non-browser clients, so this adapter crawls the .cy mirror, which serves identical
// content (same backend — sitemap lastmod matches to the millisecond) without the WAF.
//
// The mirroring holds for PAGES and not for the SITEMAPS, which is worth stating because the
// difference is most of the catalogue. Measured 2026-09-07: .cy enumerates ~605 vacancies and
// .com enumerates 2 755, yet five vacancies absent from the .cy sitemap all answered 200 on .cy.
// So .cy can serve the whole catalogue; it just does not advertise it. NewWantapplyViaHostedSitemap
// reads the fuller enumeration from .com and still fetches every page from .cy — one metered
// request for the sitemap, and the 2 755 pages free. Wantapply
// is a directApply aggregator (many employers, no per-tenant board), so the adapter is boardless
// and stays in the source facet, taking each posting's company from the JSON-LD. Its sitemap
// enumerates every vacancy as a single-segment slug page and each page server-renders a schema.org
// JobPosting ld+json — the dataart/successfactors shape (sitemap to enumerate, per-vacancy detail
// fetch). It hydrates detail only for vacancies the catalogue does not already have
// (HydratingSource), and is NOT self-closing: it re-lists all current vacancies each run, so the
// pipeline's unseen-sweep is the close signal (a vacancy that drops out of the sitemap is closed).
type wantapply struct {
	http wantapplyHTTP
	// sitemap is where the enumeration comes from; it is s.http for the ordinary construction
	// and a different transport when the fuller .com sitemap is used.
	sitemap XMLGetter
	// sitemapURL and sitemapHost describe that enumeration: the URL to read and the host its
	// entries must carry. detailHost is where the pages are actually fetched from, which is not
	// the same when the two differ.
	sitemapURL  string
	sitemapHost string
	detailHost  string
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
	// wantapplyDetailWorkers bounds the detail-fetch fan-out below the shared default (8): a burst
	// at 8 is throttled hard (a residential-IP crawl landed ~32% at 8 vs ~100% at 4). Prod egresses
	// through a proxy whose IP the .cy edge rate-caps per window, so one run still lands only part
	// of the catalogue — the HydratingSource re-crawl accretes the rest across subsequent runs.
	wantapplyDetailWorkers = 4
)

// wantapplyReserved are the single-segment paths that are pages, not vacancies. Multi-segment
// paths (/company/*, /jobs/*) are excluded structurally; these named single-segment pages are not.
var wantapplyReserved = map[string]struct{}{
	"create": {}, "sign-in": {}, "sign-up": {},
	"privacy-policy": {}, "terms-of-service": {},
}

// NewWantapply builds the Wantapply adapter over the given HTTP client: sitemap and pages both
// from the .cy mirror.
func NewWantapply(c wantapplyHTTP) Source {
	return wantapply{
		http:        c,
		sitemap:     c,
		sitemapURL:  wantapplySitemapURL,
		sitemapHost: wantapplyHostname,
		detailHost:  wantapplyHost,
	}
}

// NewWantapplyViaHostedSitemap reads the enumeration from sitemapURL through a separate
// transport — the .com sitemap, which needs the hosted tier — while still fetching every page
// through c, the ordinary proxied client on .cy. See the type doc for why that is worth doing:
// the two hosts mirror each other's PAGES, so the only thing the metered request buys is the
// list, and it buys 2 154 vacancies the .cy sitemap never mentions.
func NewWantapplyViaHostedSitemap(c wantapplyHTTP, sitemap XMLGetter, sitemapURL, sitemapHost string) Source {
	return wantapply{
		http:        c,
		sitemap:     sitemap,
		sitemapURL:  sitemapURL,
		sitemapHost: sitemapHost,
		detailHost:  wantapplyHost,
	}
}

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
	jobs, _, err := s.fetchCounting(ctx, func(string) bool { return false })
	return jobs, err
}

// FetchNew fetches detail only for a vacancy the catalogue does not already have — seen reports
// whether a slug is already ingested. A seen vacancy yields a liveness-refresh job (no detail
// request) so the pipeline refreshes its last-seen/open state WITHOUT rewriting the content
// hydrated when it was new; an unseen vacancy is hydrated from its detail page.
func (s wantapply) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	jobs, _, err := s.fetchCounting(ctx, seen)
	return jobs, err
}

// fetchCounting is the shared body, returning how many vacancies were DROPPED alongside the
// jobs. Both entry points discard the count after it is logged; the tests read it.
//
// The count exists because a dropped vacancy never reaches the pipeline — Stats counts saveOne
// failures, not candidates an adapter discarded — so a crawl that enumerated thousands and read
// almost none is indistinguishable from a small source. That is not hypothetical here: when the
// enumeration moved to the .com sitemap the list went from ~605 vacancies to 2 755 and the
// catalogue did not move, with nothing anywhere saying where the rest went.
func (s wantapply) fetchCounting(ctx context.Context, seen func(string) bool) ([]Job, int64, error) {
	vacancies, err := s.crawl(ctx)
	if err != nil {
		return nil, 0, err
	}
	var dropped atomic.Int64
	jobs := fetchDetails(vacancies, wantapplyDetailWorkers, func(v wantapplyVacancy) (Job, bool) {
		if seen(v.slug) {
			// Already ingested: refresh liveness only, no detail request. Just the identity
			// fields the pipeline's touch needs (ExternalID); content is left untouched.
			return Job{ExternalID: v.slug, URL: v.url, SeenRefresh: true}, true
		}
		job, ok := s.detail(ctx, v)
		if !ok {
			dropped.Add(1)
		}
		return job, ok
	})
	n := dropped.Load()
	if n > 0 {
		log.Printf("wantapply: dropped %d/%d enumerated vacancies this run (detail fetch failed or carried no usable JobPosting)",
			n, len(vacancies))
	}
	// The same guard echojobs carries, for the same reason: only this function knows it walked
	// the sitemap and came back empty-handed, so only it can say so. No candidates at all still
	// succeeds — a quiet source is not a broken one.
	if len(vacancies) > 0 && len(jobs) == 0 {
		return nil, n, fmt.Errorf("wantapply: enumerated %d vacancies and read none of them", len(vacancies))
	}
	return jobs, n, nil
}

// crawl reads the sitemap and returns every vacancy candidate (reserved pages, /company/*, and
// /jobs/* excluded) — the shared enumeration behind Fetch and FetchNew.
func (s wantapply) crawl(ctx context.Context) ([]wantapplyVacancy, error) {
	sitemap, err := getSitemap(ctx, s.sitemap, s.sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("wantapply: sitemap: %w", err)
	}
	var out []wantapplyVacancy
	for _, entry := range sitemap.URLs {
		slug := wantapplyVacancySlugOn(entry.Loc, s.sitemapHost)
		if slug == "" {
			continue
		}
		// The PATH is taken from the sitemap's own loc rather than rebuilt from the slug — the
		// slug is url.Parse-decoded, so reconstruction could diverge from the real page. Only
		// the host is swapped, and only when the enumeration came from the other mirror.
		out = append(out, wantapplyVacancy{slug: slug, url: s.detailURL(entry.Loc)})
	}
	return out, nil
}

// detailURL points a sitemap entry at the host the pages are fetched from, keeping its path
// byte for byte. When the sitemap and the pages come from the same host this returns loc
// unchanged, which is the ordinary case.
func (s wantapply) detailURL(loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		return loc
	}
	base, err := url.Parse(s.detailHost)
	if err != nil {
		return loc
	}
	u.Scheme, u.Host = base.Scheme, base.Host
	return u.String()
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
	// Prefer the page's rendered <div class="Description"> body — it keeps the headings and
	// lists that the flat JobPosting `description` field strips to run-together plain text. Fall
	// back to the (unformatted) JSON-LD description when the container is absent.
	description := wantapplyDescription(root)
	if description == "" {
		description = sanitizeHTML(html.UnescapeString(p.Description))
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
		Description:    description,
		Remote:         remote || isRemote(location),
		WorkMode:       workMode,
		EmploymentType: schemaEmploymentType(employmentType),
		PostedAt:       parseRFC3339(p.DatePosted),
	}, true
}

// wantapplyDescription returns the sanitized rich HTML of the page's <div class="Description">
// body — the block that preserves the headings and lists the flat JobPosting `description` field
// loses. It returns "" when the container is absent, so the caller falls back to the JSON-LD text.
func wantapplyDescription(root *html.Node) string {
	var body string
	walk(root, func(n *html.Node) bool {
		if body != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "Description") {
			body = innerHTML(n)
			return false
		}
		return true
	})
	return sanitizeHTML(body)
}

// wantapplyVacancySlug returns the vacancy slug for a sitemap loc on the default (.cy) host.
func wantapplyVacancySlug(loc string) string {
	return wantapplyVacancySlugOn(loc, wantapplyHostname)
}

// wantapplyVacancySlugOn returns the vacancy slug for a sitemap loc that is a single-segment,
// non-reserved page on host, or "" for the root, a reserved static page, a multi-segment path
// (/company/*, /jobs/*), or an unparseable/foreign URL. The host is a parameter because the
// enumeration may come from the .com mirror while the pages are read from .cy.
func wantapplyVacancySlugOn(loc, host string) string {
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil || u.Hostname() != host {
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
