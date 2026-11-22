//go:build integration

package main

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type fakeIndex struct {
	got []map[int64]int64
	err error
}

func (f *fakeIndex) SetViewCounts(_ context.Context, counts map[int64]int64) error {
	f.got = append(f.got, maps.Clone(counts))
	return f.err
}

func TestPushViewCountsWritesOnlyNewSteps(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	insert := func(ext string, views int) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, view_count)
			 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, 'acme', $2) RETURNING id`,
			ext, views).Scan(&id); err != nil {
			t.Fatalf("insert job: %v", err)
		}
		return id
	}
	crossed := insert("v1", 5)   // 4 -> 5: reaches the floor
	jumped := insert("v2", 50)   // 3 -> 50: both passes of one run, summed
	low := insert("v3", 2)       // 1 -> 2: under the floor
	sameStep := insert("v4", 41) // 40 -> 41: inside one step
	closed := insert("v5", 5)    // crosses the floor, but search never returns it
	dup := insert("v6", 5)       // likewise for a duplicate copy
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed); err != nil {
		t.Fatalf("close job: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, crossed, dup); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}
	q := db.New(pool)
	// A touched id with no jobs row (deleted since) is simply not pushed.
	deltas := map[int64]int64{crossed: 1, jumped: 47, low: 1, sameStep: 1, closed: 1, dup: 1, crossed + 1_000_000: 3}

	idx := &fakeIndex{}
	n, skipped, err := pushViewCounts(ctx, q, idx, deltas, time.Now().Add(time.Hour))
	if err != nil || n != 2 || skipped != 0 {
		t.Fatalf("pushViewCounts = %d, %d, %v; want 2, 0, nil", n, skipped, err)
	}
	want := map[int64]int64{crossed: 5, jumped: 50}
	if len(idx.got) != 1 || !maps.Equal(idx.got[0], want) {
		t.Fatalf("index got %v, want one task with %v", idx.got, want)
	}

	failing := &fakeIndex{err: errors.New("meili down")}
	if n, _, err := pushViewCounts(ctx, q, failing, deltas, time.Now().Add(time.Hour)); err == nil || n != 0 {
		t.Fatalf("failing index: pushViewCounts = %d, %v; want 0 and the error", n, err)
	}
}
