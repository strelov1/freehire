// Command backfill-ess-urls repairs the query-only ESS links stored before #4003.
// Ordinary crawls only refresh seen postings' liveness, so they cannot repair these
// URLs. The adapter's 1,200-posting hydration cap also makes a full refetch unsuitable.
//
// Reports by default; --apply writes. Walks only ESS by keyset, including closed
// postings that a later crawl may reopen. Already-corrected rows are skipped, so
// re-running safely resumes an interrupted repair. Each update atomically refreshes
// content_hash and queues an open posting for search-drain; no full reindex is needed.
// Needs only DATABASE_URL. Run after deploying the fixed adapter.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/backfillpage"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

type jobStore interface {
	ListJobsBySourceAfter(context.Context, db.ListJobsBySourceAfterParams) ([]db.Job, error)
	RepairESSJobURL(context.Context, db.RepairESSJobURLParams) (db.RepairESSJobURLRow, error)
}

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "write the corrections (default: report only)")
	flag.Parse()
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	scanned, changed, written, err := backfillAll(ctx, db.New(pool), *apply)
	if err != nil {
		log.Printf(
			"backfill-ess-urls: scanned=%d changed=%d written=%d: %v; resume by re-running",
			scanned,
			changed,
			written,
			err,
		)
		return 1
	}
	log.Printf(
		"backfill-ess-urls: done, apply=%t scanned=%d would_change=%d written=%d",
		*apply,
		scanned,
		changed,
		written,
	)
	return 0
}

func backfillAll(ctx context.Context, store jobStore, apply bool) (scanned, changed, written int64, err error) {
	err = backfillpage.Rows(ctx, store.ListJobsBySourceAfter, "ess", func(j db.Job) error {
		scanned++
		url := repairedURL(j)
		if url == j.URL {
			return nil
		}
		changed++
		if !apply {
			return nil
		}
		oldURL := j.URL
		j.URL = url
		result, err := store.RepairESSJobURL(ctx, db.RepairESSJobURLParams{
			ID:             j.ID,
			URL:            url,
			ContentHash:    pgtype.Text{String: jobhash.OfRow(j, j.Description), Valid: true},
			OldUrl:         oldURL,
			OldContentHash: j.ContentHash,
		})
		if err != nil {
			return fmt.Errorf("repair job %d: %w", j.ID, err)
		}
		if result.UpdatedCount != 1 {
			return fmt.Errorf("job %d changed concurrently; re-run to read its current content", j.ID)
		}
		written++
		return nil
	})
	return scanned, changed, written, err
}

func repairedURL(j db.Job) string {
	if j.Source != "ess" || j.IsPrivate {
		return j.URL
	}
	id := strings.TrimPrefix(j.ExternalID, ":")
	if n, err := strconv.ParseUint(id, 10, 64); err != nil || n == 0 {
		return j.URL
	}
	const legacyPrefix = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp="
	if j.URL != legacyPrefix+id {
		return j.URL
	}
	return j.URL + "/#/pdm/" + id
}
