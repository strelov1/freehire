package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// maxAttempts is the retry budget per post: the first failure leaves the post
// retryable (after its lease expires), the second dead-letters it.
const maxAttempts = 2

// extractStore adapts the generated queries + pool to telegram.ExtractStore.
// Complete writes every extracted job through the canonical UpsertJob (which
// upserts the company and gates on the dedup key) plus the enrichment enqueue,
// and marks the post extracted — all in one transaction, so a crash never
// half-persists a post.
type extractStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newExtractStore(pool *pgxpool.Pool) *extractStore {
	return &extractStore{pool: pool, q: db.New(pool)}
}

func (s *extractStore) Claim(ctx context.Context, leaseSeconds, batchSize int32) ([]telegram.PendingPost, error) {
	rows, err := s.q.ClaimTelegramPosts(ctx, db.ClaimTelegramPostsParams{
		LeaseSeconds: leaseSeconds,
		BatchSize:    batchSize,
	})
	if err != nil {
		return nil, err
	}
	posts := make([]telegram.PendingPost, len(rows))
	for i, r := range rows {
		posts[i] = telegram.PendingPost{
			Channel:  r.Channel,
			MsgID:    r.MsgID,
			Text:     r.Text,
			PostedAt: r.PostedAt.Time,
			Links:    decodeLinks(r.Links),
		}
	}
	return posts, nil
}

// decodeLinks unmarshals the stored links JSON, tolerating an empty/legacy NULL column.
func decodeLinks(b []byte) []telegram.Link {
	if len(b) == 0 {
		return nil
	}
	var links []telegram.Link
	if err := json.Unmarshal(b, &links); err != nil {
		return nil
	}
	return links
}

func (s *extractStore) Complete(ctx context.Context, post telegram.PendingPost, jobs []job.Job) error {
	return s.write(ctx, post, jobs)
}

// CompleteLinks writes link-resolved jobs — each already carrying the destination platform's
// own source identity rather than the Telegram post's. By the time they reach here that is the
// only thing that ever distinguished them from an extracted job, and it is already spent, so
// both go through one writer.
func (s *extractStore) CompleteLinks(ctx context.Context, post telegram.PendingPost, jobs []job.Job) error {
	return s.write(ctx, post, jobs)
}

// write persists every job through the canonical UpsertJob (which upserts the company and
// gates on the dedup key), enqueues enrichment for each, and marks the post extracted — all in
// one transaction, so a crash never half-persists a post.
//
// It maps and nothing else. Building the aggregate, and refusing a mis-extraction that has no
// title or identity, is the runner's — which is what lets a run report how many it refused
// instead of dropping them here and counting them as written.
func (s *extractStore) write(ctx context.Context, post telegram.PendingPost, jobs []job.Job) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	for _, j := range jobs {
		f := j.Fields()
		saved, err := qtx.UpsertJob(ctx, f.UpsertParams())
		if err != nil {
			return fmt.Errorf("upsert job %s/%s: %w", f.Source, f.ExternalID, err)
		}
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion: int32(enrich.Version),
			JobID:         saved.Job.ID,
		}); err != nil {
			return fmt.Errorf("enqueue enrichment %s/%s: %w", f.Source, f.ExternalID, err)
		}
	}

	if err := qtx.MarkTelegramPostExtracted(ctx, db.MarkTelegramPostExtractedParams{
		Channel: post.Channel,
		MsgID:   post.MsgID,
	}); err != nil {
		return fmt.Errorf("mark extracted %s/%d: %w", post.Channel, post.MsgID, err)
	}
	return tx.Commit(ctx)
}

func (s *extractStore) Fail(ctx context.Context, post telegram.PendingPost, errMsg string) error {
	_, err := s.q.RecordTelegramPostFailure(ctx, db.RecordTelegramPostFailureParams{
		LastError:   errMsg,
		MaxAttempts: maxAttempts,
		Channel:     post.Channel,
		MsgID:       post.MsgID,
	})
	return err
}

var _ telegram.ExtractStore = (*extractStore)(nil)
