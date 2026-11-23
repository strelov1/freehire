package search

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	// companyIndexUID is the production companies index, separate from the jobs index.
	// companyRebuildUID is the throwaway index a rebuild streams into before the atomic swap.
	// companyPrimaryKey is `slug`, not the jobs index's numeric `id`.
	companyIndexUID   = "companies"
	companyRebuildUID = "companies_rebuild"
	companyPrimaryKey = "slug"
)

// CompanyDocument is a company as stored in the Meilisearch companies index, keyed by `slug`.
// It carries the fields the list endpoint serves (the db.ListCompaniesRow wire shape) plus the
// denormalized facet arrays the list filters on. Meilisearch returns the whole document on a
// hit, so the handler projects it back onto db.ListCompaniesRow. pgtype scalars are unwrapped
// to plain strings (empty when NULL) and re-wrapped there to keep parity with the Postgres path.
type CompanyDocument struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Tagline    string   `json:"tagline"`
	Industries []string `json:"industries"`
	// IndustriesDerived is the facet recount's translation of the company's own domains into
	// industries; already empty for a company with curated Industries or more than two domains.
	// CompanyFilterFromValues ORs it into the Industries group; queried only, never served.
	IndustriesDerived []string `json:"industries_derived"`
	HqCountry         string   `json:"hq_country"`
	JobCount          int32    `json:"job_count"`
	Collections       []string `json:"collections"`
	Regions           []string `json:"regions"`
	Countries         []string `json:"countries"`
	Domains           []string `json:"domains"`
	CompanyTypes      []string `json:"company_types"`
	CompanySizes      []string `json:"company_sizes"`
	RemoteRegions     []string `json:"remote_regions"`
	YcBatch           []string `json:"yc_batch"`
	YcStatus          []string `json:"yc_status"`
	YcStage           []string `json:"yc_stage"`
	YcFlags           []string `json:"yc_flags"`
	Maturity          string   `json:"maturity"`
	Subindustry       string   `json:"subindustry"`
	// FeedbackCount/FeedbackRatingAvg mirror companies.feedback_count/feedback_rating_avg
	// (internal/engage/companyfeedback), display-only. A rating of 0 means "no rating" (real
	// ratings are 1-5). Neither is Meili-sortable, so "sort by rating" works only on the Postgres
	// path (ListCompanies in internal/platform/db/queries/companies.sql).
	FeedbackCount     int32   `json:"feedback_count"`
	FeedbackRatingAvg float32 `json:"feedback_rating_avg"`
	// UpdatedAt is the row's last write, carried only for the company sitemap's <lastmod>
	// (see sitemap.go). Not searchable, filterable or sortable.
	UpdatedAt time.Time `json:"updated_at"`
}

// FromCompany maps a stored company row to its index document. pgtype.Text scalars unwrap to
// their string (empty when NULL); facet arrays pass through, and an empty one matches no filter.
func FromCompany(c db.Company) CompanyDocument {
	return CompanyDocument{
		Slug:              c.Slug,
		Name:              c.Name,
		Tagline:           c.Tagline.String,
		Industries:        c.Industries,
		IndustriesDerived: c.IndustriesDerived,
		HqCountry:         c.HqCountry.String,
		JobCount:          c.JobCount,
		Collections:       c.Collections,
		Regions:           c.Regions,
		Countries:         c.Countries,
		Domains:           c.Domains,
		CompanyTypes:      c.CompanyTypes,
		CompanySizes:      c.CompanySizes,
		RemoteRegions:     c.RemoteRegions,
		YcBatch:           c.YcBatch,
		YcStatus:          c.YcStatus,
		YcStage:           c.YcStage,
		YcFlags:           c.YcFlags,
		Maturity:          c.Maturity.String,
		Subindustry:       c.Subindustry.String,
		FeedbackCount:     c.FeedbackCount,
		// A NULL pgtype.Float4 yields 0, the document's "no rating" value.
		FeedbackRatingAvg: c.FeedbackRatingAvg.Float32,
		// companies.updated_at is NOT NULL.
		UpdatedAt: c.UpdatedAt.Time,
	}
}

// companySettings is the companies index configuration. Ranking is relevance-first so an exact
// name match beats a substring, with `job_count:desc` as the last tiebreaker to reproduce the
// Postgres path's secondary order. Searchable attributes are ordered name, slug, tagline so a
// name hit ranks highest. The hiring scope (job_count > 0) is enforced at index-build time
// (see reindex-companies), so no query filter is needed.
func companySettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		SearchableAttributes: []string{"name", "slug", "tagline"},
		FilterableAttributes: []string{
			"collections", "regions", "countries", "domains", "industries",
			"industries_derived",
			"company_types", "company_sizes", "remote_regions",
			"yc_batch", "yc_status", "yc_stage", "yc_flags",
			"maturity", "subindustry",
		},
		SortableAttributes: []string{"job_count"},
		RankingRules:       []string{"words", "sort", "typo", "proximity", "attribute", "exactness", "job_count:desc"},
		Pagination:         &meilisearch.Pagination{MaxTotalHits: maxTotalHits},
	}
}

// companyFacets maps each /companies facet query param to its index attribute, in a fixed order
// so the built filter is deterministic. Array facets filter by membership and scalar facets
// (maturity/subindustry) by equality; both use Eq. The singular company_type/company_size and
// plural subindustries params mirror the Postgres handler's names.
var companyFacets = []struct{ param, attr string }{
	{"collections", "collections"},
	{"regions", "regions"},
	{"countries", "countries"},
	{"domains", "domains"},
	// Not a finer level beneath domains: this attribute holds what an importer said, `domains`
	// what the company's own postings imply. CompanyFilterFromValues reads both for this param.
	{"industries", "industries"},
	{"company_type", "company_types"},
	{"company_size", "company_sizes"},
	{"remote_regions", "remote_regions"},
	{"yc_batch", "yc_batch"},
	{"yc_status", "yc_status"},
	{"yc_stage", "yc_stage"},
	{"yc_flags", "yc_flags"},
	{"maturity", "maturity"},
	{"subindustries", "subindustry"},
}

