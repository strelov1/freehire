// Command backfill-semantic-vectors seeds Postgres with the semantic vectors that
// currently live ONLY in the Meilisearch `jobs_semantic` index. Those vectors are the
// product of a one-time, weeks-long embedding pass; until they exist in Postgres too,
// the nightly pg_dump does not back them up and a lost Meili volume would force a full
// re-embed. This one-off pages the index (id + stored vector), copies each vector into
// jobs.semantic_embedding, and exits.
//
// It is pure data movement: it reads the already-computed vectors and NEVER calls the
// embedding model, NEVER touches the provenance stamps (semantic_embedded_model/hash)
// or the semantic_outbox, and NEVER changes the embedder identity — so it cannot
// trigger a re-embed of anything. Idempotent: re-running rewrites the same values, so
// an interrupted run just resumes by starting over.
//
// Env knobs: DRY_RUN (any non-empty value reads Meili and writes nothing);
// PAGE_SIZE (documents per Meili page + Postgres batch, default 500).
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	pageSize := envInt("PAGE_SIZE", 500)
	dryRun := os.Getenv("DRY_RUN") != ""

	var sink vectorSink = pgSink{pool: pool}
	if dryRun {
		sink = noopSink{}
		log.Print("backfill-semantic-vectors: DRY RUN — reading Meili, writing nothing")
	}

	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	st, err := seeder{
		src:      meiliSource{client: client},
		sink:     sink,
		pageSize: pageSize,
	}.run(ctx)
	if err != nil {
		log.Printf("backfill-semantic-vectors: %v (pages=%d fetched=%d saved=%d)", err, st.Pages, st.Fetched, st.Saved)
		return 1
	}

	log.Printf("backfill-semantic-vectors done: pages=%d fetched=%d saved=%d dry_run=%v",
		st.Pages, st.Fetched, st.Saved, dryRun)
	return 0
}

// meiliSource is the read side: one Meili documents page per call.
type meiliSource struct{ client *search.Client }

func (m meiliSource) Page(ctx context.Context, offset, limit int) ([]search.SemanticVector, error) {
	return m.client.ListSemanticVectors(ctx, offset, limit)
}

// pgSink writes a page of vectors into jobs.semantic_embedding in one batched round
// trip. The UPDATE is by primary key and unconditional, so it is idempotent and a
// vector for a since-deleted job simply affects zero rows.
type pgSink struct{ pool *pgxpool.Pool }

func (s pgSink) Save(ctx context.Context, vecs []search.SemanticVector) (int64, error) {
	batch := &pgx.Batch{}
	for _, v := range vecs {
		batch.Queue(`UPDATE jobs SET semantic_embedding = $1 WHERE id = $2`, v.Vector, v.ID)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	var affected int64
	for range vecs {
		ct, err := br.Exec()
		if err != nil {
			return affected, err
		}
		affected += ct.RowsAffected()
	}
	return affected, nil
}

// noopSink backs --dry-run: it reports the count it would have written but touches
// nothing.
type noopSink struct{}

func (noopSink) Save(_ context.Context, vecs []search.SemanticVector) (int64, error) {
	return int64(len(vecs)), nil
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
