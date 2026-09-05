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
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/viewlog"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
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

	// The daily rollup carries the same per-(day, job) uniques, and page_uniques
	// carries only the page opens. acme had one page visitor and one API visitor, so
	// the two columns must disagree — a `page_uniques = uniques` write would pass a
	// test that only read one of them, and would then rank the digest on crawler
	// traffic in public.
	var daily, pageDaily int32
	if err := pool.QueryRow(ctx,
		"SELECT uniques, page_uniques FROM job_daily_views WHERE job_id = $1 AND day = DATE '2026-07-21'",
		j1).Scan(&daily, &pageDaily); err != nil {
		t.Fatalf("read job_daily_views: %v", err)
	}
	if daily != 2 {
		t.Errorf("acme daily uniques = %d, want 2", daily)
	}
	if pageDaily != 1 {
		t.Errorf("acme daily page_uniques = %d, want 1 (the API visitor is not a page open)", pageDaily)
	}

	// globex was opened only through the page, so both columns agree there.
	var globexUniques, globexPage int32
	if err := pool.QueryRow(ctx,
		"SELECT uniques, page_uniques FROM job_daily_views WHERE job_id = $1 AND day = DATE '2026-07-21'",
		j2).Scan(&globexUniques, &globexPage); err != nil {
		t.Fatalf("read job_daily_views: %v", err)
	}
	if globexUniques != 1 || globexPage != 1 {
		t.Errorf("globex = %d/%d uniques/page_uniques, want 1/1", globexUniques, globexPage)
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

// A day whose lines span two rotated files must sum across both, in BOTH counters.
// This is what the additive ON CONFLICT buys, and page_uniques is a separate term in
// that statement — a version that carried only `uniques` would pass every other test
// here and then quietly report half a day's page views to the digest.
func TestProcessSumsBothCountersAcrossTwoFiles(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	job := seedJob(t, pool, "acme")
	const day = "21/Jul/2026:12:00:00 +0000"

	// First file: one page visitor and one API visitor.
	dir1 := writeLog(t, "access.log.1",
		logLine("1.1.1.1", "/jobs/acme", "human1", day),
		logLine("2.2.2.2", "/api/v1/jobs/acme", "curl", day),
	)
	// Second file, same calendar day: two more page visitors.
	dir2 := writeLog(t, "access.log.1",
		logLine("3.3.3.3", "/jobs/acme", "human3", day),
		logLine("4.4.4.4", "/jobs/acme", "human4", day),
	)

	for _, dir := range []string{dir1, dir2} {
		files, err := viewlog.RotatedFiles(dir, "access.log")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := process(ctx, pool, files); err != nil {
			t.Fatalf("process %s: %v", dir, err)
		}
	}

	var uniques, pageUniques int32
	if err := pool.QueryRow(ctx,
		"SELECT uniques, page_uniques FROM job_daily_views WHERE job_id = $1 AND day = DATE '2026-07-21'",
		job).Scan(&uniques, &pageUniques); err != nil {
		t.Fatalf("read job_daily_views: %v", err)
	}
	if uniques != 4 {
		t.Errorf("uniques = %d, want 4 (2 + 2 across both files)", uniques)
	}
	if pageUniques != 3 {
		t.Errorf("page_uniques = %d, want 3 (1 + 2; the API visitor is not a page open)", pageUniques)
	}
	if v := viewCount(t, pool, job); v != 4 {
		t.Errorf("view_count = %d, want 4", v)
	}
}
