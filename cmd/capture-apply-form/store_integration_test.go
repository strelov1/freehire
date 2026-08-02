//go:build integration

// Integration tests for the capture worker's store adapter: the atomic store-and-retire,
// and the failure accounting the runner leans on.
// Run with: go test -tags=integration ./cmd/capture-apply-form/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/testdb"
)

// queueOne inserts a posting and queues a capture for it, returning both ids.
func queueOne(t *testing.T, pool *pgxpool.Pool, source, externalID string) (jobID, outboxID int64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ($1, $2, 'http://example.test', 'A job', 'job-' || $2) RETURNING id`,
		source, externalID).Scan(&jobID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO apply_form_outbox (job_id) VALUES ($1) RETURNING id`, jobID).Scan(&outboxID); err != nil {
		t.Fatalf("queue capture: %v", err)
	}
	return jobID, outboxID
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestStoreClaimCarriesThePostingIdentity(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool)
	_, outboxID := queueOne(t, pool, "greenhouse", "stripe:800001")

	claims, err := store.Claim(context.Background(), 50, 3600)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	var got applyform.Claimed
	for _, c := range claims {
		if c.OutboxID == outboxID {
			got = c
		}
	}
	if got.OutboxID == 0 {
		t.Fatalf("capture %d not among %d claims", outboxID, len(claims))
	}
	// The worker builds its fetch from the claim alone, so both halves have to be there.
	if got.Provider != "greenhouse" || got.ExternalID != "stripe:800001" {
		t.Errorf("claim = (%q, %q), want (%q, %q)", got.Provider, got.ExternalID, "greenhouse", "stripe:800001")
	}
}

func TestStoreSaveStoresTheFormAndRetiresTheCapture(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool)
	jobID, outboxID := queueOne(t, pool, "ashby", "n8n:800002")

	form := applyform.Form{
		Provider: "ashby",
		Fields:   []applyform.Field{{ID: "_systemfield_email", Label: "Email", Type: applyform.TypeText}},
	}
	if err := store.Save(ctx, outboxID, jobID, form); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := countRows(t, pool, `SELECT count(*) FROM apply_forms WHERE job_id = $1`, jobID); n != 1 {
		t.Errorf("stored %d forms, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM apply_form_outbox WHERE id = $1`, outboxID); n != 0 {
		t.Errorf("capture still queued (%d rows), want it retired", n)
	}

	// The stored provider comes from the form, which is what the worker actually captured
	// — not from the job row, which is only where the fetch was aimed.
	var provider string
	if err := pool.QueryRow(ctx, `SELECT provider FROM apply_forms WHERE job_id = $1`, jobID).Scan(&provider); err != nil {
		t.Fatalf("read provider: %v", err)
	}
	if provider != "ashby" {
		t.Errorf("provider = %q, want %q", provider, "ashby")
	}
}

func TestStoreFailRetriesThenDeadLetters(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool)
	_, outboxID := queueOne(t, pool, "greenhouse", "stripe:800003")

	dead, err := store.Fail(ctx, outboxID, "upstream 500", 2)
	if err != nil {
		t.Fatalf("first Fail: %v", err)
	}
	if dead {
		t.Fatal("dead-lettered on the first failure, want a retry")
	}

	dead, err = store.Fail(ctx, outboxID, "upstream 500 again", 2)
	if err != nil {
		t.Fatalf("second Fail: %v", err)
	}
	if !dead {
		t.Error("not dead-lettered at max attempts")
	}

	// The reason survives, because a dead letter nobody can explain is a dead end.
	var lastError string
	if err := pool.QueryRow(ctx, `SELECT last_error FROM apply_form_outbox WHERE id = $1`, outboxID).Scan(&lastError); err != nil {
		t.Fatalf("read last_error: %v", err)
	}
	if lastError != "upstream 500 again" {
		t.Errorf("last_error = %q, want the most recent reason", lastError)
	}
}

// A dead-lettered capture is out of the queue's reach — no later run re-reads it.
func TestStoreDeadLetteredCaptureIsNoLongerClaimed(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool)
	_, outboxID := queueOne(t, pool, "greenhouse", "stripe:800004")

	if _, err := store.Fail(ctx, outboxID, "gone", 1); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	claims, err := store.Claim(ctx, 100, 0)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	for _, c := range claims {
		if c.OutboxID == outboxID {
			t.Error("a dead-lettered capture was claimed again")
		}
	}
}
