package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

// dbStore adapts the generated queries + connection pool to enrich.Store. It is the
// only place the runner's domain operations meet the DB layer; the success path
// (SetJobEnrichment + DeleteEnrichmentEntry) runs in one transaction here.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

func (s *dbStore) Enqueue(ctx context.Context, targetVersion int) (int64, error) {
	return s.q.EnqueuePendingJobs(ctx, db.EnqueuePendingJobsParams{
		TargetVersion:     int32(targetVersion),
		ExcludeCategories: enrich.NonTechCategories,
	})
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
	}, nil
}

func (s *dbStore) Complete(ctx context.Context, entry enrich.Claimed, payload json.RawMessage) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if err := qtx.SetJobEnrichment(ctx, db.SetJobEnrichmentParams{
		Enrichment:        payload,
		EnrichedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
		EnrichmentVersion: int32(entry.TargetVersion),
		ID:                entry.JobID,
	}); err != nil {
		return fmt.Errorf("set enrichment: %w", err)
	}
	if err := qtx.DeleteEnrichmentEntry(ctx, entry.OutboxID); err != nil {
		return fmt.Errorf("delete outbox entry: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordEnrichmentFailure(ctx, db.RecordEnrichmentFailureParams{
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
		ID:          outboxID,
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}
