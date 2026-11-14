//go:build integration

package main

import (
	"context"
	"errors"
	"maps"
	"testing"

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

func TestPushViewCountsWritesCurrentCounts(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, view_count)
		 VALUES ('test', 'v1', 'http://example.test', 'A job', 'job-v1', 'acme', 7) RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	q := db.New(pool)
	// A touched id with no jobs row (deleted since) is simply not pushed.
	touched := map[int64]struct{}{id: {}, id + 1_000_000: {}}

	idx := &fakeIndex{}
	n, err := pushViewCounts(ctx, q, idx, touched)
	if err != nil || n != 1 {
		t.Fatalf("pushViewCounts = %d, %v; want 1, nil", n, err)
	}
	if len(idx.got) != 1 || idx.got[0][id] != 7 || len(idx.got[0]) != 1 {
		t.Fatalf("index got %v, want one task with {%d: 7}", idx.got, id)
	}

	failing := &fakeIndex{err: errors.New("meili down")}
	if n, err := pushViewCounts(ctx, q, failing, touched); err == nil || n != 0 {
		t.Fatalf("failing index: pushViewCounts = %d, %v; want 0 and the error", n, err)
	}
}
