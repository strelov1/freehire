// Command backfill-blank-company fills jobs.company for postings whose provider sets Company
// statically per board (sources/<provider>.yml), but landed empty because of a since-fixed
// ingest bug — e.g. #1699, where ukg.go never set Job.Company at all. Unlike
// cmd/backfill-company-names (which resolves a REAL name over the network for a squished-slug
// company), this needs no network call: the correct name already sits in the board file the
// affected job was ingested from, so this is a pure config-to-database backfill.
//
// A re-crawl of a still-live board would have self-healed via UpsertJob's ON CONFLICT, so what
// survives here is postings that closed or dropped off their board's listing before the ingest
// fix shipped and were never seen again.
//
//	backfill-blank-company [--dry-run]   # needs DATABASE_URL
//
// The provider list below is a hand-verified allowlist, not "every non-aggregator source": a
// provider belongs here only if its adapter sets Job.Company = e.Company UNCONDITIONALLY for
// every posting on a board (confirmed by reading the adapter). A "hub" adapter that reads the
// company per posting from the platform itself (e.g. recruitingsolutions, huntflow) must never
// be added — the board's configured company is not necessarily that posting's real employer,
// so blindly applying it would introduce wrong data instead of fixing missing data.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// staticCompanyProviders is the hand-verified allowlist described in the package doc.
var staticCompanyProviders = []string{"ukg", "teamtailor", "workday"}

func main() { worker.Main(run) }

func run() int {
	dryRun := flag.Bool("dry-run", false, "count rows that would be updated, without writing")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)

	var boards, updated, enqueued, failed int
	for _, provider := range staticCompanyProviders {
		cfg, err := sources.LoadConfig("sources/" + provider + ".yml")
		if err != nil {
			log.Printf("load %s: %v", provider, err)
			failed++
			continue
		}
		for _, e := range cfg.Sources {
			if e.Company == "" || e.Board == "" {
				continue // Validate already guarantees this in production; defensive only
			}
			pattern := sources.BoardIDPattern(e.Board)
			boards++

			if *dryRun {
				n, err := queries.CountBlankCompanyByBoard(ctx, db.CountBlankCompanyByBoardParams{
					Source:       e.Provider,
					BoardPattern: pattern,
				})
				if err != nil {
					log.Printf("count %s/%s: %v", e.Provider, e.Board, err)
					failed++
					continue
				}
				updated += int(n)
				continue
			}

			row, err := queries.BackfillBoardCompany(ctx, db.BackfillBoardCompanyParams{
				Company:      e.Company,
				CompanySlug:  normalize.Slug(e.Company),
				Source:       e.Provider,
				BoardPattern: pattern,
			})
			if err != nil {
				log.Printf("backfill %s/%s: %v", e.Provider, e.Board, err)
				failed++
				continue
			}
			updated += int(row.UpdatedCount)
			enqueued += int(row.EnqueuedCount)
		}
	}

	if *dryRun {
		log.Printf("backfill-blank-company dry-run: boards=%d would_update=%d failed=%d", boards, updated, failed)
		return 0
	}

	// jobs now carry a real company/company_slug where they had none; the derived catalogue
	// needs a matching companies row for /companies/<slug> to resolve.
	if updated > 0 {
		if err := queries.SyncCompaniesFromJobs(ctx); err != nil {
			log.Printf("sync companies: %v", err)
			return 1
		}
	}

	log.Printf("backfill-blank-company done: boards=%d updated=%d enqueued_for_search=%d failed=%d",
		boards, updated, enqueued, failed)
	return worker.ExitCode(failed, 0)
}
