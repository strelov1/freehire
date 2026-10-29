//go:build integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestProcessAgentAppliesFinishedDaysOnce(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)

	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		 VALUES ('test', 'x1', 'http://example.test', 'A job', 'job-x1', 'acme') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	dir := t.TempDir()
	line := func(ts, kind, viewer string) string {
		return `{"ts":"` + ts + `","kind":"` + kind + `","surface":"mcp","viewer":"` + viewer + `","job_ids":[` + itoa(id) + `]}` + "\n"
	}
	past := line("2026-10-26T10:00:00Z", "search", "a") +
		line("2026-10-26T10:01:00Z", "detail", "a") +
		line("2026-10-26T11:00:00Z", "detail", "b")
	if err := os.WriteFile(filepath.Join(dir, "agent-views-2026-10-26.jsonl"), []byte(past), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fixed, inside the fixture's retention: a wall clock would let the first run delete the
	// file once it ages out, and the second run would then pass without reaching the cursor.
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	today := line(now.UTC().Format(time.RFC3339), "detail", "c")
	if err := os.WriteFile(filepath.Join(dir, agentview.FileName(now)), []byte(today), 0o644); err != nil {
		t.Fatal(err)
	}

	nFiles, nViews, err := processAgent(ctx, pool, dir, now)
	if err != nil {
		t.Fatalf("processAgent: %v", err)
	}
	if nFiles != 1 || nViews != 2 {
		t.Fatalf("got %d files / %d views, want 1 / 2", nFiles, nViews)
	}

	check := func() {
		t.Helper()
		var agent, uniques, page int
		if err := pool.QueryRow(ctx,
			`SELECT agent_uniques, uniques, page_uniques FROM job_daily_views WHERE job_id = $1 AND day = '2026-10-26'`, id).
			Scan(&agent, &uniques, &page); err != nil {
			t.Fatalf("read daily: %v", err)
		}
		if agent != 2 || uniques != 0 || page != 0 {
			t.Errorf("agent/uniques/page = %d/%d/%d, want 2/0/0", agent, uniques, page)
		}
		var vc int
		if err := pool.QueryRow(ctx, `SELECT view_count FROM jobs WHERE id = $1`, id).Scan(&vc); err != nil {
			t.Fatal(err)
		}
		if vc != 2 {
			t.Errorf("view_count = %d, want 2", vc)
		}
	}
	check()

	nFiles, _, err = processAgent(ctx, pool, dir, now)
	if err != nil {
		t.Fatalf("second processAgent: %v", err)
	}
	if nFiles != 0 {
		t.Errorf("second run applied %d files, want 0", nFiles)
	}
	check()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
