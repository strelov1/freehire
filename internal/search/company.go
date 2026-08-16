package search

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/db"
)

const (
	// companyIndexUID is the production companies search index — separate from the
	// jobs index, so building it cannot regress jobs search. companyRebuildUID is the
	// throwaway index a full rebuild streams into before the atomic swap (see
	// CompanyRebuild). companyPrimaryKey is `slug` (the natural company key), not the
	// jobs index's numeric `id`; the shared ensure/create helpers are parameterized on
	// the primary key, so both index families share one code path.
	companyIndexUID   = "companies"
	companyRebuildUID = "companies_rebuild"
	companyPrimaryKey = "slug"
)

// CompanyDocument is a company as stored in the Meilisearch companies index,
// separate from the jobs index. It is keyed by `slug` (the natural company key,
// used as this index's primary key). It carries both the fields the list endpoint
// serves — `name`, `tagline`, `industries`, `hq_country`, `job_count` (the
// db.ListCompaniesRow wire shape) — and the denormalized facet arrays/scalars the
// list filters on. Meilisearch returns the whole document on a hit regardless of
// which attributes are searchable/filterable, so the handler projects a hit back
// onto db.ListCompaniesRow and the response contract is unchanged.
//
// pgtype scalars are unwrapped to plain strings (empty when NULL): a Meili document
// is plain JSON, and the handler re-wraps them into pgtype.Text when projecting the
// response so byte-for-byte parity with the Postgres path is preserved.
type CompanyDocument struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Tagline       string   `json:"tagline"`
	Industries    []string `json:"industries"`
	HqCountry     string   `json:"hq_country"`
	JobCount      int32    `json:"job_count"`
	Collections   []string `json:"collections"`
	Regions       []string `json:"regions"`
	Countries     []string `json:"countries"`
	Domains       []string `json:"domains"`
	CompanyTypes  []string `json:"company_types"`
	CompanySizes  []string `json:"company_sizes"`
	RemoteRegions []string `json:"remote_regions"`
	YcBatch       []string `json:"yc_batch"`
	YcStatus      []string `json:"yc_status"`
	YcStage       []string `json:"yc_stage"`
	YcFlags       []string `json:"yc_flags"`
	Maturity      string   `json:"maturity"`
	Subindustry   string   `json:"subindustry"`
	// FeedbackCount/FeedbackRatingAvg mirror companies.feedback_count/
	// feedback_rating_avg (internal/companyfeedback) — display-only here.
	// FeedbackRatingAvg 0 means "no rating" (an average is never really 0:
	// ratings are 1-5), the same absent-is-zero-value convention JobCount uses;
	// neither is a Meili-sortable attribute yet, so "sort by rating" only
	// works on the Postgres path (see ListCompanies in
	// internal/db/queries/companies.sql) until that's added deliberately,
	// alongside the reindex it requires.
	FeedbackCount     int32   `json:"feedback_count"`
	FeedbackRatingAvg float32 `json:"feedback_rating_avg"`
	// UpdatedAt is the row's last write, carried solely so the company sitemap —
	// which pages this index (see sitemap.go) — can emit a <lastmod>. Neither
	// searchable, filterable, nor sortable: a crawler hint, not a query surface.
	UpdatedAt time.Time `json:"updated_at"`
}

// FromCompany maps a stored company row to its index document. The pgtype.Text
// scalars (tagline, hq_country, maturity, subindustry) unwrap to their string
// value — empty when NULL, since pgtype.Text's zero value has an empty String. The
// facet arrays pass through as-is; an empty array simply matches no facet filter.
func FromCompany(c db.Company) CompanyDocument {
	return CompanyDocument{
		Slug:          c.Slug,
		Name:          c.Name,
		Tagline:       c.Tagline.String,
		Industries:    c.Industries,
		HqCountry:     c.HqCountry.String,
		JobCount:      c.JobCount,
		Collections:   c.Collections,
		Regions:       c.Regions,
		Countries:     c.Countries,
		Domains:       c.Domains,
		CompanyTypes:  c.CompanyTypes,
		CompanySizes:  c.CompanySizes,
		RemoteRegions: c.RemoteRegions,
		YcBatch:       c.YcBatch,
		YcStatus:      c.YcStatus,
		YcStage:       c.YcStage,
		YcFlags:       c.YcFlags,
		Maturity:      c.Maturity.String,
		Subindustry:   c.Subindustry.String,
		FeedbackCount: c.FeedbackCount,
		// pgtype.Float4's zero value is {Valid: false}, whose .Float32 is 0 —
		// exactly the "no rating" sentinel this document already wants, so no
		// explicit NULL handling is needed here.
		FeedbackRatingAvg: c.FeedbackRatingAvg.Float32,
		// companies.updated_at is NOT NULL, so .Time is always a real instant.
		UpdatedAt: c.UpdatedAt.Time,
	}
}

