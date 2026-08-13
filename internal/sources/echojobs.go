package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/skilltag"
)

// echojobs adapts echojobs.io, a large multi-employer tech-jobs aggregator whose postings
// resolve to the employer's own ATS URL (Workday, Greenhouse, and others show up in the feed)
// rather than to an echojobs.io-hosted page. Boardless: one global feed, sorted newest-first, no
// per-tenant board.
//
// echojobs.io retired its public JSON API (the list+detail endpoints this adapter used to call)
// around 2026-08-13 — both 404 now, verified from prod's own egress IP as well as externally.
// Two other public, SEO-maintained surfaces replace it: a paginated XML sitemap
// (https://echojobs.io/sitemap.xml -> sitemap-jobs/N.xml shards, confirmed live to be sorted
// newest-first by <lastmod>) for discovery, and a schema.org JobPosting ld+json block embedded
// in each posting's server-rendered detail page (/job/<slug>) for every structured field a
// posting has (title, company, description, remote signal, location, skills, posting date).
// Neither needs JavaScript execution — a plain GET reaches both.
//
// The sitemap carries nothing beyond a posting's URL and last-modified time — unlike the old
// list JSON, it cannot supply even a title without a detail fetch — so the "list vs detail"
// split this adapter used to have collapsed into one tier: every posting FetchNew or Fetch
// reports has its full JobPosting fetched (FetchNew still skips that fetch for a posting the
// catalogue already has — see its doc). A detail fetch that fails, or a page carrying no usable
// JobPosting, simply drops that posting: there is no list-only fallback left to fall back to.
type echojobs struct {
	http echojobsHTTP
}

// echojobsHTTP is the transport echojobs needs: XML sitemap discovery plus HTML detail pages.
type echojobsHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	echojobsBaseURL    = "https://echojobs.io"
	echojobsSitemapURL = echojobsBaseURL + "/sitemap.xml"
	// echojobsFreshnessWindow bounds how far back the sitemap walk reads: the site's whole job
	// sitemap runs into the hundreds of thousands of URLs and cannot be crawled to its end every
	// run, so the walk stops once a shard's postings age past this window — the same bound the
	// old list-page walk used, over the new data source.
	echojobsFreshnessWindow = 14 * 24 * time.Hour
	// echojobsSweepGrace widens the post-run unseen sweep to match echojobsFreshnessWindow — see
	// the sweepGrace type doc in source.go.
	echojobsSweepGrace = echojobsFreshnessWindow
)

// NewEchoJobs builds the echojobs adapter over the given HTTP client.
func NewEchoJobs(c echojobsHTTP) Source { return echojobs{http: c} }

func (echojobs) Provider() string { return "echojobs" }

// echojobs is a marketplace with one global feed, so its config entries carry no board.
func (echojobs) boardless() {}

// echojobs aggregates postings from many companies, so it stays in the source facet.
func (echojobs) aggregator() {}

func (echojobs) sweepGrace() time.Duration { return echojobsSweepGrace }

// Fetch fully hydrates every posting the sitemap walk finds within the freshness window. Unlike
// FetchNew it has no seen-set to skip already-ingested postings — the sitemap carries no field a
// cheaper "list-only" tier could serve any more, so there is nothing partial left to return.
// Kept for callers that do not drive hydration; the live pipeline always prefers FetchNew (see
// internal/pipeline.fetchBoard).
func (s echojobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the sitemap for postings inside the freshness window and fetches full detail
// (the JobPosting ld+json on each posting's own page) only for a posting the catalogue does not
// already have — seen reports whether a posting's slug is already ingested. A seen posting is
// refreshed by identity only (SeenRefresh, no detail fetch: a content-less re-upsert would wipe
// the description hydrated when it was new); an unseen posting is hydrated with its detail; a
// posting whose detail fetch fails, or whose page carries no usable JobPosting, is dropped and
// logged — there is no list-only Job left to fall back to (contrast the old list+detail split,
// where a failed detail still yielded a list-only posting). Detail fetches run under the shared
// bounded worker pool.
func (s echojobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p echojobsPosting) (Job, bool) {
		if seen(p.Slug) {
			return Job{ExternalID: p.Slug, URL: echojobsJobURL(p.Slug), SeenRefresh: true}, true
		}
		job, err := s.detail(ctx, p.Slug)
		if err != nil {
			log.Printf("echojobs: detail %q: %v; dropping (sitemap carries no list-only fallback fields)", p.Slug, err)
			return Job{}, false
		}
		return job, true
	}), nil
}

