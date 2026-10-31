//go:build integration

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/viewlog"
	"github.com/strelov1/freehire/internal/platform/db"
)

// chunkFixture seeds six jobs and one rotated log over two days, and returns the jobs and
// the per-job figures a single transaction applying the whole file would leave. Job i has
// i page visitors and one API visitor on the first day, so `uniques` (i+1) and
// `page_uniques` (i) differ on every row — a write that conflated the two columns cannot
// pass. Jobs 2 and 4 are seen on a second day too, so a job spans two batch rows and a
// chunk boundary can fall between them.
type chunkWant struct{ uniques, page, viewCount int32 }

func chunkFixture(t *testing.T, pool *pgxpool.Pool) ([]viewlog.LogFile, []int64, []chunkWant) {
	t.Helper()
	const day1 = "21/Jul/2026:12:00:00 +0000"
	const day2 = "22/Jul/2026:12:00:00 +0000"
	var lines []string
	var ids []int64
	var want []chunkWant
	for i := 1; i <= 6; i++ {
		slug := fmt.Sprintf("job-%d", i)
		ids = append(ids, seedJob(t, pool, slug))
		for v := 0; v < i; v++ {
			lines = append(lines, logLine(fmt.Sprintf("10.0.%d.%d", i, v), "/jobs/"+slug, "human", day1))
		}
		lines = append(lines, logLine(fmt.Sprintf("10.1.%d.1", i), "/api/v1/jobs/"+slug, "curl", day1))
		w := chunkWant{uniques: int32(i + 1), page: int32(i), viewCount: int32(i + 1)}
		if i == 2 || i == 4 {
			lines = append(lines, logLine("10.2.0.1", "/jobs/"+slug, "human", day2))
			w.viewCount++
		}
		want = append(want, w)
	}
	dir := writeLog(t, "access.log.1", lines...)
	files, err := viewlog.RotatedFiles(dir, "access.log")
	if err != nil {
		t.Fatal(err)
	}
	return files, ids, want
}

func checkChunkTotals(t *testing.T, pool *pgxpool.Pool, ids []int64, want []chunkWant) {
	t.Helper()
	for i, id := range ids {
		var uniques, page int32
		if err := pool.QueryRow(context.Background(),
			"SELECT uniques, page_uniques FROM job_daily_views WHERE job_id = $1 AND day = DATE '2026-07-21'",
			id).Scan(&uniques, &page); err != nil {
			t.Fatalf("job %d: read job_daily_views: %v", i+1, err)
		}
		if uniques != want[i].uniques || page != want[i].page {
			t.Errorf("job %d: uniques/page_uniques = %d/%d, want %d/%d",
				i+1, uniques, page, want[i].uniques, want[i].page)
		}
		if v := viewCount(t, pool, id); v != want[i].viewCount {
			t.Errorf("job %d: view_count = %d, want %d", i+1, v, want[i].viewCount)
		}
	}
}

func withChunkRows(t *testing.T, n int) {
	t.Helper()
	prev := applyChunkRows
	applyChunkRows = n
	t.Cleanup(func() { applyChunkRows = prev })
}

// Committing in chunks must leave exactly what one transaction over the whole file did.
func TestProcessChunkedMatchesOneTransaction(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	withChunkRows(t, 3) // 8 batch rows -> chunks of 3, 3, 2

	files, ids, want := chunkFixture(t, pool)
	nFiles, nViews, err := process(ctx, pool, files)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if nFiles != 1 || nViews != 29 { // sum(i+1) for i=1..6 = 27, plus 2 second-day views
		t.Errorf("got %d files / %d views, want 1 / 29", nFiles, nViews)
	}
	checkChunkTotals(t, pool, ids, want)

	if n, _, err := process(ctx, pool, files); err != nil || n != 0 {
		t.Fatalf("re-run: %d files, err %v; want 0 files", n, err)
	}
	checkChunkTotals(t, pool, ids, want)
}

