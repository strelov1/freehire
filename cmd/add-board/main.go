// Command add-board is how a curator adds or retires a board catalog row by hand — the
// database-backed replacement for editing sources/*.yml directly.
//
// It reports by default and writes only under --apply, like cmd/merge-companies: the
// candidate is validated and printed before anything is persisted. A curator-added board
// starts at status='active' (a curator addition is already verified, so there is no
// unproven pending period to model, unlike a crowdsourced contribution). Retiring a board
// sets its status to 'retired' rather than deleting the row, preserving history.
//
// A crowdsourced board is seeded with a company placeholder derived from the board slug
// (boardcatalog.PlaceholderCompany) — a URL-only submission never carries the real
// display name. --rename corrects it once a curator, reviewing the pending list, knows
// the real one.
//
// Usage:
//
//	go run ./cmd/add-board --provider=greenhouse --board=acme --company="Acme" [--apply]
//	go run ./cmd/add-board --retire --provider=greenhouse --board=acme [--apply]
//	go run ./cmd/add-board --rename --provider=greenhouse --board=acme --company="Acme, Inc." [--apply]
package main

import (
	"context"
	"errors"
	"flag"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually write; without it the run only reports")
	retire := flag.Bool("retire", false, "retire an existing board instead of adding one")
	rename := flag.Bool("rename", false, "correct an existing board's company name instead of adding one")
	provider := flag.String("provider", "", "provider key (required)")
	board := flag.String("board", "", "board id (required unless the provider is boardless)")
	region := flag.String("region", "", "region, for a provider with regional hosts (optional)")
	company := flag.String("company", "", "company name (required when adding)")
	hub := flag.Bool("hub", false, "mark the board a community/agency hub (adding only)")
	tenantsFlag := flag.String("tenants", "", "comma-separated key:value pairs (adding only)")
	flag.Parse()

	if *provider == "" {
		log.Print("add-board: --provider is required")
		return 1
	}

	switch {
	case *retire:
		return runRetire(*provider, *board, *region, *apply)
	case *rename:
		if *company == "" {
			log.Print("add-board: --company is required with --rename")
			return 1
		}
		return runRename(*provider, *board, *region, *company, *apply)
	default:
		return runAdd(*provider, *board, *region, *company, *hub, *tenantsFlag, *apply)
	}
}

// withDB opens a real database connection (worker.Bootstrap, i.e. DATABASE_URL) and
// hands it to action, closing the connection afterward. The one thing runAdd, runRetire,
// and runRename each do once they decide to actually write.
func withDB(action func(ctx context.Context, pool *pgxpool.Pool) int) int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	return action(ctx, pool)
}

// runAdd is the CLI entry point for adding a board: it reports the candidate, and under
// apply delegates to addBoard over a real database connection. Split from addBoard so a
// test can exercise addBoard directly against a throwaway database instead of the
// environment's real one.
func runAdd(provider, board, region, company string, hub bool, tenantsFlag string, apply bool) int {
	if company == "" {
		log.Print("add-board: --company is required when adding")
		return 1
	}
	tenants, err := parseTenants(tenantsFlag)
	if err != nil {
		log.Printf("add-board: %v", err)
		return 1
	}
	in := boardcatalog.InsertInput{Provider: provider, Board: board, Region: region, Company: company, Hub: hub, Tenants: tenants}

	registry := sources.Taxonomy()
	if err := boardcatalog.Validate(in, registry); err != nil {
		log.Printf("add-board: invalid: %v", err)
		return 1
	}
	log.Printf("add-board: would add provider=%s board=%s region=%q company=%q status=active",
		provider, board, region, company)
	if !apply {
		log.Print("add-board: dry run, nothing written. Re-run with --apply to add.")
		return 0
	}
	return withDB(func(ctx context.Context, pool *pgxpool.Pool) int { return addBoard(ctx, pool, in, registry) })
}

func addBoard(ctx context.Context, pool *pgxpool.Pool, in boardcatalog.InsertInput, registry map[string]sources.Source) int {
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	b, err := boardcatalog.Insert(ctx, repo, in, boardcatalog.StatusActive, registry)
	if err != nil {
		if errors.Is(err, boardcatalog.ErrDuplicateBoard) {
			log.Printf("add-board: %s/%s already exists (pending or active) — nothing written", in.Provider, in.Board)
			return 1
		}
		log.Printf("add-board: insert: %v", err)
		return 1
	}
	log.Printf("add-board: added id=%d provider=%s board=%s status=%s", b.ID, b.Provider, b.Board, b.Status)
	return 0
}

// runRetire mirrors runAdd's split: it reports, then under apply delegates to
// retireBoard over a real database connection.
func runRetire(provider, board, region string, apply bool) int {
	log.Printf("add-board: would retire provider=%s board=%s region=%q", provider, board, region)
	if !apply {
		log.Print("add-board: dry run, nothing written. Re-run with --apply to retire.")
		return 0
	}
	return withDB(func(ctx context.Context, pool *pgxpool.Pool) int {
		return retireBoard(ctx, pool, provider, board, region)
	})
}

// runRename mirrors runAdd/runRetire's split: it reports, then under apply delegates to
// renameBoard over a real database connection.
func runRename(provider, board, region, company string, apply bool) int {
	log.Printf("add-board: would rename provider=%s board=%s region=%q to company=%q", provider, board, region, company)
	if !apply {
		log.Print("add-board: dry run, nothing written. Re-run with --apply to rename.")
		return 0
	}
	return withDB(func(ctx context.Context, pool *pgxpool.Pool) int {
		return renameBoard(ctx, pool, provider, board, region, company)
	})
}

func renameBoard(ctx context.Context, pool *pgxpool.Pool, provider, board, region, company string) int {
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	found, err := repo.Rename(ctx, provider, board, region, company)
	if err != nil {
		log.Printf("add-board: rename: %v", err)
		return 1
	}
	if !found {
		log.Printf("add-board: no live (pending/active) board matches provider=%s board=%s region=%q — nothing renamed",
			provider, board, region)
		return 1
	}
	log.Printf("add-board: renamed provider=%s board=%s region=%q to company=%q", provider, board, region, company)
	return 0
}

func retireBoard(ctx context.Context, pool *pgxpool.Pool, provider, board, region string) int {
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	found, err := repo.Retire(ctx, provider, board, region)
	if err != nil {
		log.Printf("add-board: retire: %v", err)
		return 1
	}
	if !found {
		log.Printf("add-board: no live (pending/active) board matches provider=%s board=%s region=%q — nothing retired",
			provider, board, region)
		return 1
	}
	log.Printf("add-board: retired provider=%s board=%s region=%q", provider, board, region)
	return 0
}