// echojobsPosting is one posting discovered from the sitemap: its slug and last-modified time.
// The sitemap carries no other field.
type echojobsPosting struct {
	Slug    string
	LastMod *time.Time
}

// crawl discovers postings by walking the sitemap index's job shards in order, stopping once a
// shard's postings age past echojobsFreshnessWindow — the shards are confirmed sorted
// newest-first, so this is the "stop once stale" walk the old page-based crawl used, over a
// different data source. Per the house pagination rule, a failure fetching the sitemap index
// itself or the FIRST shard is a board-level error; a later shard failing ends the walk with
// what was already gathered.
func (s echojobs) crawl(ctx context.Context) ([]echojobsPosting, error) {
	shards, err := s.sitemapShards(ctx)
	if err != nil {
		return nil, fmt.Errorf("echojobs: sitemap index: %w", err)
	}
	cutoff := time.Now().Add(-echojobsFreshnessWindow)
	var postings []echojobsPosting
	for i, shardURL := range shards {
		entries, err := s.sitemapShard(ctx, shardURL)
		if err != nil {
			if i == 0 {
				return nil, fmt.Errorf("echojobs: shard %s: %w", shardURL, err)
			}
			log.Printf("echojobs: shard %s failed, stopping with %d postings gathered: %v", shardURL, len(postings), err)
			return postings, nil
		}
		stale := false
		for _, p := range entries {
			if p.LastMod != nil && p.LastMod.Before(cutoff) {
				stale = true
				continue
			}
			postings = append(postings, p)
		}
		if stale {
			return postings, nil
		}
	}
	return postings, nil
}

