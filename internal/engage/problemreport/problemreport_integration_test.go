//go:build integration

package problemreport

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestRecordAccountQueueQuotaAndSanitizedLogs(t *testing.T) {
	pool := testdb.Pool(t)
	//nolint:staticcheck // Uses the same exported context key as RequestIDs middleware.
	ctx := context.WithValue(t.Context(), observability.RequestIDKey, "server-request-id")
	var user, other int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email) VALUES ('problem-one@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email) VALUES ('problem-two@example.test') RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	service := New(pool, db.New(pool))
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	in := sampleInput()
	in.Actual = "Failure contacting private@example.test with Bearer secretvalue"
	first, err := service.Record(ctx, user, in)
	if err != nil || first.ID == 0 || first.Status != "open" || first.RequestCount != 1 || first.FirstReportedAt.IsZero() {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if strings.Contains(logs.String(), "private@example.test") || strings.Contains(logs.String(), "secretvalue") || strings.Contains(logs.String(), "Failure contacting") {
		t.Fatalf("logs contain payload: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "server-request-id") || !strings.Contains(logs.String(), "problem_report_recorded") {
		t.Fatalf("missing correlation: %s", logs.String())
	}
	var actual, requestID string
	if err := pool.QueryRow(ctx, `SELECT actual, request_id FROM problem_reports WHERE id=$1`, first.ID).Scan(&actual, &requestID); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(actual, "secretvalue") || strings.Contains(actual, "private@example.test") || requestID != "server-request-id" {
		t.Fatalf("stored payload/correlation: %s %s", actual, requestID)
	}
	second, err := service.Record(ctx, user, in)
	if err != nil || second.ID != first.ID || second.RequestCount != 2 {
		t.Fatalf("repeat=%+v err=%v", second, err)
	}
	separate, err := service.Record(ctx, other, in)
	if err != nil || separate.ID == first.ID || separate.RequestCount != 1 {
		t.Fatalf("cross-account=%+v err=%v", separate, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE problem_reports SET status='resolved' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := service.Record(ctx, user, in)
	if err != nil || reopened.ID == first.ID || reopened.RequestCount != 1 {
		t.Fatalf("reopened=%+v err=%v", reopened, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE problem_report_daily SET writes=20 WHERE user_id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, user, in); !errors.Is(err, ErrDailyCap) {
		t.Fatalf("cap=%v", err)
	}
	var count int32
	if err := pool.QueryRow(ctx, `SELECT request_count FROM problem_reports WHERE id=$1`, reopened.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("capped write changed queue: %d %v", count, err)
	}
}

func TestRecordConcurrentQuotaAndRollback(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := t.Context()
	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email) VALUES ('problem-race@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	service := New(pool, db.New(pool))
	// A failed queue write after claiming quota must roll back that claim.
	if _, err := pool.Exec(ctx, `ALTER TABLE problem_reports ADD CONSTRAINT test_failure CHECK (goal <> 'force storage failure')`); err != nil {
		t.Fatal(err)
	}
	failed := sampleInput()
	failed.Goal = "force storage failure"
	if receipt, err := service.Record(ctx, user, failed); err == nil || receipt.ID != 0 {
		t.Fatalf("DB failure returned success: %+v %v", receipt, err)
	}
	var writes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM problem_report_daily WHERE user_id=$1`, user).Scan(&writes); err != nil || writes != 0 {
		t.Fatalf("failed report consumed quota: %d %v", writes, err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 25)
	for range 25 {
		wg.Go(func() { _, err := service.Record(ctx, user, sampleInput()); results <- err })
	}
	wg.Wait()
	close(results)
	accepted, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrDailyCap):
			refused++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 20 || refused != 5 {
		t.Fatalf("accepted=%d refused=%d", accepted, refused)
	}
	if err := pool.QueryRow(ctx, `SELECT request_count FROM problem_reports WHERE user_id=$1`, user).Scan(&writes); err != nil || writes != 20 {
		t.Fatalf("queue count=%d err=%v", writes, err)
	}
}
