// Command backfill-echojobs fills the description of echojobs jobs whose original ingest never
// hydrated one — either from the pre-hydration era (the adapter originally read only the list
// endpoint, which omitted the posting body entirely) or from echojobs.io retiring its public
// JSON API around 2026-08-13 (see internal/sources/echojobs.go), which left every newly-
// discovered posting from that point with an empty description until the adapter was rewritten.
// Either way the result is the same: a blank, broken job page, reported live on freehire.me.
// This one-off worker pages every source='echojobs' row, re-fetches the posting's detail (GET
// /job/{slug} + its embedded schema.org JobPosting ld+json — see sources.EchoJobsDescription)
// via the shared HTTP client, and rewrites the description plus a refreshed content_hash so the
// row re-indexes. Follow it with `make reindex` (and cmd/backfill-derive to re-derive
// skills/facets from the new descriptions).
//
// echojobs is boardless, so its stored external_id carries the empty-board namespace prefix
// (NamespaceExternalID("", jobHandle) == ":jobHandle") — jobHandle strips that colon before
// calling the detail endpoint, which is keyed by the raw slug.
//
// It pages by keyset and exits. Idempotent: a row whose stored description already equals the
// fetched one is skipped, so a second run (e.g. after a rate-limit interruption) rewrites only
// what a prior run missed. A single posting whose detail fetch fails is counted and skipped,
// never aborting the run.
package main

import (
	"context"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// backfillBatchSize bounds how many rows are read per keyset page.
const backfillBatchSize = 500

// jobStore is the slice of the data layer the backfill needs: page one provider's rows by keyset
// and rewrite a row's description + content_hash. *db.Queries satisfies it; the test uses a fake.
type jobStore interface {
	ListJobsBySourceAfter(ctx context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error)
	UpdateJobDescription(ctx context.Context, arg db.UpdateJobDescriptionParams) (int64, error)
}

// descriptionFetcher fetches the sanitized description for a stored job's job_handle, returning
// ok=false when the detail is unavailable (failed request or empty body).
type descriptionFetcher func(ctx context.Context, jobHandle string) (string, bool)

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
	fetch := func(ctx context.Context, jobHandle string) (string, bool) {
		return sources.EchoJobsDescription(ctx, client, jobHandle)
	}

	scanned, updated, failed, err := backfillAll(ctx, db.New(pool), fetch)
	if err != nil {
		log.Printf("backfill-echojobs: %v", err)
		return 1
	}
	log.Printf("backfill-echojobs done: scanned=%d updated=%d failed=%d", scanned, updated, failed)
	return 0
}

// jobHandle strips echojobs' empty-board namespace prefix from a stored external_id, recovering
// the raw job_handle the detail endpoint is keyed by.
func jobHandle(externalID string) string {
	return strings.TrimPrefix(externalID, ":")
}

// backfillAll pages every echojobs row and rewrites the description of rows whose fetched detail
// body differs from what is stored. It pages by keyset (id > last seen) so concurrent writes
// cannot skip or repeat rows. A row whose detail fetch fails is counted (failed) and skipped.
func backfillAll(ctx context.Context, store jobStore, fetch descriptionFetcher) (scanned, updated, failed int, err error) {
	var afterID int64
	for {
		jobs, err := store.ListJobsBySourceAfter(ctx, db.ListJobsBySourceAfterParams{
			Source:    "echojobs",
			AfterID:   afterID,
			BatchSize: backfillBatchSize,
		})
		if err != nil {
			return scanned, updated, failed, err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		for _, j := range jobs {
			scanned++
			desc, ok := fetch(ctx, jobHandle(j.ExternalID))
			if !ok {
				failed++
				continue
			}
			if desc == j.Description {
				continue // already current — idempotent skip
			}
			hash := jobhash.OfRow(j, desc)
			if _, err := store.UpdateJobDescription(ctx, db.UpdateJobDescriptionParams{
				ID:          j.ID,
				Description: desc,
				ContentHash: pgtype.Text{String: hash, Valid: true},
			}); err != nil {
				return scanned, updated, failed, err
			}
			updated++
		}

		if len(jobs) < backfillBatchSize {
			break
		}
	}
	return scanned, updated, failed, nil
}
