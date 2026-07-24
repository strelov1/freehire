// Command reindex-companies rebuilds the Meilisearch companies index from Postgres.
// It streams every hiring company (job_count > 0) into a fresh index and atomically
// swaps it in (see search.CompanyRebuild), then exits — run it on a schedule (cron),
// on its own flock, and never stacked with the jobs reindex on the same host (a swap
// transiently holds both the old and new index, ~2x that index's disk). Building this
// index never touches the jobs index, so jobs search cannot regress.
//
// Indexing is a full swap-rebuild: the live index keeps serving until the single
// atomic swap, and re-runs are safe (the rebuild index always starts empty).
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/worker"
)

// reindexBatchSize bounds how many companies are read from Postgres and pushed to
// Meilisearch per keyset round. Companies are a small, slow-moving directory, so a
// modest batch amortizes the round-trip without holding much in memory.
const reindexBatchSize = 2000

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Bootstrap owns config + pool, so the required-config check lands just after the
	// pool opens; the connect is cheap and cleanup closes it on this early return.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	reader := &queriesReader{q: db.New(pool)}

	indexed, err := reindexCompanies(ctx, reader, client.NewCompanyRebuild(), reindexBatchSize)
	if err != nil {
		log.Printf("reindex-companies: %v", err)
		return 1
	}
	log.Printf("reindex-companies done: indexed=%d", indexed)
	return 0
}

// companyReader pages hiring companies by keyset, so reindexCompanies' scan loop is
// unit-testable with a fake in place of the Postgres-backed queriesReader.
type companyReader interface {
	Page(ctx context.Context, afterSlug string, limit int32) ([]db.Company, error)
}

// queriesReader adapts *db.Queries to companyReader via ListCompaniesForReindex.
type queriesReader struct{ q *db.Queries }

func (r *queriesReader) Page(ctx context.Context, afterSlug string, limit int32) ([]db.Company, error) {
	return r.q.ListCompaniesForReindex(ctx, db.ListCompaniesForReindexParams{
		AfterSlug: afterSlug,
		BatchSize: limit,
	})
}

// companyRebuilder is the subset of search.CompanyRebuild the reindex drives — a
// fresh-index build session that swaps into production atomically on Promote.
type companyRebuilder interface {
	Prepare(ctx context.Context) error
	Push(ctx context.Context, docs []search.CompanyDocument) error
	Promote(ctx context.Context) error
	// Cleanup drops a half-built rebuild index. reindexCompanies defers it so a run
	// that aborts before Promote's swap-and-drop does not leave an orphan index.
	Cleanup(ctx context.Context) error
}

// reindexCompanies streams every hiring company into a fresh index and swaps it in.
// It pages by keyset (slug > last seen), so rows inserted or re-ordered mid-run are
// neither skipped nor repeated, mapping each row through search.FromCompany. A short
// page (fewer than batchSize) ends the scan. Prepare/Promote bracket the stream so an
// emptied catalogue still swaps in a clean, empty index rather than leaving a stale one.
func reindexCompanies(ctx context.Context, r companyReader, b companyRebuilder, batchSize int32) (int, error) {
	if err := b.Prepare(ctx); err != nil {
		return 0, err
	}

	// Promote ends the happy path by swapping the rebuild index in and dropping the old
	// one; any earlier return is an abort that leaves the half-built rebuild index
	// behind. Drop it in a defer (mirrors the jobs reindexFull) so an aborted run never
	// orphans an index that eats disk until the next run's Prepare clears it.
	// Best-effort on a cancellation-immune context so it can still reach Meilisearch
	// when the abort was the parent ctx being cancelled.
	promoted := false
	defer func() {
		if promoted {
			return
		}
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := b.Cleanup(cctx); err != nil {
			log.Printf("reindex-companies: cleanup aborted rebuild index (best-effort): %v", err)
		}
	}()

	var indexed int
	afterSlug := ""
	for {
		rows, err := r.Page(ctx, afterSlug, batchSize)
		if err != nil {
			return indexed, err
		}
		if len(rows) == 0 {
			break
		}
		docs := make([]search.CompanyDocument, len(rows))
		for i, row := range rows {
			docs[i] = search.FromCompany(row)
		}
		if err := b.Push(ctx, docs); err != nil {
			return indexed, err
		}
		indexed += len(docs)
		afterSlug = rows[len(rows)-1].Slug
		if int32(len(rows)) < batchSize {
			break
		}
	}
	if err := b.Promote(ctx); err != nil {
		return indexed, err
	}
	promoted = true
	return indexed, nil
}
