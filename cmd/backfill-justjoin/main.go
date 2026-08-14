// Command backfill-justjoin fills the description of justjoin jobs ingested before per-offer
// detail hydration existed. The justjoin adapter originally read only the list endpoint, which
// omits the posting body, so those rows were stored with an empty description (a blank, broken
// job page). This one-off worker pages every source='justjoin' row, re-fetches the posting's
// detail (GET /v1/offers/{slug}) via the shared HTTP client, and rewrites the description plus a
// refreshed content_hash so the row re-indexes. Follow it with `make reindex` (and
// cmd/backfill-derive to re-derive skills/facets from the new descriptions).
//
// It pages by keyset and exits. Idempotent: a row whose stored description already equals the
// fetched one is skipped, so a second run (e.g. after a rate-limit interruption) rewrites only
// what a prior run missed. A single posting whose detail fetch fails is counted and skipped,
// never aborting the run.
package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/backfillpage"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// jobStore is the slice of the data layer the backfill needs: page one provider's rows by keyset
// and rewrite a row's description + content_hash. *db.Queries satisfies it; the test uses a fake.
type jobStore interface {
	ListJobsBySourceAfter(ctx context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error)
	UpdateJobDescription(ctx context.Context, arg db.UpdateJobDescriptionParams) (int64, error)
}

// descriptionFetcher fetches the sanitized description for a stored job URL, returning ok=false
// when the detail is unavailable (bad URL, failed request, or empty body).
type descriptionFetcher func(ctx context.Context, jobURL string) (string, bool)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	client := sources.NewClient()
	fetch := func(ctx context.Context, jobURL string) (string, bool) {
		return sources.JustJoinDescription(ctx, client, jobURL)
	}

	scanned, updated, failed, err := backfillAll(ctx, db.New(pool), fetch)
	if err != nil {
		log.Printf("backfill-justjoin: %v", err)
		return 1
	}
	log.Printf("backfill-justjoin done: scanned=%d updated=%d failed=%d", scanned, updated, failed)
	return 0
}

// backfillAll pages every justjoin row and rewrites the description of rows whose fetched detail
// body differs from what is stored. The keyset walk itself (id > last seen, so concurrent writes
// cannot skip or repeat rows) is shared with the sibling source backfills via backfillpage.Rows.
// A row whose detail fetch fails is counted (failed) and skipped.
func backfillAll(ctx context.Context, store jobStore, fetch descriptionFetcher) (scanned, updated, failed int, err error) {
	err = backfillpage.Rows(ctx, store.ListJobsBySourceAfter, "justjoin", func(j db.Job) error {
		scanned++
		desc, ok := fetch(ctx, j.URL)
		if !ok {
			failed++
			return nil
		}
		if desc == j.Description {
			return nil // already current — idempotent skip
		}
		hash := jobhash.OfRow(j, desc)
		if _, err := store.UpdateJobDescription(ctx, db.UpdateJobDescriptionParams{
			ID:          j.ID,
			Description: desc,
			ContentHash: pgtype.Text{String: hash, Valid: true},
		}); err != nil {
			return err
		}
		updated++
		return nil
	})
	return scanned, updated, failed, err
}
