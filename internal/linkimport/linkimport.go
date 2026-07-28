// Package linkimport imports one vacancy into the catalog from its page URL. Some
// postings live only as a single detail page that no board feed enumerates (a Teamtailor
// custom-domain site with an empty listing, a Breezy private-link posting), so a board
// entry cannot reach them — but a link to the page can.
//
// It is the on-demand counterpart to the board crawlers, and the shared engine behind
// both surfaces that offer it: cmd/resolve-url (an operator with a list of URLs) and the
// browser extension's "add this page" (a signed-in user standing on a vacancy we lack).
// Keeping it in one package keeps one definition of the import: the registry order, the
// canonical write path, the enrichment enqueue, and the search push.
package linkimport

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/vocab"
)

// Result identifies the posting an import wrote, under the destination's own catalog
// identity — so a later crawl of the same posting from its board dedups onto this row.
type Result struct {
	Source     string
	ExternalID string
	PublicSlug string
}

// Importer resolves job URLs and writes what it parses. idx may be nil (no search engine
// configured), which skips the index push.
type Importer struct {
	pool *pgxpool.Pool
	q    *db.Queries
	idx  *search.Client
	reg  []linksource.Source
}

// New builds an Importer over the given database and HTTP transport. ingest is the
// provider-keyed ingest registry, which board coverage reuses to read a vacancy on any ATS
// that has a crawler but no single-page adapter; a nil registry disables that step and is
// what tests with a canned page pass. The resolver order itself lives in
// linksource.ImportRegistry, so it is stated once.
func New(pool *pgxpool.Pool, q *db.Queries, idx *search.Client, c linksource.Client, ingest map[string]sources.Source) *Importer {
	return &Importer{
		pool: pool,
		q:    q,
		idx:  idx,
		reg:  linksource.ImportRegistry(c, ingest),
	}
}

// Import resolves raw through the registry and writes the vacancy it parses. ok=false
// means the page is not a single vacancy we can read (no adapter matched, or the page
// carries no posting) — the caller decides what to do with it, and nothing is written. A
// non-nil error is a transient fetch/parse failure or a write failure.
func (im *Importer) Import(ctx context.Context, raw string) (Result, bool, error) {
	resolved, err := linksource.ResolveLinks(ctx, im.reg, []string{raw})
	if err != nil {
		return Result{}, false, err
	}
	if len(resolved) == 0 {
		return Result{}, false, nil
	}
	return im.write(ctx, resolved[0])
}

// write persists one resolved job through the Job aggregate factory and the canonical
// UpsertJob, enqueuing it for enrichment in the same transaction — the same write path as
// ingest and tg-extract, so facets, slugs and the enrichment outbox stay consistent.
func (im *Importer) write(ctx context.Context, r linksource.Resolved) (Result, bool, error) {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      r.Source,
			ExternalID:  r.Job.ExternalID,
			Title:       r.Job.Title,
			Company:     r.Job.Company,
			Location:    r.Job.Location,
			Description: r.Job.Description,
			WorkMode:    r.Job.WorkMode,
		},
		URL:    r.Job.URL,
		Remote: r.Job.Remote,
	})
	if err != nil {
		return Result{}, false, err
	}
	params := j.Fields().UpsertParams()
	if r.Job.PostedAt != nil {
		params.PostedAt = pgtype.Timestamptz{Time: *r.Job.PostedAt, Valid: true}
	}
	params.RoleFingerprint = pgtype.Text{String: jobhash.RoleFingerprint(params), Valid: true}

	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return Result{}, false, err
	}
	defer tx.Rollback(ctx)
	qtx := im.q.WithTx(tx)
	res, err := qtx.UpsertJob(ctx, params)
	if err != nil {
		return Result{}, false, err
	}
	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion:     int32(enrich.Version),
		JobID:             res.Job.ID,
		ExcludeCategories: vocab.NonTechCategories,
	}); err != nil {
		return Result{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, false, err
	}
	im.index(ctx, res)

	return Result{
		Source:     res.Job.Source,
		ExternalID: res.Job.ExternalID,
		PublicSlug: res.Job.PublicSlug,
	}, true, nil
}

// index pushes a just-written open job to the live search index, best-effort — same doc
// build as cmd/ingest, so the company page and /jobs search show it immediately instead of
// waiting for the next full reindex. Unlike ingest it does NOT gate on Inserted/Changed: an
// import is an explicit act on one URL, so a re-import must (re)index an already-present
// posting rather than silently no-op on an unchanged hash. The Meili upsert is idempotent.
// A build or push failure is logged and swallowed — the job is already persisted and the
// batch reindex reconciles. A non-canonical repost or a closed job is never made
// searchable, matching ingest.
func (im *Importer) index(ctx context.Context, saved db.UpsertJobRow) {
	if im.idx == nil || saved.Job.DuplicateOf.Valid || saved.Job.ClosedAt.Valid {
		return
	}
	// The job-reality signal needs this role's cluster counts; a lookup failure degrades
	// to a unique role (counts 1) rather than skipping the push.
	repost, mass := int64(1), int64(1)
	if c, err := im.q.RoleClusterCount(ctx, db.RoleClusterCountParams{
		CompanySlug:     saved.Job.CompanySlug,
		RoleFingerprint: saved.Job.RoleFingerprint,
	}); err == nil {
		repost, mass = c.RepostCount, c.MassCount
	}
	doc, err := search.FromJob(saved.Job)
	if err != nil {
		log.Printf("linkimport: build index doc for job %d: %v", saved.Job.ID, err)
		return
	}
	reality := jobview.ClassifyReality(saved.Job, time.Now(), int(repost), int(mass))
	doc.Reality = &reality
	if err := im.idx.SubmitJobs(ctx, []search.JobDocument{doc}); err != nil {
		log.Printf("linkimport: index job %d: %v", saved.Job.ID, err)
	}
}