// CompanyFilterFromValues turns the facet params of a /companies request into a Meilisearch
// filter: values within a facet are ORed, facets ANDed. An absent facet adds no constraint and
// no facet at all yields nil. An empty scalar in the document matches no requested value.
//
// The industries facet reads two attributes (below), widened the same way by the Postgres path
// in internal/api/handler; a test compares both so list and meta.total cannot disagree.
func CompanyFilterFromValues(v url.Values) any {
	var g [][]string
	for _, f := range companyFacets {
		// splitFacetValues, not nonEmpty, so a comma-joined value and a repeated key mean the
		// same, as on the job-search path. Taken literally `countries=DE,BR` became a value no
		// company carries and answered ZERO. No facet value contains a comma, so splitting is safe.
		included := splitFacetValues(v[f.param])
		if len(included) == 0 {
			continue
		}
		group := make([]string, 0, len(included))
		for _, val := range included {
			group = append(group, Eq(f.attr, val))
		}
		// An industry reaches a company from two ordered sources: the curated `industries` and
		// `industries_derived` (already empty for a curated company or one with more than two
		// domains). Both join the SAME group so they OR, with precedence baked in at
		// materialization time.
		if f.param == "industries" {
			for _, val := range included {
				group = append(group, Eq("industries_derived", val))
			}
		}
		g = append(g, group)
	}
	return Filter(g...)
}

// CompanySearchParams is a company search request against the companies index.
// Filter is the value built by CompanyFilterFromValues (nil for none).
type CompanySearchParams struct {
	Query  string
	Filter any
	Limit  int
	Offset int
}

// CompanyResult holds the matched company documents and Meilisearch's estimated
// total (the filtered count that backs the list's meta.total on the Meili path).
type CompanyResult struct {
	Hits  []CompanyDocument
	Total int64
}

// SearchCompanies runs a ranked query against the companies index and decodes the hits. The
// index is bound lazily from the shared manager, leaving the jobs index fields untouched. Errors
// map as in Search: a cancelled context re-raises the sentinel and a Meili 400 (malformed client
// filter) becomes ErrBadQuery.
func (c *Client) SearchCompanies(ctx context.Context, p CompanySearchParams) (CompanyResult, error) {
	resp, err := c.manager.Index(companyIndexUID).SearchWithContext(ctx, p.Query, &meilisearch.SearchRequest{
		Filter: p.Filter,
		Limit:  int64(p.Limit),
		Offset: int64(p.Offset),
	})
	if err != nil {
		return CompanyResult{}, queryErr(ctx, "search: company query", err)
	}
	var hits []CompanyDocument
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return CompanyResult{}, fmt.Errorf("search: decode company hits: %w", err)
	}
	return CompanyResult{Hits: hits, Total: resp.EstimatedTotalHits}, nil
}

// CompanyRebuild is a fresh-index build session for a full companies reindex, mirroring the jobs
// Rebuild's stream-then-swap but scoped to the companies index so it cannot disturb jobs search.
// Push enqueues without waiting so Meilisearch auto-batches; Promote awaits the pushes, swaps the
// rebuild index over the live one and drops the old.
type CompanyRebuild struct {
	c       *Client
	rebuild meilisearch.IndexManager
	tasks   []int64
}

// NewCompanyRebuild starts a full rebuild of the companies index.
func (c *Client) NewCompanyRebuild() *CompanyRebuild { return &CompanyRebuild{c: c} }

// Prepare ensures the live companies index exists (Promote's swap needs both) and creates a fresh
// rebuild index, discarding any leftover from an aborted run.
func (r *CompanyRebuild) Prepare(ctx context.Context) error {
	if err := r.c.ensure(ctx, r.c.manager.Index(companyIndexUID), companyIndexUID, companyPrimaryKey, companySettings()); err != nil {
		return err
	}
	if err := r.c.dropIndex(ctx, companyRebuildUID); err != nil {
		return err
	}
	r.rebuild = r.c.manager.Index(companyRebuildUID)
	return r.c.ensure(ctx, r.rebuild, companyRebuildUID, companyPrimaryKey, companySettings())
}

// Push enqueues a batch into the rebuild index without waiting; the task uid is awaited in
// Promote so Meilisearch can auto-batch consecutive tasks.
func (r *CompanyRebuild) Push(ctx context.Context, docs []CompanyDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := companyPrimaryKey
	task, err := r.rebuild.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: company rebuild push: %w", err)
	}
	r.tasks = append(r.tasks, task.TaskUID)
	return nil
}

// Promote waits for every enqueued batch, swaps the rebuild index over the live companies index
// and drops the old one.
func (r *CompanyRebuild) Promote(ctx context.Context) error {
	for _, uid := range r.tasks {
		if err := r.c.awaitTask(ctx, r.rebuild, uid); err != nil {
			return err
		}
	}
	if err := r.c.swapIndexes(ctx, companyIndexUID, companyRebuildUID); err != nil {
		return err
	}
	return r.c.dropIndex(ctx, companyRebuildUID)
}

// Cleanup drops the rebuild index, tolerating its absence. reindexCompanies defers it so an
// aborted run leaves no orphan. Idempotent.
func (r *CompanyRebuild) Cleanup(ctx context.Context) error {
	return r.c.dropIndex(ctx, companyRebuildUID)
}
