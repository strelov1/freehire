//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/search/search"
)

// The backfill computes the input hash in SQL and cmd/embed computes it in Go, and the
// two must agree byte for byte: a drift would not fail anywhere, it would make every
// backfilled job look changed and re-embed it. Only a job whose stamp is current is
// filled — a stale one's text may differ from what its vectors were computed from.
func TestIntegration_BackfillMatchesEmbedInputHash(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	model := search.CurrentEmbedderModel()

	seed := func(ext, title, company, description string, current bool) int64 {
		t.Helper()
		var id int64
		err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, company, description, public_slug,
			                   content_hash, enrichment, is_tech, semantic_embedded_model, semantic_embedded_hash)
			 VALUES ('test', $1, 'http://example.test', $2, $3, $4, 'job-' || $1,
			         'h-' || $1, '{}', true, $5, CASE WHEN $6 THEN 'h-' || $1 ELSE 'h-older' END)
			 RETURNING id`,
			ext, title, company, description, model, current).Scan(&id)
		if err != nil {
			t.Fatalf("seed %s: %v", ext, err)
		}
		return id
	}
	// Non-ASCII on purpose: the SQL side hashes convert_to(..., 'UTF8'), Go hashes the
	// string's bytes, and only a multi-byte text tells those apart.
	current := seed("current", "Разработчик Go — Senior", "Acme GmbH & Co. KG", "<p>Ünïcode, \"quotes\" and\nnewlines.</p>", true)
	stale := seed("stale", "Go Engineer", "Acme", "Old text.", false)

	bounds, err := q.SemanticInputHashBackfillBounds(ctx)
	if err != nil {
		t.Fatalf("bounds: %v", err)
	}
	n, err := q.BackfillSemanticInputHashChunk(ctx, db.BackfillSemanticInputHashChunkParams{
		FromID: bounds.MinID, ToID: bounds.MaxID + 1, TargetModel: model,
	})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 1 {
		t.Errorf("filled %d rows, want 1 (the current one)", n)
	}

	rows, err := q.GetJobsByIDs(ctx, []int64{current, stale})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, j := range rows {
		switch j.ID {
		case current:
			if want := search.EmbedInputHash(j); !j.SemanticEmbeddedInputHash.Valid || j.SemanticEmbeddedInputHash.String != want {
				t.Errorf("SQL hash = %v, Go hash = %q; they must agree", j.SemanticEmbeddedInputHash, want)
			}
		case stale:
			if j.SemanticEmbeddedInputHash.Valid {
				t.Errorf("stale job got input hash %q, want NULL", j.SemanticEmbeddedInputHash.String)
			}
		}
	}

	// A re-run writes nothing.
	if n, err := q.BackfillSemanticInputHashChunk(ctx, db.BackfillSemanticInputHashChunkParams{
		FromID: bounds.MinID, ToID: bounds.MaxID + 1, TargetModel: model,
	}); err != nil || n != 0 {
		t.Errorf("re-run filled %d (err %v), want 0", n, err)
	}
}
