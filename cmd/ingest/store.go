package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobdedup"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/vocab"
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

// clustersByRole reports whether a persisted write is worth asking the role-cluster
// question about — the gate in front of jobdedup.CanonicalForRole.
//
// It is a cost gate, not a correctness one: the lookup is a range scan on
// jobs_open_role_cluster_idx, but a full crawl replays millions of rows per pass against
// a database that is already the fleet's I/O bottleneck, so which writes pay for it is a
// real choice. The batch recompute remains the reconciler for whatever this skips.
//
// Only a genuinely new posting is asked about. A per-city fan-out arrives as inserts, so
// this catches it at the moment it happens, and a full pass then pays one lookup per new
// vacancy — thousands — instead of one per row re-seen, which is nearly the whole
// catalogue. What it deliberately leaves to the batch recompute is the posting that turns
// into a duplicate later, when an edit makes its title and description match a sibling's:
// rarer than the fan-out, and no worse off than before this existed.
//
// A row that already carries a marker knows its own answer and is not re-asked.
func clustersByRole(row db.UpsertJobRow) bool {
	return row.Inserted.Bool && !row.Job.DuplicateOf.Valid
}

func (s *dbStore) Save(ctx context.Context, j job.Job) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after Commit is a no-op; suppress its error (matches the other stores).
	defer func() { _ = tx.Rollback(ctx) }()

	// The aggregate's read projection carries every persistable field; the write path
	// never touches enrichment (SetJobEnrichment owns those columns), so it is not mapped.
	// The mapping also stamps content_hash — which the upsert reports back as
	// inserted/changed, driving the incremental index push below — and role_fingerprint.
	params := j.Fields().UpsertParams()

	qtx := s.q.WithTx(tx)
	saved, err := qtx.UpsertJob(ctx, params)
	if err != nil {
		return fmt.Errorf("upsert job: %w", err)
	}

	// A company that posts one role once per city sends it as many board identities, all
	// sharing a role_fingerprint. Ask — after the upsert, because the answer depends on
	// the written row's id — whether an older canonical copy already exists, and point
	// this row at it. Without this the copies stay separate searchable jobs until the
	// next batch recompute hours later, and every one of them reaches search, the
	// subscription digests, and the company's job count as a vacancy of its own.
	deduped := false
	if clustersByRole(saved) {
		if canon, ok := jobdedup.CanonicalForRole(ctx, qtx, params, saved.Job.ID); ok {
			if _, err := qtx.MarkJobDuplicateOf(ctx, db.MarkJobDuplicateOfParams{
				ID:          saved.Job.ID,
				DuplicateOf: pgtype.Int8{Int64: canon.ID, Valid: true},
			}); err != nil {
				return fmt.Errorf("mark job %d duplicate of %d: %w", saved.Job.ID, canon.ID, err)
			}
			deduped = true
		}
	}

	// A duplicate never reaches search, so enriching it pays an LLM for an invisible row.
	if !deduped {
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion:     s.targetVersion,
			JobID:             saved.Job.ID,
			ExcludeCategories: vocab.NonTechCategories,
		}); err != nil {
			return fmt.Errorf("enqueue enrichment: %w", err)
		}
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
	// A non-canonical repost is not searchable, so it never reaches the live index —
	// whether it was marked by a prior recompute or by the write above. What still falls
	// to the reindex is a row that only became a repost after it was written.
	if s.indexer != nil && needsIndex(saved) && !deduped && !saved.Job.DuplicateOf.Valid {
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

// ExistingExternalIDs returns the set of external_ids already stored for the board about to be
// crawled — the pipeline's seen-set for a hydrating source, so per-posting detail is fetched only
// for postings the catalogue lacks. Implements pipeline.seenLookup.
//
// The lookup runs once per board, so a board-bearing provider reads only its own namespace: on
// workday the provider-wide query returns 1.27M ids in ~168s against ~1.8s for a board's 25k. A
// boardless adapter has no namespace to scope by and reads the provider's whole set, which is what
// its single "board" means anyway.
//
// Each id maps to its row's tech evidence (is_tech IS TRUE), which the refresh path judges the
// catalogue filter on — a re-listed posting carries no description to derive it from.
func (s *dbStore) ExistingExternalIDs(ctx context.Context, source, board string) (map[string]bool, error) {
	set := map[string]bool{}
	if board == "" {
		rows, err := s.q.ExistingExternalIDs(ctx, source)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
		}
		return set, nil
	}
	// The pattern is escaped by sources.BoardIDPattern: a board name may contain LIKE syntax,
	// and a third of the workday board names carry an underscore.
	rows, err := s.q.ExistingExternalIDsByBoard(ctx, db.ExistingExternalIDsByBoardParams{
		Source:  source,
		Pattern: sources.BoardIDPattern(board),
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
	}
	return set, nil
}

// Touch refreshes an already-ingested posting's liveness (last_seen_at, reopen if closed) by
// its (source, external_id) identity, without rewriting content — the path a HydratingSource
// (justjoin) uses for an offer it re-listed but did not re-fetch, so its hydrated description
// and facets are preserved. Implements pipeline.toucher.
func (s *dbStore) Touch(ctx context.Context, source, externalID string) error {
	companySlug, err := s.q.TouchJob(ctx, db.TouchJobParams{
		Source:     source,
		ExternalID: externalID,
	})
	if err != nil {
		return fmt.Errorf("touch job %s/%s: %w", source, externalID, err)
	}
	// Record the company as crawled, exactly as Save does, so the post-run stale sweep keeps
	// this company in scope. Otherwise a company whose offers were all touched (none newly
	// saved) would fall out of the crawled-set and its removed offers would never close.
	if s.crawled != nil {
		s.crawled.record(source, companySlug)
	}
	return nil
}
