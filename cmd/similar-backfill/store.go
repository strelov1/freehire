package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

// dbStore adapts the generated queries to similarjobs.Store. Every operation but
// NearestJobs is a single independent statement (a SELECT, a job-to-job join query, or
// a one-row UPDATE) — no transaction needed. NearestJobs is the one exception: it needs
// a transaction-scoped SET LOCAL, so it keeps its own connection via pool.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

func (s *dbStore) PendingJobIDs(ctx context.Context, limit int) ([]int64, error) {
	return s.q.SelectJobsNeedingSimilarBackfill(ctx, int32(limit))
}

// JobGeneration reads jobID's current chunk-generation marker. A NULL
// semantic_embedded_hash (pgtype.Text with Valid=false) surfaces as "", which
// SetSimilarJobIDs's generation guard still compares correctly: IS NOT DISTINCT FROM
// treats SQL NULL and Go's zero-valued pgtype.Text the same way on the write side.
func (s *dbStore) JobGeneration(ctx context.Context, jobID int64) (string, error) {
	hash, err := s.q.GetJobSemanticGeneration(ctx, jobID)
	if err != nil {
		return "", err
	}
	return hash.String, nil
}

// overFetchMultiplier bounds how many nearest chunks NearestJobsToJob's per-source-chunk
// LATERAL probe asks pgvector for, past the final limit — headroom to absorb whatever
// the closed-job/same-company filters discard, without falling back to the whole-table
// scan the pre-rewrite query needed (see that query's comment). 10x is a starting
// heuristic, not a measured value; widen it if a source job in a company-dense cluster
// starts coming back under-filled.
const overFetchMultiplier = 10

// NearestJobs wraps db.NearestJobsToJob, which already returns rows ordered nearest
// (lowest distance) first — this just projects out the job ids in that same order.
//
// hnsw.ef_search (pgvector's HNSW candidate-list size, default 40) bounds how many
// candidates the index scan itself can return — below overFetch, each LATERAL probe's
// `LIMIT over_fetch` silently gets fewer rows than requested, no error, just a
// short-filled (and less accurate) result. Raised via SET LOCAL inside a transaction so
// it never leaks onto a pooled connection's next, unrelated query.
func (s *dbStore) NearestJobs(ctx context.Context, jobID int64, limit int) ([]int64, error) {
	overFetch := limit * overFetchMultiplier

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", overFetch)); err != nil {
		return nil, fmt.Errorf("set hnsw.ef_search: %w", err)
	}
	rows, err := s.q.WithTx(tx).NearestJobsToJob(ctx, db.NearestJobsToJobParams{
		JobID:      jobID,
		LimitCount: int32(limit),
		OverFetch:  int32(overFetch),
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.JobID
	}
	return ids, nil
}

func (s *dbStore) SetSimilarJobIDs(ctx context.Context, jobID int64, similarJobIDs []int64, generation string) (bool, error) {
	rows, err := s.q.SetSimilarJobIDs(ctx, db.SetSimilarJobIDsParams{
		ID:                 jobID,
		SimilarJobIds:      similarJobIDs,
		ExpectedGeneration: pgtype.Text{String: generation, Valid: generation != ""},
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
