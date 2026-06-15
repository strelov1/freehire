// Command reslug backfills public_slug/company_slug after a deliberate change to
// the slug builder (see internal/normalize). Slugs are otherwise immutable, so a
// normalization change (e.g. ASCII transliteration of non-Latin names) only takes
// effect for new rows; this one-off worker recomputes every existing job's slug
// and rewrites the ones that changed. Run it once after deploying such a change;
// it pages the whole table and exits. Idempotent — a second run rewrites nothing.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/worker"
)

// reslugBatchSize bounds how many jobs are read per keyset page.
const reslugBatchSize = 500

func main() {
	os.Exit(run())
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)

	scanned, updated, err := reslugAll(ctx, queries)
	if err != nil {
		log.Printf("reslug: %v", err)
		return 1
	}

	// jobs.company_slug now carries the new slugs; re-key the companies catalogue
	// to match (and drop the rows orphaned by the change) so company pages resolve.
	if err := queries.SyncCompaniesFromJobs(ctx); err != nil {
		log.Printf("reslug: sync companies: %v", err)
		return 1
	}
	orphans, err := queries.DeleteOrphanCompanies(ctx)
	if err != nil {
		log.Printf("reslug: delete orphan companies: %v", err)
		return 1
	}

	log.Printf("reslug done: scanned=%d updated=%d companies_orphaned=%d", scanned, updated, orphans)
	return 0
}

// reslugAll recomputes every job's slug and rewrites the ones that differ. It
// pages by keyset (id > last seen) so concurrent writes cannot skip or repeat
// rows. The recomputed slug is a pure function of immutable fields, so two rows
// never collide on public_slug (the shortcode derives from the unique dedup key).
func reslugAll(ctx context.Context, q *db.Queries) (scanned, updated int, err error) {
	var afterID int64
	for {
		jobs, err := q.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{
			AfterID:   afterID,
			BatchSize: reslugBatchSize,
		})
		if err != nil {
			return scanned, updated, err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		for _, j := range jobs {
			scanned++
			publicSlug := normalize.JobSlug(j.Title, j.Company, j.Source, j.ExternalID)
			companySlug := normalize.Slug(j.Company)
			if publicSlug == j.PublicSlug && companySlug == j.CompanySlug {
				continue
			}
			if err := q.UpdateJobSlugs(ctx, db.UpdateJobSlugsParams{
				ID:          j.ID,
				PublicSlug:  publicSlug,
				CompanySlug: companySlug,
			}); err != nil {
				return scanned, updated, err
			}
			updated++
		}

		if len(jobs) < reslugBatchSize {
			break
		}
	}

	return scanned, updated, nil
}
