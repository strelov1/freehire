//go:build integration

// Integration tests for the apply-form store and its capture queue — the "one current
// form per job" replace, the enqueue gate that keeps a posting from being fetched once
// per ingest run, and the claim/lease/dead-letter semantics copied from
// enrichment_outbox. All of it is SQL behavior, so it can only be verified against a
// real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// applyFormPayload returns a minimal captured-form document. The store is deliberately
// incurious about the shape — it holds what the platform said — so the tests only need
// two distinguishable documents.
func applyFormPayload(marker string) []byte {
	return []byte(`{"fields":[{"id":"` + marker + `"}]}`)
}

// applyFormMarker reads back the marker applyFormPayload wrote. Postgres stores jsonb
// decomposed, so it returns the document reformatted — comparing raw bytes would be
// asserting on Postgres's whitespace, not on which capture won.
func applyFormMarker(t *testing.T, payload []byte) string {
	t.Helper()
	var doc struct {
		Fields []struct {
			ID string `json:"id"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("decode payload %s: %v", payload, err)
	}
	if len(doc.Fields) != 1 {
		t.Fatalf("payload carried %d fields, want 1: %s", len(doc.Fields), payload)
	}
	return doc.Fields[0].ID
}

// storedApplyForm returns the form stored for a job and fails unless there is exactly
// one. "At most one current form per job" is the store's entire shape, so it is asserted
// on every read rather than in a test of its own.
func storedApplyForm(t *testing.T, pool *pgxpool.Pool, jobID int64) (provider string, payload []byte) {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT provider, payload FROM apply_forms WHERE job_id = $1", jobID)
	if err != nil {
		t.Fatalf("select apply_forms: %v", err)
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		if err := rows.Scan(&provider, &payload); err != nil {
			t.Fatalf("scan apply_form: %v", err)
		}
		found++
	}
	if found != 1 {
		t.Fatalf("stored forms = %d, want 1 (a capture replaces, never accumulates)", found)
	}
	return provider, payload
}

func queuedCaptureJobIDs(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT job_id FROM apply_form_outbox ORDER BY job_id")
	if err != nil {
		t.Fatalf("select apply_form_outbox: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan job_id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestApplyFormStore(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a second capture replaces the first", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "gh-1")

		for _, marker := range []string{"first", "second"} {
			if err := q.UpsertApplyForm(ctx, UpsertApplyFormParams{
				JobID:    job,
				Provider: "greenhouse",
				Payload:  applyFormPayload(marker),
			}); err != nil {
				t.Fatalf("upsert %s: %v", marker, err)
			}
		}

		provider, payload := storedApplyForm(t, pool, job)
		if provider != "greenhouse" {
			t.Errorf("provider = %q, want %q", provider, "greenhouse")
		}
		if got := applyFormMarker(t, payload); got != "second" {
			t.Errorf("stored capture = %q, want %q (the later one)", got, "second")
		}
	})
}

func TestApplyFormEnqueue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a job with no form is queued", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "gh-new")

		if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
			t.Fatal(err)
		}

		got := queuedCaptureJobIDs(t, pool)
		if len(got) != 1 || got[0] != job {
			t.Errorf("queued = %v, want [%d]", got, job)
		}
	})

	t.Run("a job that already has a form is not queued", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "gh-captured")
		if err := q.UpsertApplyForm(ctx, UpsertApplyFormParams{
			JobID: job, Provider: "greenhouse", Payload: applyFormPayload("already"),
		}); err != nil {
			t.Fatal(err)
		}

		if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
			t.Fatal(err)
		}

		if got := queuedCaptureJobIDs(t, pool); len(got) != 0 {
			t.Errorf("queued = %v, want none — a captured posting must not be re-fetched every run", got)
		}
	})

	t.Run("enqueueing twice queues once", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "gh-twice")

		for range 2 {
			if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
				t.Fatal(err)
			}
		}

		if got := queuedCaptureJobIDs(t, pool); len(got) != 1 {
			t.Errorf("queued = %v, want exactly one entry", got)
		}
	})
}

func TestApplyFormClaim(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("the freshest posting is claimed first and carries its platform identity", func(t *testing.T) {
		truncate(t, pool)
		// Insert the older posting first so its outbox id is lower — proving the claim
		// orders by posted_at, not by insertion order.
		older := insertJob(t, pool, "board:older")
		newer := insertJob(t, pool, "board:newer")
		setPostedAt(t, pool, older, "2024-01-01T00:00:00Z")
		setPostedAt(t, pool, newer, "2024-06-01T00:00:00Z")
		for _, id := range []int64{older, newer} {
			if _, err := q.EnqueueApplyFormCapture(ctx, id); err != nil {
				t.Fatal(err)
			}
		}

		claimed, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 2 {
			t.Fatalf("claim: rows=%d err=%v, want 2", len(claimed), err)
		}
		if claimed[0].JobID != newer || claimed[1].JobID != older {
			t.Errorf("claim order = [%d, %d], want [%d, %d] (fresher first)",
				claimed[0].JobID, claimed[1].JobID, newer, older)
		}
		// The worker builds its fetch from the row alone, so the claim must carry the
		// provider and the board-namespaced posting id.
		if claimed[0].Source != "test" || claimed[0].ExternalID != "board:newer" {
			t.Errorf("claimed identity = (%q, %q), want (%q, %q)",
				claimed[0].Source, claimed[0].ExternalID, "test", "board:newer")
		}
	})

	t.Run("a claimed entry is leased away from a second claim", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "leased")
		if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
			t.Fatal(err)
		}

		if _, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		again, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 0 {
			t.Errorf("second claim = %d rows, want 0 while the lease holds", len(again))
		}
	})

	t.Run("an expired lease is reclaimed without a reaper", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "expired")
		if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
			t.Fatal(err)
		}
		if _, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}

		again, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 1 || again[0].JobID != job {
			t.Errorf("reclaim = %v, want the entry back once its lease expired", again)
		}
	})
}

func TestApplyFormCompletionAndFailure(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("completing a capture removes it from the queue", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "done")
		if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v", len(claimed), err)
		}

		if err := q.DeleteApplyFormEntry(ctx, claimed[0].ID); err != nil {
			t.Fatal(err)
		}

		if got := queuedCaptureJobIDs(t, pool); len(got) != 0 {
			t.Errorf("queue = %v, want empty after completion", got)
		}
	})

	t.Run("a failure is recorded and retried until attempts run out", func(t *testing.T) {
		truncate(t, pool)
		job := insertJob(t, pool, "flaky")
		if _, err := q.EnqueueApplyFormCapture(ctx, job); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v", len(claimed), err)
		}
		id := claimed[0].ID

		first, err := q.RecordApplyFormFailure(ctx, RecordApplyFormFailureParams{
			ID: id, LastError: "boom", MaxAttempts: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if first.FailedAt.Valid {
			t.Fatal("first failure dead-lettered, want it retried")
		}
		// The lease is deliberately left standing, so the entry is out of reach for the
		// rest of this run and comes back to a later one — modelled here by a lapsed lease.
		if held, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		} else if len(held) != 0 {
			t.Errorf("reclaim within the same run = %d rows, want 0", len(held))
		}
		retry, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil || len(retry) != 1 {
			t.Fatalf("reclaim on a later run: rows=%d err=%v, want 1", len(retry), err)
		}

		second, err := q.RecordApplyFormFailure(ctx, RecordApplyFormFailureParams{
			ID: id, LastError: "boom again", MaxAttempts: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !second.FailedAt.Valid {
			t.Error("second failure not dead-lettered, want it marked failed at max attempts")
		}

		final, err := q.ClaimApplyFormBatch(ctx, ClaimApplyFormBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(final) != 0 {
			t.Errorf("claim after dead-letter = %d rows, want 0", len(final))
		}
	})
}
