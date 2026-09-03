// Command backfill-board-catalog is the one-off worker that seeds the boards table from
// the current sources/*.yml catalog, at status='active' — every one of them is already
// crawling successfully in prod, so none of them should re-enter the unproven pending
// window (see docs/superpowers/specs/2026-09-03-board-catalog-in-db-design.md).
//
// It is the last piece of code in the repository allowed to parse sources/*.yml, and is
// deleted once it has run once in prod and cmd/ingest has cut over to reading boards
// from the database instead.
//
// Idempotent: a re-run's insert of an already-backfilled identity collides with the row
// the first run created (boards_identity_key, a pending/active row) and is counted as
// already-present rather than an error, so stopping and re-running the backfill is free.
//
// Usage:
//
//	go run ./cmd/backfill-board-catalog           # reads sources/
//	go run ./cmd/backfill-board-catalog sources    # same, explicit
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// notBoardFiles are files under sources/ that do not hold a CompanyEntry list —
// mirrors cmd/validate-sources' own exclusion.
var notBoardFiles = map[string]bool{"telegram.yml": true}

func main() { worker.Main(run) }

func run() int {
	dir := "sources"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		log.Printf("backfill-board-catalog: scan %s: %v", dir, err)
		return 1
	}
	if len(paths) == 0 {
		log.Printf("backfill-board-catalog: no *.yml files under %s", dir)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	registry := sources.Taxonomy()

	var inserted, alreadyPresent, failed int
	for _, path := range paths {
		if notBoardFiles[filepath.Base(path)] {
			continue
		}
		n, present, f := backfillFile(ctx, repo, registry, path)
		inserted += n
		alreadyPresent += present
		failed += f
	}
	log.Printf("backfill-board-catalog: inserted=%d already_present=%d failed=%d",
		inserted, alreadyPresent, failed)
	return worker.ExitCode(failed, 0)
}

func backfillFile(ctx context.Context, repo boardcatalog.Repository, registry map[string]sources.Source, path string) (inserted, alreadyPresent, failed int) {
	cfg, err := sources.LoadConfig(path)
	if err != nil {
		log.Printf("backfill-board-catalog: %s: %v", path, err)
		return 0, 0, 1
	}
	for _, e := range cfg.Sources {
		in := boardcatalog.InsertInput{
			Provider: e.Provider,
			Board:    e.Board,
			Region:   e.Region,
			Company:  e.Company,
			Hub:      e.Hub,
			Tenants:  e.Tenants,
		}
		b, err := boardcatalog.Insert(ctx, repo, in, boardcatalog.StatusActive, registry)
		switch {
		case errors.Is(err, boardcatalog.ErrDuplicateBoard):
			alreadyPresent++
		case err != nil:
			log.Printf("backfill-board-catalog: %s: insert %s/%s: %v", path, e.Provider, e.Board, err)
			failed++
		case b.Status == boardcatalog.StatusRejected:
			log.Printf("backfill-board-catalog: %s: %s/%s stored as rejected: %s", path, e.Provider, e.Board, b.RejectedReason)
			failed++
		default:
			inserted++
		}
	}
	return inserted, alreadyPresent, failed
}
