package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// jobIndexer buffers a persisted job's document for the live search index. It is
// nil when the worker has no search engine configured (indexing is then skipped).
type jobIndexer interface {
	Add(ctx context.Context, doc search.JobDocument)
}

// dbStore adapts the generated queries + connection pool to pipeline.Store. Save runs
// the job upsert and the gated enrichment enqueue in one transaction, so a newly
// ingested job is queued for enrichment atomically with its write. When an indexer
// is configured, a write that inserted or changed indexed content is also fed to
// the live search index (best-effort, after the commit).
type dbStore struct {
	pool          *pgxpool.Pool
	q             *db.Queries
	targetVersion int32
	indexer       jobIndexer
	crawled       *crawledSet
}

func newDBStore(pool *pgxpool.Pool, targetVersion int, indexer jobIndexer, crawled *crawledSet) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool), targetVersion: int32(targetVersion), indexer: indexer, crawled: crawled}
}

// needsIndex reports whether a persisted write changed what search would show: a
// new row, or one whose indexed content (content_hash) changed. A re-ingest that
// only refreshed bookkeeping (last_seen_at) reports neither and is skipped.
// Changed is already true on insert (a NULL prior hash is DISTINCT FROM any value);
// Inserted is OR-ed in to keep the "new or changed" intent explicit.
func needsIndex(row db.UpsertJobRow) bool {
	return row.Changed || row.Inserted.Bool
}

func (s *dbStore) Save(ctx context.Context, j job.Job) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// The aggregate's read projection carries every persistable field; the write path
	// never touches enrichment (SetJobEnrichment owns those columns), so it is not mapped.
	f := j.Fields()
	params := db.UpsertJobParams{
		Source:      f.Source,
		ExternalID:  f.ExternalID,
		URL:         f.URL,
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
		PublicSlug:  f.PublicSlug,
		Location:    f.Location,
		Remote:      f.Remote,
		Description: f.Description,
		PostedAt:    toTimestamptz(f.PostedAt),
		Countries:   f.Countries,
		Regions:     f.Regions,
		Cities:      f.Cities,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: toInt4(f.ExperienceYearsMin),
	}
	// Fingerprint the indexed fields so the upsert can report whether this write
	// changed searchable content (drives incremental indexing below).
	params.ContentHash = pgtype.Text{String: jobhash.Of(params), Valid: true}
	// role_fingerprint is the repost IDENTITY (excludes posted_at/url/slug), so a
	// reposted role clusters for the job-reality signal — distinct from content_hash.
	params.RoleFingerprint = pgtype.Text{String: jobhash.RoleFingerprint(params), Valid: true}

	qtx := s.q.WithTx(tx)
	saved, err := qtx.UpsertJob(ctx, params)
	if err != nil {
		return fmt.Errorf("upsert job: %w", err)
	}

	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion:     s.targetVersion,
		JobID:             saved.Job.ID,
		ExcludeCategories: enrich.NonTechCategories,
	}); err != nil {
		return fmt.Errorf("enqueue enrichment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Record this (provider, company) as crawled so the post-run stale sweep only
	// closes jobs of companies this run actually wrote — never a provider's whole
	// catalogue when a run crawled only some of its boards. Uses the persisted row so
	// aggregator sources (one board, per-job companies) scope by their real companies.
	if s.crawled != nil {
		s.crawled.record(saved.Job.Source, saved.Job.CompanySlug)
	}

	// Best-effort incremental indexing of the now-committed row: only when the
	// write inserted or changed indexed content, and only if an indexer is wired.
	// A document-build failure is logged, never propagated — the batch reindex is
	// the reconciler, and indexing must not fail ingest. The doc is built from the
	// persisted row, so a re-ingested already-enriched job keeps its enrichment
	// facets. The signal only covers fields UpsertJob writes: changes made by other
	// paths (enrichment via SetJobEnrichment, collections via
	// PropagateCollectionsToJobs) reconcile on the next batch reindex, not here.
	if s.indexer != nil && needsIndex(saved) {
		// The job-reality signal needs this role's cluster counts; a lookup failure
		// degrades to a unique role (counts 1) rather than failing the index push.
		repost, mass := int64(1), int64(1)
		if c, err := s.q.RoleClusterCount(ctx, db.RoleClusterCountParams{
			CompanySlug:     saved.Job.CompanySlug,
			RoleFingerprint: saved.Job.RoleFingerprint,
		}); err != nil {
			log.Printf("ingest: role-cluster count for job %d: %v", saved.Job.ID, err)
		} else {
			repost, mass = c.RepostCount, c.MassCount
		}
		doc, err := search.FromJob(saved.Job)
		if err != nil {
			log.Printf("ingest: build index doc for job %d: %v", saved.Job.ID, err)
		} else {
			reality := jobview.ClassifyReality(saved.Job, time.Now(), int(repost), int(mass))
			doc.Reality = &reality
			s.indexer.Add(ctx, doc)
		}
	}

	return nil
}

// Close soft-closes a posting by its (source, external_id) identity — the stream-driven
// close path a self-closing source (jobtech) uses for ads its incremental feed reports
// removed. Idempotent (the query no-ops on an already-closed or absent row), so a re-sent
// removal in the trailing window costs nothing.
func (s *dbStore) Close(ctx context.Context, source, externalID string) error {
	if _, err := s.q.CloseJobBySourceExternalID(ctx, db.CloseJobBySourceExternalIDParams{
		Source:     source,
		ExternalID: externalID,
	}); err != nil {
		return fmt.Errorf("close job %s/%s: %w", source, externalID, err)
	}
	return nil
}

// toTimestamptz maps an optional posted_at to the pgtype the generated params expect;
// a nil time becomes SQL NULL.
func toTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// toInt4 maps an optional int (e.g. experience_years_min) to the pgtype the generated
// params expect; a nil pointer becomes SQL NULL.
func toInt4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}
