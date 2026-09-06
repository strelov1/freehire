package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries + a pool to the Repository. targetVersion is the
// enrichment schema version a newly created job is enqueued at (enrich.Version), so a
// manual job flows into enrichment like every other source.
type QueriesRepository struct {
	q             *db.Queries
	pool          *pgxpool.Pool
	targetVersion int32
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool, targetVersion int32) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool, targetVersion: targetVersion}
}

// Create runs the manual-job upsert, the enrichment enqueue and the search enqueue in one
// transaction, so a newly created job is queued for both atomically with its write (the
// same transactional-outbox property as the ingest write path).
//
// The search enqueue is unconditional. cmd/ingest gates its own on "inserted or changed"
// because it replays millions of rows a pass; a moderator write is one deliberate row, and
// ClaimSearchOutboxBatch already skips an entry whose job has since closed or become a
// non-canonical repost. cmd/search-drain applies the CategoryUnresolved/DescriptionMissing
// rules on the way out, so nothing is gated here that is not gated there.
func (r *QueriesRepository) Create(ctx context.Context, f job.Fields, actorID int64) (job.Job, job.Extras, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	// The Fields→params mapping (including the manual-salary seeding and the actor
	// stamp) lives on the aggregate (job.Fields.UpsertManualParams), shared with
	// every other write path. The mint query also folds the manual salary into the
	// enrichment payload so it displays at once.
	row, err := qtx.UpsertManualJob(ctx, f.UpsertManualParams(actorID))
	if err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("upsert manual job: %w", err)
	}
	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion: r.targetVersion,
		JobID:         row.ID,
	}); err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("enqueue enrichment: %w", err)
	}
	if err := qtx.EnqueueSearchOutbox(ctx, row.ID); err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("enqueue search outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	return job.FromRow(row)
}

// BySlug loads a job by its public slug, returning ErrJobNotFound when no job matches,
// the matched job was not moderator-authored (created_by IS NULL) — so the edit path can
// never touch an automated-source (ATS/telegram) vacancy, whatever its declared source —
// or the matched job is private.
//
// created_by does not say "a moderator wrote this" on its own: InsertPrivateJob stamps it
// with the submitter too, so before the is_private check a moderator who knew the slug of
// a jd-tailor-intake private JD could read it here and then rewrite it through Update.
// GetJobBySlug carries no is_private predicate by design — internal/ingest/jdresolve says
// so, and leaves ownership to each caller. This is this caller's half of that bargain.
func (r *QueriesRepository) BySlug(ctx context.Context, slug string) (job.Job, job.Extras, error) {
	row, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if !row.CreatedBy.Valid || row.IsPrivate {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	return job.FromRow(row)
}

// Update writes the full resulting row for a moderator-authored job and queues it for the
// live facet index, in one transaction. The query's created_by scope means a missing or
// non-moderator-created slug affects no row (ErrNoRows → ErrJobNotFound). The Fields→params
// mapping lives on the aggregate (job.Fields.UpdateManualParams), shared with every other
// write path, so the derived columns move with the edited content instead of being left
// behind here.
//
// An edit changes precisely what search shows — the title, the body, the derived facets —
// so it needs the enqueue as much as Create does. The transaction is what the enqueue is
// for: writing the row and queueing it separately can leave the row written and unqueued,
// which is the state this was fixing.
func (r *QueriesRepository) Update(ctx context.Context, slug string, f job.Fields, actorID int64) (job.Job, job.Extras, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	row, err := qtx.UpdateManualJob(ctx, f.UpdateManualParams(slug, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if err := qtx.EnqueueSearchOutbox(ctx, row.ID); err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("enqueue search outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	return job.FromRow(row)
}
