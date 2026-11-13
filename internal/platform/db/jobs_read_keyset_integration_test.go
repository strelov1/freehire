//go:build integration

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The keyset scans of the whole catalogue (cmd/reindex, cmd/backfill-derive, cmd/search-drain's
// rebuilds) page by `WHERE id > $after ORDER BY id LIMIT $batch`. They read jobs_read, which
// joins job_activity. A LEFT JOIN cannot carry `j.id > $after` to the nullable side, so a plain
// `LEFT JOIN job_activity a ON a.job_id = j.id` lets the planner choose a Merge Left Join that
// walks job_activity_pkey from the FIRST id up to the cursor to emit one batch: each page costs
// the whole prefix, so a pass over the catalogue is quadratic. Measured on a 1.2M-row synthetic
// copy: 12,679 buffers for one 1000-row page at the midpoint against 258 on the table alone.
//
// The view therefore joins with LEFT JOIN LATERAL (... WHERE x.job_id = j.id LIMIT 1), which can
// only be a parameterised primary-key probe per emitted row. This test pins that on the plan the
// generated statements actually get, by counting the rows read from job_activity in EXPLAIN
// (ANALYZE) and requiring the count to follow the batch, not the cursor. It asserts rows, never
// time, and sets no planner setting: only the statistics the planner would have in production
// (ANALYZE after the load; a fresh test database has none and the planner picks differently).
const (
	keysetRows  = 12000
	keysetBatch = 50
)

// planNode is the part of an EXPLAIN (FORMAT JSON) node this test reads.
type planNode struct {
	NodeType      string     `json:"Node Type"`
	RelationName  string     `json:"Relation Name"`
	IndexName     string     `json:"Index Name"`
	ActualRows    float64    `json:"Actual Rows"`
	ActualLoops   float64    `json:"Actual Loops"`
	FilteredRows  float64    `json:"Rows Removed by Filter"`
	RecheckedRows float64    `json:"Rows Removed by Index Recheck"`
	Plans         []planNode `json:"Plans"`
}

// rowsRead sums Actual Rows x Actual Loops over every node that reads the named relation.
func (n planNode) rowsRead(relation string) float64 {
	var total float64
	if n.RelationName == relation {
		total += (n.ActualRows + n.FilteredRows + n.RecheckedRows) * n.ActualLoops
	}
	for _, child := range n.Plans {
		total += child.rowsRead(relation)
	}
	return total
}

// explainRowsRead runs EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) over the statement as generated
// and returns the rows read from relation, plus the node types of the plan for the failure text.
func explainRowsRead(t *testing.T, pool *pgxpool.Pool, relation, sql string, args ...any) (float64, string) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		t.Fatalf("explain: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode explain: %v (%d plans) in %s", err, len(plans), raw)
	}
	return plans[0].Plan.rowsRead(relation), nodeTypes(plans[0].Plan)
}

func nodeTypes(n planNode) string {
	out := n.NodeType
	if n.IndexName != "" {
		out += "(" + n.IndexName + ")"
	}
	for _, c := range n.Plans {
		out += " > " + nodeTypes(c)
	}
	return out
}

func TestJobsReadKeysetScansReadActivityPerBatchNotPerPrefix(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// One statement, so ids are 1..keysetRows; the insert trigger gives each job its activity row.
	if _, err := pool.Exec(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, description)
		SELECT 'greenhouse', 'acme:' || g, 'https://example.test/' || g, 'Engineer ' || g, 'slug-' || g, 'Acme', 'acme', 'Body ' || g
		FROM generate_series(1, $1) g`, keysetRows); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE jobs`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE job_activity`); err != nil {
		t.Fatal(err)
	}

	// Each generated statement with its arguments for a cursor.
	type scan struct {
		name string
		sql  string
		args func(after int64) []any
	}
	scans := []scan{
		{"ListIndexableJobsByIDAfter", listIndexableJobsByIDAfter, func(a int64) []any { return []any{a, int32(keysetBatch)} }},
		{"ListJobsByIDAfter", listJobsByIDAfter, func(a int64) []any { return []any{a, int32(keysetBatch)} }},
		{"ListLiveJobsByIDAfter", listLiveJobsByIDAfter, func(a int64) []any {
			return []any{a, pgtype.Timestamptz{Valid: true}, int32(keysetBatch)}
		}},
		{"ListJobsBySourceAfter", listJobsBySourceAfter, func(a int64) []any { return []any{"greenhouse", a, int32(keysetBatch)} }},
	}
	// The early, middle and late cursor: the bad plan reads cursor + batch rows at every one.
	cursors := []int64{keysetRows / 10, keysetRows / 2, keysetRows * 9 / 10}

	// A page may read a few more rows than it emits (the batch is probed per emitted row), never
	// the prefix. The prefix at the smallest cursor is already 24 times the bound.
	const bound = 2 * keysetBatch
	for _, s := range scans {
		for _, after := range cursors {
			t.Run(fmt.Sprintf("%s/after=%d", s.name, after), func(t *testing.T) {
				read, shape := explainRowsRead(t, pool, "job_activity", s.sql, s.args(after)...)
				if read > bound {
					t.Errorf("read %.0f job_activity rows for a %d-row page after id %d (bound %d; the id prefix is %d): "+
						"the keyset scan joins job_activity by walking it from the first id. Plan: %s",
						read, keysetBatch, after, bound, after, shape)
				}
			})
		}
	}

	// The statement still answers: a page of the batch size carrying the activity stamps.
	page, err := q.ListIndexableJobsByIDAfter(ctx, ListIndexableJobsByIDAfterParams{AfterID: keysetRows / 2, BatchSize: keysetBatch})
	if err != nil {
		t.Fatalf("ListIndexableJobsByIDAfter: %v", err)
	}
	if len(page) != keysetBatch || !page[0].LastSeenAt.Valid {
		t.Fatalf("page = %d rows, first last_seen_at valid = %v; want %d rows carrying the activity stamps",
			len(page), len(page) > 0 && page[0].LastSeenAt.Valid, keysetBatch)
	}
}
