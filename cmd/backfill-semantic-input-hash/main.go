// Command backfill-semantic-input-hash fills jobs.semantic_embedded_input_hash for the
// jobs embedded before the column existed (migrations/0192), then exits.
//
// cmd/embed stamps the column whenever it writes a job's chunks, and checks it before
// re-embedding: a job whose content_hash moved on a derived facet alone is re-stamped
// without a TEI call. A job with NO stored input hash cannot be told apart that way and is
// re-embedded as before — so until this pass runs, the next dictionary or geography
// deploy re-embeds every already-embedded job it touches, the ~92k-row wave of
// 2026-10-21 that this column exists to stop.
//
// Only jobs whose stamp is CURRENT are filled — see BackfillSemanticInputHashChunk for
// why a stale one must stay NULL.
//
// Run it repeatedly and it costs nothing: each chunk's UPDATE only touches rows still
// NULL, so a re-run writes nothing and stopping mid-way is free.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// defaultChunkSize is how many ids one UPDATE spans, overridable with
// BACKFILL_SEMANTIC_INPUT_HASH_CHUNK. An id RANGE, not a row count: measured 2026-10-22
// the ~12.7M jobs rows are spread over ~1.6 billion ids, so 1M ids is ~8k rows read and
// some hundreds of embedded ones hashed — each of those de-TOASTs a description, which is
// what bounds how wide one statement should be.
const defaultChunkSize = 1_000_000

// pauseBetweenChunks lets the host breathe between statements; the pass is never urgent.
const pauseBetweenChunks = 200 * time.Millisecond

// chunkSize is the binary's own reading of its knob, so a test asserts THIS worker's
// wiring rather than re-stating it.
func chunkSize() (int64, error) {
	return worker.EnvInt64("BACKFILL_SEMANTIC_INPUT_HASH_CHUNK", defaultChunkSize)
}

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	step, err := chunkSize()
	if err != nil {
		log.Printf("backfill-semantic-input-hash: %v", err)
		return 1
	}

	q := db.New(pool)
	bounds, err := q.SemanticInputHashBackfillBounds(ctx)
	if err != nil {
		log.Printf("backfill-semantic-input-hash: bounds: %v", err)
		return 1
	}
	model := search.CurrentEmbedderModel()
	log.Printf("backfill-semantic-input-hash: ids %d..%d, model %s, chunk=%d (%d statements)",
		bounds.MinID, bounds.MaxID, model, step, (bounds.MaxID-bounds.MinID)/step+1)

	var filled int64
	lastLog := time.Now()
	for from := bounds.MinID; from <= bounds.MaxID; from += step {
		n, err := q.BackfillSemanticInputHashChunk(ctx, db.BackfillSemanticInputHashChunkParams{
			FromID:      from,
			ToID:        from + step,
			TargetModel: model,
		})
		if err != nil {
			// Every chunk is its own transaction, so what was filled survives and a re-run
			// resumes from it.
			log.Printf("backfill-semantic-input-hash: chunk %d..%d after %d filled: %v", from, from+step, filled, err)
			return 1
		}
		filled += n

		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-semantic-input-hash: progress filled=%d at id=%d of %d", filled, from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-semantic-input-hash: cancelled after %d filled, resume by re-running", filled)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}
	log.Printf("backfill-semantic-input-hash: done, filled=%d", filled)
	return 0
}
