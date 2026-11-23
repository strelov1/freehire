package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// dbStore adapts the generated queries + connection pool to enrich.Store. It is the
// only place the runner's domain operations meet the DB layer; the success path
// (SetJobEnrichment + DeleteEnrichmentEntry) runs in one transaction here.
type dbStore struct {
	pool           *pgxpool.Pool
	q              *db.Queries
	includeNonTech bool
}

func newDBStore(pool *pgxpool.Pool, includeNonTech bool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool), includeNonTech: includeNonTech}
}

// enqueueChunk is how many jobs one EnqueuePendingJobs call queues: small enough that each
// commits in seconds, so ingest's enqueue never waits long on its keys.
const enqueueChunk = 5000

// Enqueue walks the catalogue newest-first in committed chunks, so a run stopped mid-pass
// keeps what it queued.
// shortcut: no persisted cursor, every run restarts the pass from the newest id; persist one
// if the repeated scan shows up as load.
func (s *dbStore) Enqueue(ctx context.Context, targetVersion int) (int64, error) {
	var total, before int64
	for {
		r, err := s.q.EnqueuePendingJobs(ctx, db.EnqueuePendingJobsParams{
			BeforeID:       before,
			TargetVersion:  int32(targetVersion), //nolint:gosec // enrich.Version, a small constant
			IncludeNonTech: s.includeNonTech,
			ChunkSize:      enqueueChunk,
		})
		if err != nil {
			return total, err
		}
		total += r.Enqueued
		if r.LastID == 0 {
			return total, nil
		}
		before = r.LastID
	}
}

func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]enrich.Claimed, error) {
	rows, err := s.q.ClaimEnrichmentBatch(ctx, db.ClaimEnrichmentBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]enrich.Claimed, len(rows))
	for i, r := range rows {
		out[i] = enrich.Claimed{
			OutboxID:      r.ID,
			JobID:         r.JobID,
			TargetVersion: int(r.TargetVersion),
		}
	}
	return out, nil
}

// reapChunk is how many queue entries one DeleteIneligibleEnrichmentOutbox call reads, so each
// statement stays short however long the queue grows.
const reapChunk = 100_000

// Reap walks the queue in job_id chunks until it has deleted maxRows entries or read them all.
func (s *dbStore) Reap(ctx context.Context, maxRows int) (int64, error) {
	var reaped int64
	for after := int64(0); reaped < int64(maxRows); {
		r, err := s.q.DeleteIneligibleEnrichmentOutbox(ctx, db.DeleteIneligibleEnrichmentOutboxParams{
			AfterJobID: after,
			ChunkSize:  reapChunk,
			MaxRows:    int32(int64(maxRows) - reaped), //nolint:gosec // bounded by maxRows, an int32-sized limit
		})
		if err != nil {
			return reaped, err
		}
		reaped += r.Deleted
		if r.LastJobID == 0 {
			break
		}
		after = r.LastJobID
	}
	return reaped, nil
}

func (s *dbStore) Job(ctx context.Context, id int64) (enrich.JobInput, error) {
	j, err := s.q.GetJob(ctx, id)
	if err != nil {
		return enrich.JobInput{}, err
	}
	return enrich.JobInput{
		Title:       j.Title,
		Company:     j.Company,
		Location:    j.Location,
		Remote:      j.Remote,
		Description: j.Description,
		URL:         j.URL,
		// The dictionary-derived country/region columns tell the prompt builder to skip
		// the geo ask when geography is already pinned (see enrich.GeoPinned).
		GeoPinned: enrich.GeoPinned(j.Countries, j.Regions),
		// A curated hint for a hand-picked set of known outsourcing/outstaffing
		// companies (see enrich.CompanyTypeHints); empty for every other company.
		CompanyTypeHint: enrich.CompanyTypeHints[j.CompanySlug],
	}, nil
}

func (s *dbStore) Complete(ctx context.Context, entry enrich.Claimed, payload json.RawMessage) error {
	if entry.TargetVersion < math.MinInt32 || entry.TargetVersion > math.MaxInt32 {
		return fmt.Errorf("enrichment target version is outside the database integer range")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if _, err := qtx.LockJobForEnrichmentCompletion(ctx, entry.JobID); err != nil {
		return fmt.Errorf("lock enrichment posting: %w", err)
	}
	owns, err := qtx.HasEnrichmentCompletionClaim(ctx, db.HasEnrichmentCompletionClaimParams{OutboxID: entry.OutboxID, JobID: entry.JobID, TargetVersion: int32(entry.TargetVersion)})
	if err != nil {
		return fmt.Errorf("check enrichment claim: %w", err)
	}
	if !owns {
		return tx.Commit(ctx)
	}
	if err := qtx.SetJobEnrichment(ctx, db.SetJobEnrichmentParams{
		Enrichment:        payload,
		EnrichedAt:        pgconv.Instant(time.Now()),
		EnrichmentVersion: int32(entry.TargetVersion),
		ID:                entry.JobID,
	}); err != nil {
		return fmt.Errorf("set enrichment: %w", err)
	}
	// The enrichment.* facets (seniority, salary, category...) live in the search document,
	// so the job goes back on the drain; without it they reached search only at a full rebuild.
	// Requeue, not Enqueue: ingest queued a new job minutes ago, and if the drain has that
	// entry claimed, a DO NOTHING enqueue would be deleted with it, facets unpushed.
	if err := qtx.RequeueSearchOutbox(ctx, entry.JobID); err != nil {
		return fmt.Errorf("enqueue search outbox: %w", err)
	}
	if err := qtx.DeleteEnrichmentEntry(ctx, entry.OutboxID); err != nil {
		return fmt.Errorf("delete outbox entry: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, errMsg string, policy enrich.FailurePolicy) (bool, error) {
	row, err := s.q.RecordEnrichmentFailure(ctx, db.RecordEnrichmentFailureParams{
		LastError:         errMsg,
		PostingAtFault:    policy.PostingAtFault,
		MaxAttempts:       int32(policy.MaxAttempts),
		UpstreamGraceDays: int32(policy.UpstreamGraceDays),
		ID:                outboxID,
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}
