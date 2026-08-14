// Command backfill-himalayas-companyname repairs jobs.company for himalayas rows ingested
// while the adapter stored Himalayas' companyName sentinel verbatim (see
// sources.HimalayasCompanyNameSentinel — Himalayas' own feed renders companyName as the
// literal string "name" for a subset of postings, an unresolved template field on their end).
//
// The adapter now falls back to the feed's companySlug field, but that only reaches new or
// re-crawled rows: himalayas is boardless with a per-run page budget far short of its full
// catalogue (recency-ordered, so old rows fall out of the crawled window and are never
// revisited by a normal ingest). This backfill instead recovers the company slug from each
// row's own stored url, which Himalayas' canonical job page always carries at
// /companies/<slug>/jobs/... — the same signal, just read back from the DB instead of the
// live feed (companySlug itself isn't stored anywhere).
//
// It pages every source='himalayas' row, and for each one still carrying the sentinel,
// extracts the slug and rewrites company/company_slug plus a refreshed content_hash. A row
// whose url doesn't match the expected shape is counted (unresolved) and left alone rather
// than aborting the run. Idempotent: a second run finds no sentinel rows left to fix.
package main

import (
	"context"
	"log"
	"regexp"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/backfillpage"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// companySlugPath pulls the company slug out of a canonical Himalayas job URL
// (https://himalayas.app/companies/<slug>/jobs/...), same pattern the adapter's live fallback
// no longer needs (it reads companySlug straight off the feed) but a stored row only has url.
var companySlugPath = regexp.MustCompile(`himalayas\.app/companies/([^/]+)/jobs/`)

// jobStore is the slice of the data layer the backfill needs. *db.Queries satisfies it; the
// test uses a fake.
type jobStore interface {
	ListJobsBySourceAfter(ctx context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error)
	UpdateJobCompany(ctx context.Context, arg db.UpdateJobCompanyParams) (int64, error)
}

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

	total, updated, unresolved, err := backfillAll(ctx, db.New(pool))
	if err != nil {
		log.Printf("backfill-himalayas-companyname: %v", err)
		return 1
	}
	log.Printf("backfill-himalayas-companyname done: scanned=%d updated=%d unresolved=%d", total, updated, unresolved)
	return 0
}

// backfillAll pages every himalayas row and rewrites company/company_slug on rows still
// carrying HimalayasCompanyNameSentinel. The keyset walk itself (id > last seen, so concurrent
// writes cannot skip or repeat rows) is shared with the sibling source backfills via
// backfillpage.Rows. A sentinel row whose url yields no slug is counted (unresolved) and
// skipped, never aborting the run.
func backfillAll(ctx context.Context, store jobStore) (total, updated, unresolved int, err error) {
	err = backfillpage.Rows(ctx, store.ListJobsBySourceAfter, "himalayas", func(j db.Job) error {
		total++
		if j.Company != sources.HimalayasCompanyNameSentinel {
			return nil
		}
		m := companySlugPath.FindStringSubmatch(j.URL)
		if m == nil {
			unresolved++
			return nil
		}
		company := m[1]
		companySlug := normalize.Slug(company)

		row := j
		row.Company = company
		row.CompanySlug = companySlug
		hash := jobhash.OfRow(row, j.Description)

		if _, err := store.UpdateJobCompany(ctx, db.UpdateJobCompanyParams{
			ID:          j.ID,
			Company:     company,
			CompanySlug: companySlug,
			ContentHash: pgtype.Text{String: hash, Valid: true},
		}); err != nil {
			return err
		}
		updated++
		return nil
	})
	return total, updated, unresolved, err
}
