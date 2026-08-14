package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

// dbStore adapts the generated queries to similarjobs.Store. Unlike cmd/embed's
// dbStore, no transaction is needed here: each operation is a single independent
// statement (a SELECT, a job-to-job join query, or a one-row UPDATE), not a
// multi-table commit that must land atomically.
type dbStore struct {
	q *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{q: db.New(pool)}
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

// NearestJobs wraps db.NearestJobsToJob, which already returns rows ordered nearest
// (lowest distance) first — this just projects out the job ids in that same order.
func (s *dbStore) NearestJobs(ctx context.Context, jobID int64, limit int) ([]int64, error) {
	rows, err := s.q.NearestJobsToJob(ctx, db.NearestJobsToJobParams{
		JobID:      jobID,
		LimitCount: int32(limit),
	})
	if err != nil {
		return nil, err
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
