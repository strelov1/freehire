//go:build integration

package main

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// stat is one insights_company_stats row as the tests spell it.
type stat struct {
	Slug                 string
	Day                  string
	Added, Removed, Open int32
}

func (s stat) key() string { return s.Slug + "|" + s.Day }

func params(rows []stat) []db.ComputeInsightsCompanyStatsRow {
	out := make([]db.ComputeInsightsCompanyStatsRow, len(rows))
	for i, r := range rows {
		d, err := time.Parse("2006-01-02", r.Day)
		if err != nil {
			panic(err)
		}
		out[i] = db.ComputeInsightsCompanyStatsRow{
			CompanySlug: r.Slug, Day: pgtype.Date{Time: d, Valid: true},
			Added: r.Added, Removed: r.Removed, Open: r.Open,
		}
	}
	return out
}

// readStats returns the whole table, ordered, and each row's tuple identity. A row that
// was written by a statement gets a new xmin and ctid; a row nothing touched keeps both,
// which is how the tests tell "unchanged" from "rewritten to the same values".
func readStats(t *testing.T, pool *pgxpool.Pool) ([]stat, map[string]string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT company_slug, to_char(day,'YYYY-MM-DD'), added, removed, open,
		xmin::text || '/' || ctid::text FROM insights_company_stats ORDER BY company_slug, day`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := []stat{}
	ident := map[string]string{}
	for rows.Next() {
		var s stat
		var id string
		if err := rows.Scan(&s.Slug, &s.Day, &s.Added, &s.Removed, &s.Open, &id); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
		ident[s.key()] = id
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got, ident
}

func publish(t *testing.T, pool *pgxpool.Pool, rows []stat) statsPublication {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	res, err := publishCompanyStats(ctx, tx, params(rows))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return res
}

func assertSnapshot(t *testing.T, pool *pgxpool.Pool, want []stat) map[string]string {
	t.Helper()
	got, ident := readStats(t, pool)
	if want == nil {
		want = []stat{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot mismatch\n got  %+v\n want %+v", got, want)
	}
	return ident
}

// assertUntouched fails if any of keys was rewritten between two reads of the table.
func assertUntouched(t *testing.T, before, after map[string]string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if before[k] == "" || before[k] != after[k] {
			t.Errorf("row %s was rewritten: %q -> %q", k, before[k], after[k])
		}
	}
}

func assertRewritten(t *testing.T, before, after map[string]string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if before[k] == "" || before[k] == after[k] {
			t.Errorf("row %s was not rewritten: %q -> %q", k, before[k], after[k])
		}
	}
}

// TestPublishCompanyStatsDifferences walks one table through the snapshots a daily rerun
// meets, asserting the FULL table and the change counts after each, and that rows nothing
// changed keep their tuple.
func TestPublishCompanyStatsDifferences(t *testing.T) {
	pool := testdb.Pool(t)

	a1 := stat{"a", "2026-01-01", 2, 0, 2}
	a2 := stat{"a", "2026-01-02", 1, 1, 2}
	a3 := stat{"a", "2026-01-03", 0, 1, 1}
	b1 := stat{"b", "2026-01-01", 1, 0, 1}

	steps := []struct {
		name                       string
		rows                       []stat
		inserted, updated, deleted int64
		untouched, rewritten       []string // keys, judged against the previous step
	}{
		{name: "first publication into an empty table", rows: []stat{a1, a2, a3, b1}, inserted: 4},
		{name: "rerun of the same snapshot", rows: []stat{a1, a2, a3, b1},
			untouched: []string{a1.key(), a2.key(), a3.key(), b1.key()}},
		{
			// A late posting on a's first day raises added, and open with it on every later
			// day of a: three rows change though only one day's events did.
			name:    "correction of an old day shifts the later running totals",
			rows:    []stat{{"a", "2026-01-01", 3, 0, 3}, {"a", "2026-01-02", 1, 1, 3}, {"a", "2026-01-03", 0, 1, 2}, b1},
			updated: 3, untouched: []string{b1.key()}, rewritten: []string{a1.key(), a2.key(), a3.key()},
		},
		{
			// A reopened job removes a's removal day; b is renamed to b2.
			name:     "vanished day and renamed company",
			rows:     []stat{{"a", "2026-01-01", 3, 0, 3}, {"a", "2026-01-03", 0, 0, 3}, {"b2", "2026-01-01", 1, 0, 1}},
			inserted: 1, updated: 1, deleted: 2, untouched: []string{a1.key()},
		},
		{
			name:     "companies merged into one slug",
			rows:     []stat{{"m", "2026-01-01", 4, 0, 4}, {"m", "2026-01-03", 0, 0, 4}},
			inserted: 2, deleted: 3,
		},
		{name: "empty snapshot empties the table", rows: nil, deleted: 2},
		{name: "repopulated after being emptied", rows: []stat{a1, b1}, inserted: 2},
	}

	prev := map[string]string{}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			res := publish(t, pool, step.rows)
			if res.inserted != step.inserted || res.updated != step.updated || res.deleted != step.deleted {
				t.Errorf("changes = +%d ~%d -%d, want +%d ~%d -%d",
					res.inserted, res.updated, res.deleted, step.inserted, step.updated, step.deleted)
			}
			want := append([]stat(nil), step.rows...)
			slices.SortFunc(want, func(a, b stat) int { return strings.Compare(a.key(), b.key()) })
			cur := assertSnapshot(t, pool, want)
			assertUntouched(t, prev, cur, step.untouched...)
			assertRewritten(t, prev, cur, step.rewritten...)
			prev = cur
		})
	}
}

// A publication that does not commit leaves the table as it was, and the stage with it.
func TestPublishCompanyStatsRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	keep := []stat{{"a", "2026-01-01", 1, 0, 1}, {"b", "2026-01-01", 2, 0, 2}}
	publish(t, pool, keep)
	before := assertSnapshot(t, pool, keep)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publishCompanyStats(ctx, tx, params([]stat{{"c", "2026-01-02", 9, 9, 9}})); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	after := assertSnapshot(t, pool, keep)
	assertUntouched(t, before, after, keep[0].key(), keep[1].key())
}

// Two rows for one key cannot come out of the query, but if they ever did the publication
// must fail rather than keep an arbitrary one.
func TestPublishCompanyStatsRejectsDuplicateKeys(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = publishCompanyStats(ctx, tx, params([]stat{{"a", "2026-01-01", 1, 0, 1}, {"a", "2026-01-01", 2, 0, 2}}))
	if err == nil {
		t.Fatal("duplicate (company, day) keys were published")
	}
}

func insertJob(t *testing.T, pool *pgxpool.Pool, n int, company, created, closed string) {
	t.Helper()
	closedAt := "NULL"
	reason := "''"
	if closed != "" {
		closedAt = "'" + closed + " 12:00:00+00'"
		reason = "'unseen'"
	}
	q := fmt.Sprintf(`INSERT INTO jobs(public_slug,source,external_id,title,url,company,company_slug,created_at,closed_at,closed_reason)
		VALUES ('rc-%d','test','rc-%d','Engineer','https://example.com/%d','%s','%s','%s 12:00:00+00',%s,%s)`,
		n, n, n, company, company, created, closedAt, reason)
	if _, err := pool.Exec(context.Background(), q); err != nil {
		t.Fatal(err)
	}
}

// TestRebuildCompanyRollupsPublishesStatsDifferentially drives the whole worker: the
// snapshot computed on the analytics database, run after run, against jobs that change.
func TestRebuildCompanyRollupsPublishesStatsDifferentially(t *testing.T) {
	ctx := context.Background()
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	rebuild := func() {
		t.Helper()
		if err := rebuildCompanyRollups(ctx, analytics, primary); err != nil {
			t.Fatal(err)
		}
	}

	insertJob(t, analytics, 1, "x", "2026-01-05", "2026-01-07")
	insertJob(t, analytics, 2, "x", "2026-01-06", "")
	insertJob(t, analytics, 3, "y", "2026-01-05", "")
	rebuild()
	first := assertSnapshot(t, primary, []stat{
		{"x", "2026-01-05", 1, 0, 1}, {"x", "2026-01-06", 1, 0, 2}, {"x", "2026-01-07", 0, 1, 1},
		{"y", "2026-01-05", 1, 0, 1},
	})
	var growth int
	if err := primary.QueryRow(ctx, `SELECT count(*) FROM insights_company_growth`).Scan(&growth); err != nil || growth != 2 {
		t.Fatalf("the other company tables were not published with it: growth rows=%d err=%v", growth, err)
	}

	rebuild()
	second := assertSnapshot(t, primary, []stat{
		{"x", "2026-01-05", 1, 0, 1}, {"x", "2026-01-06", 1, 0, 2}, {"x", "2026-01-07", 0, 1, 1},
		{"y", "2026-01-05", 1, 0, 1},
	})
	assertUntouched(t, first, second, "x|2026-01-05", "x|2026-01-06", "x|2026-01-07", "y|2026-01-05")

	// A posting dated before x's first day: every later running total of x moves, y does not.
	insertJob(t, analytics, 4, "x", "2026-01-04", "")
	rebuild()
	third := assertSnapshot(t, primary, []stat{
		{"x", "2026-01-04", 1, 0, 1}, {"x", "2026-01-05", 1, 0, 2}, {"x", "2026-01-06", 1, 0, 3}, {"x", "2026-01-07", 0, 1, 2},
		{"y", "2026-01-05", 1, 0, 1},
	})
	assertUntouched(t, second, third, "y|2026-01-05")
	assertRewritten(t, second, third, "x|2026-01-05", "x|2026-01-06", "x|2026-01-07")

	// x merged into y: x's rows go, y absorbs its history.
	if _, err := analytics.Exec(ctx, `UPDATE jobs SET company_slug='y', company='y' WHERE company_slug='x'`); err != nil {
		t.Fatal(err)
	}
	rebuild()
	assertSnapshot(t, primary, []stat{
		{"y", "2026-01-04", 1, 0, 1}, {"y", "2026-01-05", 2, 0, 3}, {"y", "2026-01-06", 1, 0, 4}, {"y", "2026-01-07", 0, 1, 3},
	})

	// Nothing left to report: the published table empties, as a full rebuild did.
	if _, err := analytics.Exec(ctx, `DELETE FROM jobs`); err != nil {
		t.Fatal(err)
	}
	rebuild()
	assertSnapshot(t, primary, nil)
}

// All five snapshots publish in one transaction: when a later table fails, the stats
// table is left exactly as it was.
func TestRebuildCompanyRollupsKeepsStatsWhenALaterTableFails(t *testing.T) {
	ctx := context.Background()
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	insertJob(t, analytics, 1, "x", "2026-01-05", "")
	sentinel := []stat{{"sentinel", "2026-01-01", 1, 0, 1}}
	publish(t, primary, sentinel)
	before := assertSnapshot(t, primary, sentinel)

	if _, err := primary.Exec(ctx, `ALTER TABLE insights_company_growth ADD CONSTRAINT never CHECK (false)`); err != nil {
		t.Fatal(err)
	}
	if err := rebuildCompanyRollups(ctx, analytics, primary); err == nil {
		t.Fatal("rebuild succeeded although the growth table rejects every row")
	}
	after := assertSnapshot(t, primary, sentinel)
	assertUntouched(t, before, after, "sentinel|2026-01-01")
}