// A run that fails part-way leaves the chunks before the failure committed. The re-run
// must resume after them and not add their views a second time.
func TestProcessResumesAfterAPartialFailure(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	withChunkRows(t, 3)

	files, ids, want := chunkFixture(t, pool)

	// Job 5 sits in the third chunk: make its daily row unwritable for the first run.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION fail_job_5() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.job_id = %d THEN RAISE EXCEPTION 'injected failure'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER fail_job_5 BEFORE INSERT ON job_daily_views
			FOR EACH ROW EXECUTE FUNCTION fail_job_5();`, ids[4])); err != nil {
		t.Fatal(err)
	}
	if _, _, err := process(ctx, pool, files); err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("first run err = %v, want the injected failure", err)
	}
	// Jobs 1-4 (the first two chunks) are applied; job 5 onwards is not, and the file is
	// not marked finished.
	if v := viewCount(t, pool, ids[0]); v != want[0].viewCount {
		t.Errorf("job 1 after the failed run: view_count = %d, want %d (its chunk committed)", v, want[0].viewCount)
	}
	if v := viewCount(t, pool, ids[5]); v != 0 {
		t.Errorf("job 6 after the failed run: view_count = %d, want 0", v)
	}

	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_job_5 ON job_daily_views`); err != nil {
		t.Fatal(err)
	}
	nFiles, nViews, err := process(ctx, pool, files)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if nFiles != 1 || nViews != 29-16 { // jobs 1-4 carried 2+3+4+5 + 2 second-day = 16
		t.Errorf("re-run: %d files / %d views, want 1 / 13 (only what the failed run left)", nFiles, nViews)
	}
	checkChunkTotals(t, pool, ids, want)
}

// Two runs over one file: a run whose cursor is stale (another run moved it) must apply
// nothing rather than a chunk the other run already applied.
func TestAdvanceViewLogCursorRefusesAStaleCursor(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)
	day := pgtype.Date{Time: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), Valid: true}

	advance := func(jobID int64, prev *int64) int64 {
		t.Helper()
		p := db.AdvanceViewLogCursorParams{Signature: 7, Filename: "f", JobID: jobID, Day: day}
		if prev != nil {
			p.PrevJobID = pgtype.Int8{Int64: *prev, Valid: true}
			p.PrevDay = day
		}
		n, err := q.AdvanceViewLogCursor(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	ten := int64(10)
	if n := advance(10, nil); n != 1 {
		t.Fatalf("first advance = %d, want 1", n)
	}
	if n := advance(20, nil); n != 0 {
		t.Errorf("advance from no cursor over an existing one = %d, want 0", n)
	}
	if n := advance(20, &ten); n != 1 {
		t.Errorf("advance from the current cursor = %d, want 1", n)
	}
	if n := advance(30, &ten); n != 0 {
		t.Errorf("advance from a stale cursor = %d, want 0", n)
	}
}

// The agent pass is chunked too, and resumes the same way.
func TestProcessAgentResumesAfterAPartialFailure(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	withChunkRows(t, 2)

	var ids []int64
	var lines strings.Builder
	for i := 1; i <= 5; i++ {
		ids = append(ids, seedJob(t, pool, fmt.Sprintf("agent-%d", i)))
		for v := 0; v < i; v++ {
			fmt.Fprintf(&lines, `{"ts":"2026-10-26T10:00:00Z","kind":"detail","surface":"mcp","viewer":"v%d","job_ids":[%d]}`+"\n", v, ids[i-1])
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent-views-2026-10-26.jsonl"), []byte(lines.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION fail_agent_4() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.job_id = %d THEN RAISE EXCEPTION 'injected failure'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER fail_agent_4 BEFORE INSERT ON job_daily_views
			FOR EACH ROW EXECUTE FUNCTION fail_agent_4();`, ids[3])); err != nil {
		t.Fatal(err)
	}
	if _, _, err := processAgent(ctx, pool, dir, now); err == nil {
		t.Fatal("first run: want the injected failure")
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_agent_4 ON job_daily_views`); err != nil {
		t.Fatal(err)
	}
	nFiles, nViews, err := processAgent(ctx, pool, dir, now)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if nFiles != 1 || nViews != 3+4+5 { // jobs 1-2 (3 views) committed in the failed run
		t.Errorf("re-run: %d files / %d views, want 1 / 12", nFiles, nViews)
	}
	for i, id := range ids {
		if v := viewCount(t, pool, id); v != int32(i+1) {
			t.Errorf("agent job %d: view_count = %d, want %d", i+1, v, i+1)
		}
	}
}
