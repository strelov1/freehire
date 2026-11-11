//go:build integration

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestTargetedRunPreservesIneligibleTargetsAndUnrelatedExpiry(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	seed := func(name, source string, private, old, deadline bool) int64 {
		t.Helper()
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO jobs
   (source,external_id,url,title,public_slug,is_private,posted_at,last_seen_at,application_deadline,liveness_strikes)
   VALUES ($1,$2,'https://127.0.0.1/never-probe',$2,$2,$3,
    now()-CASE WHEN $4 THEN interval '90 days' ELSE interval '0 days' END,
    now()-CASE WHEN $4 THEN interval '90 days' ELSE interval '0 days' END,
    CASE WHEN $5 THEN (now() AT TIME ZONE 'UTC')::date-1 END,1) RETURNING id`,
			source, name, private, old, deadline).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	fresh := seed("target-fresh", "instaffo", false, false, false)
	private := seed("target-private", "manual", true, true, false)
	seed("unrelated-deadline", "greenhouse", false, false, true)
	seed("unrelated-telegram", "telegram", false, true, false)
	seed("unrelated-whatjobs", "whatjobs", false, true, false)

	// Prove all three bulk paths have real work in this fixture, then roll it
	// back: an accidentally unscoped run must produce observable collateral closes.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	if n, err := q.CloseJobsPastApplicationDeadline(ctx, ""); err != nil || n != 1 {
		t.Fatalf("deadline control: count=%d err=%v", n, err)
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-expiryWindow), Valid: true}
	if n, err := q.CloseStaleUnsignalledJobs(ctx, db.CloseStaleUnsignalledJobsParams{Sources: []string{"telegram"}, Cutoff: cutoff}); err != nil || n != 1 {
		t.Fatalf("telegram control: count=%d err=%v", n, err)
	}
	fallbackSources, sweepGrace := registeredAgeFallback()
	if !slices.Contains(fallbackSources, "whatjobs") {
		t.Fatal("WhatJobs control is not a real registered age-fallback source")
	}
	if n, err := q.CloseStaleUnseenUnprobeableJobs(ctx, db.CloseStaleUnseenUnprobeableJobsParams{Sources: fallbackSources, Cutoff: cutoff, SeenCutoff: pgtype.Timestamptz{Time: time.Now().Add(-unseenWindow(fallbackSources, sweepGrace)), Valid: true}}); err != nil || n != 1 {
		t.Fatalf("registered fallback control: count=%d err=%v", n, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	// Bootstrap must connect only to this disposable test database. Dummy feed
	// credentials satisfy the live-registry startup guard without making API calls.
	t.Setenv("DATABASE_URL", pool.Config().ConnString())
	t.Setenv("ADZUNA_APP_ID", "integration-dummy")
	t.Setenv("ADZUNA_APP_KEY", "integration-dummy")
	t.Setenv("WHATJOBS_PUBLISHER_IDS", "us:integration-dummy")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	oldFlags, oldArgs, oldLog := flag.CommandLine, os.Args, log.Writer()
	t.Cleanup(func() { flag.CommandLine, os.Args = oldFlags, oldArgs; log.SetOutput(oldLog) })
	flag.CommandLine = flag.NewFlagSet("liveness-integration", flag.ContinueOnError)
	os.Args = []string{"liveness", "-job-ids=" + fmt.Sprintf("%d,%d", fresh, private)}
	var logs bytes.Buffer
	log.SetOutput(&logs)
	if code := run(); code != 0 {
		t.Fatalf("run exit=%d:\n%s", code, logs.String())
	}
	// Both requested rows are ineligible: the registered posting is fresh and the
	// manual posting is private. Zero probes also proves no HTTP fetch was attempted.
	if !strings.Contains(logs.String(), "liveness done: probed=0 closed=0 struck=0 failed=0") {
		t.Fatalf("targeted run attempted work outside its eligible scope:\n%s", logs.String())
	}
	var total, changed, deletions int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE closed_at IS NOT NULL OR closed_reason<>'' OR liveness_strikes<>1) FROM jobs`).Scan(&total, &changed); err != nil {
		t.Fatal(err)
	}
	if total != 5 || changed != 0 {
		t.Fatalf("targeted run changed seeded jobs: total=%d changed=%d", total, changed)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox`).Scan(&deletions); err != nil {
		t.Fatal(err)
	}
	if deletions != 0 {
		t.Fatalf("targeted run queued %d collateral search deletions", deletions)
	}
}
