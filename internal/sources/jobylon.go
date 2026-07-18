package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// jobylon adapts Jobylon (jobylon.com), a Nordic ATS powering ~1000 employers' career sites. Its
// per-company embed widget caps at ~32 postings, so instead the adapter enumerates Jobylon's
// single global jobs sitemap (emp.jobylon.com/sitemap.xml → sitemap-jobs.xml, ~8600 URLs) and
// hydrates each posting from its canonical page's schema.org JobPosting ld+json — the
// sitemap-plus-ld+json-detail shape shared with isolvedhire/successfactors/clinch.
//
// It is boardless (one global feed, no per-tenant board) and an aggregator (each posting's
// employer comes from its own hiringOrganization), so it stays in the source facet and inherits
// the reindex aggregator/ATS-duplicate suppression. It is also a HydratingSource: it lists every
// sitemap URL each crawl but fetches a posting's detail only when the catalogue does not already
// have it, so a routine crawl costs only as many detail requests as there are new postings.
type jobylon struct {
	http jobylonHTTP
}

// jobylonHTTP is the client capability the adapter needs: the sitemaps as buffered XML and each
// job page as parsed HTML (for its ld+json).
type jobylonHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewJobylon builds the Jobylon adapter over the given client.
func NewJobylon(c jobylonHTTP) Source { return jobylon{http: c} }

func (jobylon) Provider() string { return "jobylon" }

func (jobylon) boardless() {}

func (jobylon) aggregator() {}

const (
	// jobylonSitemapIndex is the global sitemap index; its "sitemap-jobs" child lists every job.
	jobylonSitemapIndex = "https://emp.jobylon.com/sitemap.xml"
	// jobylonJobsNeedle selects the jobs sub-sitemap from the index, surviving a file rename.
	jobylonJobsNeedle = "sitemap-jobs"
)

// jobylonJobIDPattern captures the numeric posting id from a /jobs/<id>-<slug>/ URL. A company
// page (/companies/<id>-<slug>/) or any non-job loc yields no match and is skipped.
var jobylonJobIDPattern = regexp.MustCompile(`/jobs/(\d+)`)

// jobylonJobID extracts the native posting id from a job URL, or "" when the URL is not a job page.
func jobylonJobID(loc string) string { return firstSubmatch(jobylonJobIDPattern, loc) }

func (s jobylon) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	locs, err := s.jobLocs(ctx)
	if err != nil {
		return nil, err
	}
	// List-only fallback (no seen set): hydrate every posting.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, loc)
	}), nil
}

// FetchNew is the hydrating crawl: it lists every sitemap URL, but fetches a posting's detail only
// for an id the catalogue does not already have. A seen posting is emitted as a liveness refresh
// (identity only, no detail request, no content rewrite); an unseen posting is hydrated from its
// ld+json. Detail fetches run under the shared bounded worker pool.
func (s jobylon) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	locs, err := s.jobLocs(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		// Already ingested: refresh liveness by identity only — no detail request. Re-upserting
		// content-less would wipe the description/facets hydrated when it was new, so the pipeline
		// routes this to a liveness touch instead. locs are pre-filtered to /jobs/<id>, so the id
		// is always present.
		if id := jobylonJobID(loc); seen(id) {
			return Job{ExternalID: id, URL: loc, SeenRefresh: true}, true
		}
		return s.detail(ctx, loc)
	}), nil
}

// jobLocs resolves the jobs sub-sitemap of the global index and returns every /jobs/<id> URL —
// the shared enumeration behind Fetch and FetchNew.
func (s jobylon) jobLocs(ctx context.Context) ([]string, error) {
	jobsSM, err := resolveSubSitemap(ctx, s.http, jobylonSitemapIndex, jobylonJobsNeedle)
	if err != nil {
		return nil, fmt.Errorf("jobylon: sitemap index: %w", err)
	}
	if jobsSM == "" {
		return nil, fmt.Errorf("jobylon: no %q sub-sitemap in index", jobylonJobsNeedle)
	}
	locs, err := sitemapJobLocs(ctx, s.http, jobsSM, jobylonJobID)
	if err != nil {
		return nil, fmt.Errorf("jobylon: jobs sitemap: %w", err)
	}
	return locs, nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false when
// the URL has no id, the fetch fails, the page carries no JobPosting, or the posting's title or
// company resolves empty (an empty dedup key / company would break the public slug), so the caller
// skips just that posting.
func (s jobylon) detail(ctx context.Context, loc string) (Job, bool) {
	id := jobylonJobID(loc)
	if id == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p jobylonPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	title := html.UnescapeString(p.Title)
	company := html.UnescapeString(p.HiringOrganization.Name)
	if title == "" || company == "" {
		return Job{}, false
	}

	location := p.location()
	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       title,
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// Jobylon states no structured workplace-type, so remote is inferred from the location
		// text alone; WorkMode stays empty for the pipeline to resolve.
		Remote:   isRemote(location),
		PostedAt: parseRFC3339(p.DatePosted),
	}, true
}

// jobylonPosting is the schema.org JobPosting decoded from a job page's ld+json. It deliberately
// does NOT model employmentType: Jobylon emits it inconsistently (absent, a string, or an array
// like ["CONTRACTOR"]), and modeling it as a string would fail the whole-posting decode on the
// array form; Go ignores the unmodeled field and the classify/enrich stages derive the value.
type jobylonPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// location joins each jobLocation place as "City, Region, Country" (skipping blank parts), deduped
// and separated by "; ", so a job open in several places lists them all. Most postings carry only
// the locality; the country is left to the geo dictionary rather than parsed out of streetAddress.
func (p jobylonPosting) location() string {
	return distinctJoin(p.JobLocation, "; ", func(pl schemaPlace) string { return pl.Address.Location() })
}
