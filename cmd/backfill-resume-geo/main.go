// Command backfill-resume-geo re-derives the candidate geography (resume_countries /
// resume_regions / resume_cities) from the structured résumé already stored for each
// user. It is the reconciler the derivation needs so that a change to the location
// dictionary reaches existing candidates — the same role cmd/backfill-derive plays for
// jobs, and the one thing the structured résumé itself still lacks.
//
// It needs ONLY DATABASE_URL. Unlike cmd/backfill-resume-structured — which reads the CV
// out of object storage, de-identifies it, and spends an LLM call per user — this worker
// re-reads a structure that is already persisted and runs it through a deterministic
// dictionary. That is what makes it cheap enough to re-run after ANY dictionary edit.
//
// Idempotent by construction: it derives through resume.DeriveGeography, the same
// projection the upload path uses, so a second run over unchanged data and an unchanged
// dictionary writes identical values. It only visits users whose structured résumé still
// describes their stored CV — deriving geography from a superseded structure would route
// around the staleness rule that governs the structure itself — and every write carries
// the same monotonic guard, so a CV replaced mid-run is left to the upload path.
//
//	go run ./cmd/backfill-resume-geo                # every eligible user
//	go run ./cmd/backfill-resume-geo --user 291     # a single user
//	go run ./cmd/backfill-resume-geo --dry-run      # report, don't persist
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/database"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
)

func main() {
	var userID int64
	var dryRun bool
	flag.Int64Var(&userID, "user", 0, "backfill a single user id (0 = every eligible user)")
	flag.BoolVar(&dryRun, "dry-run", false, "report what would be written without persisting")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	rows, err := queries.ListUsersForResumeGeoBackfill(ctx, userID)
	if err != nil {
		log.Fatalf("list eligible users: %v", err)
	}
	log.Printf("backfill-resume-geo: %d user(s) with a current structured résumé%s", len(rows), dryRunSuffix(dryRun))

	var resolved, unresolved, unstated, failed int
	for _, row := range rows {
		countries, regions, cities := resume.DeriveGeography(row.Location)

		switch {
		case countries == nil && regions == nil:
			// The CV states no location: the answer is "not known", stored as NULL.
			unstated++
		case len(countries) == 0 && len(regions) == 0:
			// The CV states a place the curated dictionary could not resolve. Logged
			// individually because this set IS the dictionary's coverage gap, and the
			// only way to see it is to look at the strings that landed here.
			unresolved++
			log.Printf("user %d: unresolved location %q", row.ID, row.Location)
		default:
			resolved++
		}

		if dryRun {
			continue
		}
		if err := queries.SetUserResumeGeography(ctx, db.SetUserResumeGeographyParams{
			ID:               row.ID,
			ResumeCountries:  countries,
			ResumeRegions:    regions,
			ResumeCities:     cities,
			ResumeUploadedAt: row.ResumeUploadedAt,
		}); err != nil {
			log.Printf("user %d: persist: %v", row.ID, err)
			failed++
		}
	}

	log.Printf("backfill-resume-geo: done — %d resolved, %d stated but unresolved, %d stating no location, %d failed",
		resolved, unresolved, unstated, failed)
}

func dryRunSuffix(dry bool) string {
	if dry {
		return " (dry-run)"
	}
	return ""
}