// echojobsSitemapIndex is the top-level https://echojobs.io/sitemap.xml: an index of
// sub-sitemaps, only some of which (sitemap-jobs/N.xml) carry postings — the rest (nav,
// categories, blog, companies) are the site's other pages.
type echojobsSitemapIndex struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// echojobsURLSet is one job-sitemap shard (sitemap-jobs/N.xml): a flat list of posting URLs.
type echojobsURLSet struct {
	URLs []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"url"`
}

// echojobsJobShardRE matches a job-postings sitemap shard's URL, as opposed to the index's
// other shards (sitemap-nav.xml, sitemap-categories.xml, sitemap-blog.xml,
// sitemap-companies.xml) — verified live.
var echojobsJobShardRE = regexp.MustCompile(`/sitemap-jobs/\d+\.xml$`)

// sitemapShards fetches the sitemap index and returns the job shard URLs, in the order the
// index lists them (verified live: shard 1 is newest).
func (s echojobs) sitemapShards(ctx context.Context) ([]string, error) {
	var idx echojobsSitemapIndex
	if err := s.http.GetXML(ctx, echojobsSitemapURL, &idx); err != nil {
		return nil, err
	}
	var shards []string
	for _, sm := range idx.Sitemaps {
		if echojobsJobShardRE.MatchString(sm.Loc) {
			shards = append(shards, sm.Loc)
		}
	}
	return shards, nil
}

// sitemapShard fetches one job shard and returns its postings, decoding a slug from each URL
// and dropping any entry whose URL doesn't match the expected /job/<slug> shape (defensive: the
// sitemap format is outside our control).
func (s echojobs) sitemapShard(ctx context.Context, shardURL string) ([]echojobsPosting, error) {
	var set echojobsURLSet
	if err := s.http.GetXML(ctx, shardURL, &set); err != nil {
		return nil, err
	}
	postings := make([]echojobsPosting, 0, len(set.URLs))
	for _, u := range set.URLs {
		slug := echojobsSlug(u.Loc)
		if slug == "" {
			continue
		}
		postings = append(postings, echojobsPosting{Slug: slug, LastMod: parseRFC3339(u.LastMod)})
	}
	return postings, nil
}

// echojobsSlugRE extracts a posting's slug from its sitemap or job-page URL.
var echojobsSlugRE = regexp.MustCompile(`/job/([^/?#]+)/?$`)

func echojobsSlug(loc string) string {
	m := echojobsSlugRE.FindStringSubmatch(loc)
	if m == nil {
		return ""
	}
	return m[1]
}

func echojobsJobURL(slug string) string {
	return echojobsBaseURL + "/job/" + slug
}

// echojobsJobPosting is the schema.org JobPosting decoded from a /job/<slug> page's ld+json
// block. jobLocation is a single Place (absent for a fully-remote posting) and
// applicantLocationRequirements a single Country — verified live on both an on-site posting
// (JLL) and a remote one (Doowii). skills is a comma-separated string, not an array, unlike the
// old list JSON's required_skills.
type echojobsJobPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address schemaAddress `json:"address"`
	} `json:"jobLocation"`
	ApplicantLocationRequirements struct {
		Name string `json:"name"`
	} `json:"applicantLocationRequirements"`
	Skills string `json:"skills"`
}

// location prefers the explicit jobLocation address and falls back to
// applicantLocationRequirements — the only signal a fully-remote posting exposes, its
// jobLocation being absent.
func (p echojobsJobPosting) location() string {
	if loc := p.JobLocation.Address.Location(); loc != "" {
		return loc
	}
	return p.ApplicantLocationRequirements.Name
}

// errEchojobsNoJobPosting means a detail page fetched fine but carried no usable JobPosting
// ld+json block (absent, or present with an empty title) — distinct from a fetch error so a
// caller's log line can tell "echojobs is unreachable/blocking us" from "this one page is
// malformed".
var errEchojobsNoJobPosting = fmt.Errorf("no usable JobPosting ld+json block")

// detail fetches one posting's page and maps its JobPosting ld+json to a Job, returning an error
// when the fetch fails or the page carries no usable JobPosting — so the caller drops just that
// posting, with enough detail to distinguish the two failure modes in its log.
func (s echojobs) detail(ctx context.Context, slug string) (Job, error) {
	root, err := s.http.GetHTML(ctx, echojobsJobURL(slug))
	if err != nil {
		return Job{}, fmt.Errorf("fetch: %w", err)
	}
	var p echojobsJobPosting
	if !ldJobPosting(root, &p) || p.Title == "" {
		return Job{}, errEchojobsNoJobPosting
	}

	// jobLocationType is the only structured work-arrangement signal: TELECOMMUTE means
	// remote. Absent → leave WorkMode empty rather than guess hybrid/onsite.
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")

	return Job{
		ExternalID:  slug,
		URL:         echojobsJobURL(slug),
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Location:    p.location(),
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		Skills:      skilltag.Canonicalize(echojobsSplitSkills(p.Skills)),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, nil
}

// echojobsSplitSkills splits the JobPosting skills field's comma-separated string into its
// individual terms, trimming whitespace and dropping empties. Returns nil for an empty input so
// skilltag.Canonicalize(nil) stays a no-op rather than allocating.
func echojobsSplitSkills(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// EchoJobsDescription fetches the sanitized description for a stored echojobs job by its slug
// (ExternalID, minus the boardless namespace prefix — see cmd/backfill-echojobs) — for a
// backfill worker filling the description of rows whose original ingest failed to hydrate one.
// Returns ok=false when the page fetch fails, carries no JobPosting, or the posting has no body.
func EchoJobsDescription(ctx context.Context, c HTMLGetter, slug string) (string, bool) {
	root, err := c.GetHTML(ctx, echojobsJobURL(slug))
	if err != nil {
		return "", false
	}
	var p struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(root, &p) || p.Description == "" {
		return "", false
	}
	return sanitizeHTML(html.UnescapeString(p.Description)), true
}
