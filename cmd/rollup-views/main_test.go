//go:build integration

// Integration test for the nginx-log view-rollup worker. It aggregates real log
// files into jobs.view_count / job_daily_views and tracks processed files by
// content signature, all of which is SQL + filesystem behavior only verifiable against a
// real Postgres. Run with: go test -tags=integration ./cmd/rollup-views/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/viewlog"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedJob(t *testing.T, pool *pgxpool.Pool, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', $1, 'http://example.test', 'J', $1) RETURNING id`, slug).Scan(&id); err != nil {
		t.Fatalf("seed job %q: %v", slug, err)
	}
	return id
}

func viewCount(t *testing.T, pool *pgxpool.Pool, id int64) int32 {
	t.Helper()
	var v int32
	if err := pool.QueryRow(context.Background(),
		"SELECT view_count FROM jobs WHERE id = $1", id).Scan(&v); err != nil {
		t.Fatalf("read view_count: %v", err)
	}
	return v
}

func logLine(ip, path, ua, ts string) string {
	return fmt.Sprintf(`%s - - [%s] "GET %s HTTP/2.0" 200 0 "-" "%s"`, ip, ts, path, ua)
}

// writeLog writes a rotated log file and returns the temp dir it lives in.
func writeLog(t *testing.T, name string, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestProcessAppliesAndIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	j1 := seedJob(t, pool, "acme")
	j2 := seedJob(t, pool, "globex")

	const day = "21/Jul/2026:12:00:00 +0000"
	dir := writeLog(t, "access.log.1",
		logLine("1.1.1.1", "/jobs/acme", "human1", day),         // acme: visitor A (page)
		logLine("1.1.1.1", "/jobs/acme", "human1", day),         // repeat A -> deduped
		logLine("2.2.2.2", "/api/v1/jobs/acme", "curl", day),    // acme: visitor B (api)
		logLine("3.3.3.3", "/jobs/globex", "human3", day),       // globex: 1
		logLine("4.4.4.4", "/jobs/ghost-nonexistent", "h", day), // unknown slug -> ignored
	)

	files, err := viewlog.RotatedFiles(dir, "access.log")
	if err != nil {
		t.Fatal(err)
	}

	nFiles, nViews, err := process(ctx, pool, files)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if nFiles != 1 {
		t.Errorf("nFiles = %d, want 1", nFiles)
	}
	if nViews != 3 {
		t.Errorf("nViews = %d, want 3 (acme 2 + globex 1)", nViews)
	}
	if v := viewCount(t, pool, j1); v != 2 {
		t.Errorf("acme view_count = %d, want 2", v)
	}
	if v := viewCount(t, pool, j2); v != 1 {
		t.Errorf("globex view_count = %d, want 1", v)
	}

	// The daily rollup carries the same per-(day, job) uniques.
	var daily int32
	if err := pool.QueryRow(ctx,
		"SELECT uniques FROM job_daily_views WHERE job_id = $1 AND day = DATE '2026-07-21'", j1).Scan(&daily); err != nil {
		t.Fatalf("read job_daily_views: %v", err)
	}
	if daily != 2 {
		t.Errorf("acme daily uniques = %d, want 2", daily)
	}

	// Re-running over the same file must NOT double-count (processed-file cursor).
	nFiles2, _, err := process(ctx, pool, files)
	if err != nil {
		t.Fatalf("re-run process: %v", err)
	}
	if nFiles2 != 0 {
		t.Errorf("re-run nFiles = %d, want 0 (already processed)", nFiles2)
	}
	if v := viewCount(t, pool, j1); v != 2 {
		t.Errorf("acme view_count after re-run = %d, want 2 (idempotent)", v)
	}
}

// TestProcessSkipsGzippedCopyOfProcessedFile is the regression guard for the cursor
// design: a file applied while uncompressed must not be re-applied once logrotate
// gzips it (a new inode, same content). The content signature must recognize it.
func TestProcessSkipsGzippedCopyOfProcessedFile(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	j := seedJob(t, pool, "acme")

	const ts = "21/Jul/2026:12:00:00 +0000"
	content := logLine("1.1.1.1", "/jobs/acme", "human", ts) + "\n" +
		logLine("2.2.2.2", "/jobs/acme", "human2", ts)

	// 1. Apply the uncompressed rotated file.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "access.log.1"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := viewlog.RotatedFiles(dir, "access.log")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := process(ctx, pool, files); err != nil {
		t.Fatalf("first process: %v", err)
	}
	if v := viewCount(t, pool, j); v != 2 {
		t.Fatalf("after uncompressed apply: view_count = %d, want 2", v)
	}

	// 2. logrotate compresses the same content into a new file (new inode). A
	//    backfill run must recognize the content and skip it — no double-count.
	gzDir := t.TempDir()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(content))
	zw.Close()
	if err := os.WriteFile(filepath.Join(gzDir, "access.log.2.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	gzFiles, err := viewlog.RotatedFiles(gzDir, "access.log")
	if err != nil {
		t.Fatal(err)
	}
	nFiles, _, err := process(ctx, pool, gzFiles)
	if err != nil {
		t.Fatalf("gzip process: %v", err)
	}
	if nFiles != 0 {
		t.Errorf("gzip re-run nFiles = %d, want 0 (same content already processed)", nFiles)
	}
	if v := viewCount(t, pool, j); v != 2 {
		t.Errorf("view_count after gzip re-run = %d, want 2 (no double-count across compression)", v)
	}
}
