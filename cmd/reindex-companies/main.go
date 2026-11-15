// Command reindex-companies rebuilds the Meilisearch companies index from Postgres. It streams
// every hiring company (job_count > 0) into a fresh index and atomically swaps it in (see
// search.CompanyRebuild), then exits. Run it on a schedule, never stacked with the jobs reindex:
// Meilisearch has one serial task queue (the second rebuild looks like a hang) and a swap
// transiently holds about twice the index's disk. It never touches the jobs index.
//
// The live index keeps serving until the swap, and re-runs are safe (the rebuild index starts empty).
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// reindexBatchSize bounds how many companies are read and pushed per keyset round.
const reindexBatchSize = 2000

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Bootstrap owns config and pool, so this check runs after the pool opens.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	// Shares Meilisearch's single task queue with the jobs rebuild; the two must never overlap.
	releaseLock, gotLock, err := worker.HoldHeavyIndexLock(ctx, pool)
	if err != nil {
		log.Printf("index lock: %v", err)
		return 1
	}
	if !gotLock {
		log.Print("reindex-companies: another catalogue-wide index job is running — skipping this run")
		return 0
	}
	defer releaseLock()

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

// companyReader pages hiring companies by keyset; a fake stands in for it in tests.
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

// companyRebuilder is the subset of search.CompanyRebuild the reindex drives; Promote swaps atomically.
type companyRebuilder interface {
	Prepare(ctx context.Context) error
	Push(ctx context.Context, docs []search.CompanyDocument) error
	Promote(ctx context.Context) error
	// Cleanup drops a half-built rebuild index left by an aborted run.
	Cleanup(ctx context.Context) error
}

// reindexCompanies streams every hiring company into a fresh index and swaps it in.
// It pages by keyset (slug > last seen) so rows inserted mid-run are neither skipped nor
// repeated; a short page ends the scan. Prepare/Promote bracket the stream so an emptied
// catalogue still swaps in a clean, empty index.
func reindexCompanies(ctx context.Context, r companyReader, b companyRebuilder, batchSize int32) (int, error) {
	if err := b.Prepare(ctx); err != nil {
		return 0, err
	}

	// Any return before Promote leaves a half-built index; drop it in a defer (as the jobs
	// reindexFull does). Best-effort, on a cancellation-immune context so it still reaches
	// Meilisearch when the parent ctx was cancelled.
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
