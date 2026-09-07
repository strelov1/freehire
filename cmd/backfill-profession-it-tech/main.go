// Command backfill-profession-it-tech is a one-off: sets is_tech = true on the
// Profession itdev/itops rows stored before that source began asserting the signal at
// ingest (issue #2601, see internal/ingest/sources/profession.go's IsTechHint). Without
// it, only newly-crawled or re-crawled postings from these two boards get the
// correction — the rows already in the catalogue stay stuck with is_tech unknown,
// never enter the enrichment queue, and stay excluded from search
// (search.CategoryUnresolved).
//
// A single UPDATE, not chunked: the affected set is bounded to two boards (low
// thousands at most), unlike the multi-million-row backfills that need an id-range
// walk. Idempotent — the underlying query's IS DISTINCT FROM guard means re-running
// this costs nothing once every matching row is already true.
//
// Follow this with a full `make reindex`: is_tech is not part of content_hash, so an
// incremental search-drain push alone would never reach these pre-existing rows — the
// same gap cmd/backfill-clearance and cmd/backfill-company-type-hint document.
//
// Needs DATABASE_URL.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	boards := sources.ProfessionITBoardNames()
	patterns := make([]string, len(boards))
	for i, board := range boards {
		patterns[i] = externalid.BoardPattern(board)
	}

	n, err := db.New(pool).BackfillProfessionITBoardTech(ctx, patterns)
	if err != nil {
		log.Printf("backfill-profession-it-tech: %v", err)
		return 1
	}
	log.Printf("backfill-profession-it-tech: set is_tech = true on %d rows across boards %v", n, boards)
	return 0
}