// companySettings is the companies index configuration. Search is relevance-first:
// the default ranking rules (words → typo → proximity → attribute → exactness) put
// an exact name match ahead of a mere substring, `sort` keeps an explicit sort
// honoured, and `job_count:desc` is appended as the final tiebreaker so among
// equally-relevant matches the most active company surfaces first — reproducing the
// Postgres path's `job_count DESC` secondary order under a relevance primary.
// Searchable attributes are ordered name → slug → tagline so `attribute` rank
// favours a name hit. The facet arrays/scalars are filterable; `job_count` is
// sortable so the tiebreaker rule can read it. The hiring scope (`job_count > 0`)
// is enforced at index-build time (see reindex-companies), so no query filter is
// needed for it.
func companySettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		SearchableAttributes: []string{"name", "slug", "tagline"},
		FilterableAttributes: []string{
			"collections", "regions", "countries", "domains", "industries",
			"company_types", "company_sizes", "remote_regions",
			"yc_batch", "yc_status", "yc_stage", "yc_flags",
			"maturity", "subindustry",
		},
		SortableAttributes: []string{"job_count"},
		RankingRules:       []string{"words", "sort", "typo", "proximity", "attribute", "exactness", "job_count:desc"},
		Pagination:         &meilisearch.Pagination{MaxTotalHits: maxTotalHits},
	}
}

// companyFacets maps each /companies facet query param to its index attribute, in a
// fixed order so the built filter is deterministic. The array facets filter by
// membership (Meili's `attr = "v"` matches an array containing v) and the scalar
// facets (maturity/subindustry) by equality; both use the same Eq fragment. Note
// the singular params company_type/company_size and plural subindustries map to the
// plural columns and the scalar `subindustry` respectively, mirroring the Postgres
// handler's param names.
var companyFacets = []struct{ param, attr string }{
	{"collections", "collections"},
	{"regions", "regions"},
	{"countries", "countries"},
	{"domains", "domains"},
	// The finer level beneath domains: domains names ~20 coarse verticals from job
	// enrichment, industries names what a company does when that lands in "other".
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

// CompanyFilterFromValues turns the facet params of a /companies request into a
// Meilisearch filter: values within one facet are ORed into a group, facets are
// ANDed across groups. An absent facet adds no constraint; no facet at all yields
// nil (no filter). A NULL scalar (empty maturity/subindustry in the document)
// matches no value, since the fragment is an equality against the requested value.
func CompanyFilterFromValues(v url.Values) any {
	var g [][]string
	for _, f := range companyFacets {
		included := nonEmpty(v[f.param])
		if len(included) == 0 {
			continue
		}
		group := make([]string, len(included))
		for i, val := range included {
			group[i] = Eq(f.attr, val)
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

// SearchCompanies runs a ranked query against the companies index and decodes the
// hits. The index binding is taken lazily from the shared manager, so this adds no
// state to Client and never touches the jobs index fields. Error mapping mirrors
// Search: a cancelled context re-raises the sentinel, and a Meili 400 (a malformed
// filter from client input) maps to ErrBadQuery so the handler can render 400.
func (c *Client) SearchCompanies(ctx context.Context, p CompanySearchParams) (CompanyResult, error) {
	resp, err := c.manager.Index(companyIndexUID).SearchWithContext(ctx, p.Query, &meilisearch.SearchRequest{
		Filter: p.Filter,
		Limit:  int64(p.Limit),
		Offset: int64(p.Offset),
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return CompanyResult{}, fmt.Errorf("search: company query: %w", ctxErr)
		}
		if isBadRequest(err) {
			return CompanyResult{}, fmt.Errorf("search: company query: %w: %v", ErrBadQuery, err)
		}
		return CompanyResult{}, fmt.Errorf("search: company query: %w", err)
	}
	var hits []CompanyDocument
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return CompanyResult{}, fmt.Errorf("search: decode company hits: %w", err)
	}
	return CompanyResult{Hits: hits, Total: resp.EstimatedTotalHits}, nil
}

// CompanyRebuild is a fresh-index build session for a full companies reindex,
// mirroring the jobs Rebuild's stream-then-swap approach but keyed by `slug` and
// scoped to the companies index — it never references the jobs UIDs, so it cannot
// disturb jobs search. Documents stream into the throwaway rebuild index (Push
// enqueues without waiting so Meilisearch auto-batches), then Promote awaits the
// pushes, atomically swaps the rebuild index over the live one, and drops the old.
type CompanyRebuild struct {
	c       *Client
	rebuild meilisearch.IndexManager
	tasks   []int64
}

// NewCompanyRebuild starts a full rebuild of the companies index.
func (c *Client) NewCompanyRebuild() *CompanyRebuild { return &CompanyRebuild{c: c} }

// Prepare ensures the live companies index exists (the swap in Promote needs both)
// and creates a fresh, empty rebuild index with the companies settings, discarding
// any leftover rebuild index from an aborted prior run.
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

// Push enqueues a batch into the rebuild index WITHOUT waiting for it to finish —
// the task uid is kept and awaited in Promote, so Meilisearch auto-batches the
// consecutive document tasks instead of indexing each in isolation.
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

// Promote waits for every enqueued batch, atomically swaps the rebuild index over
// the live companies index, and drops the now-old index. After this the live uid
// serves the freshly built data.
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

// Cleanup drops the rebuild index, tolerating its absence. reindexCompanies defers it
// so a run that aborts before Promote (whose swap-and-drop is the normal teardown)
// does not leave an orphan rebuild index — mirroring the jobs Rebuild. Idempotent.
func (r *CompanyRebuild) Cleanup(ctx context.Context) error {
	return r.c.dropIndex(ctx, companyRebuildUID)
}
