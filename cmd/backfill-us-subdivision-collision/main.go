// Command backfill-us-subdivision-collision re-derives the jobs that freehire#3113's
// fix in internal/dict/location/dictionaries.go can now resolve correctly: a US
// posting whose location's trailing field is the bare two-letter code "IN", "DE", or
// "ID" was read as India, Germany, or Indonesia (those codes collided with a curated
// country and were omitted from subdivisionToCountry outright) instead of Indiana,
// Delaware, or Idaho.
//
// cmd/backfill-derive is the general tool for this (any dictionary change needs a
// re-derive to reach existing rows, AGENTS.md), but a full pass over the whole ~12.7M-
// row catalogue costs 15-30h of careful, monitored host time
// (see hire-backfill-derive-prod-run-profile and friends) to fix what
// ListJobIDsWithUSSubdivisionCollision (internal/platform/db/queries/jobs.sql) measured
// at ~179k candidate rows — 1.5% of the table. This command re-derives only that
// candidate set, by primary-key lookup (GetJob), so the host never pays for the other
// 98.5% of rows the dictionary change cannot possibly affect.
//
// The candidate query is deliberately broad (it also matches genuine India/Germany/
// Indonesia postings written "City, CC") — the recompute below is what decides, and a
// row whose derived value doesn't change costs one skipped write. Idempotent for the
// same reason cmd/backfill-derive is: every derived column is a pure function of the
// row's own raw fields, so a second run rewrites nothing.
//
// Follow with a full `make reindex` — none of these columns are part of content_hash,
// so an incremental search-drain push would not reach rows already in the index
// (the same gap cmd/backfill-clearance documents).
//
// Needs DATABASE_URL.
package main

import (
	"context"
	"log"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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

	queries := db.New(pool)

	canon, err := loadAliasRegistry(ctx, queries)
	if err != nil {
		log.Printf("backfill-us-subdivision-collision: %v", err)
		return 1
	}

	ids, err := queries.ListJobIDsWithUSSubdivisionCollision(ctx)
	if err != nil {
		log.Printf("backfill-us-subdivision-collision: list candidates: %v", err)
		return 1
	}
	log.Printf("backfill-us-subdivision-collision: %d candidate rows", len(ids))

	var scanned, updated, slugsMoved int
	for _, id := range ids {
		j, err := queries.GetJob(ctx, id)
		if err != nil {
			log.Printf("backfill-us-subdivision-collision: get job %d: %v", id, err)
			continue
		}
		scanned++

		params, changed, slugMoved := deriveRow(j, canon)
		if !changed {
			continue
		}
		if err := queries.UpdateJobDerived(ctx, params); err != nil {
			log.Printf("backfill-us-subdivision-collision: update job %d: %v", id, err)
			continue
		}
		updated++
		if slugMoved {
			slugsMoved++
		}
		if scanned%10_000 == 0 {
			log.Printf("backfill-us-subdivision-collision: scanned %d, updated %d, slugs_moved %d",
				scanned, updated, slugsMoved)
		}
	}

	if slugsMoved > 0 {
		if err := queries.SyncCompaniesFromJobs(ctx); err != nil {
			log.Printf("backfill-us-subdivision-collision: sync companies: %v", err)
			return 1
		}
		orphaned, err := queries.DeleteOrphanCompanies(ctx)
		if err != nil {
			log.Printf("backfill-us-subdivision-collision: delete orphan companies: %v", err)
			return 1
		}
		log.Printf("backfill-us-subdivision-collision: companies_orphaned %d", orphaned)
	}

	log.Printf("backfill-us-subdivision-collision done: scanned=%d updated=%d slugs_moved=%d (follow with a reindex)",
		scanned, updated, slugsMoved)
	return 0
}

// loadAliasRegistry reads company_slug_aliases into a folded-key lookup, the same
// precaution cmd/backfill-derive takes: an empty registry looks exactly like a
// catalogue with no merges, and proceeding on a read failure would rewrite a merged
// posting back to its source spelling.
func loadAliasRegistry(ctx context.Context, queries *db.Queries) (map[string]string, error) {
	rows, err := queries.ListCompanySlugAliases(ctx)
	if err != nil {
		return nil, err
	}
	canon := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, seen := canon[r.FoldedKey]; !seen {
			canon[r.FoldedKey] = r.CanonicalSlug
		}
	}
	return canon, nil
}

// deriveRow is cmd/backfill-derive's deriveRow, unchanged: re-derive a job's facets,
// role_fingerprint, and slugs, and report whether the derived values differ from what
// is stored. Kept as a private copy rather than an import — cmd/backfill-derive does
// not export it — because this command is a narrow, one-time candidate-set variant of
// the same full-table tool, not a new permanent entry point worth building a shared
// package around.
func deriveRow(j db.Job, canon map[string]string) (params db.UpdateJobDerivedParams, changed, slugMoved bool) {
	d := jobderive.Derive(jobderive.Input{
		Title:       j.Title,
		Company:     j.Company,
		Source:      j.Source,
		ExternalID:  j.ExternalID,
		Location:    j.Location,
		Description: j.Description,
		WorkMode:    j.WorkMode,
		IsTechHint:  sources.ProfessionConfirmsTech(j.Source, j.ExternalID),
	})
	if canonical, ok := canon[normalize.FoldSlug(d.CompanySlug)]; ok {
		d.CompanySlug = canonical
	}
	fingerprint := jobhash.RoleFingerprint(db.UpsertJobParams{
		CompanySlug: d.CompanySlug,
		Title:       j.Title,
		Description: j.Description,
	})
	experience := pgconv.Int4(d.ExperienceYearsMin)
	isTech := pgconv.Bool(d.IsTech)
	requiresClearance := pgconv.Bool(d.RequiresClearance)

	facetsMoved := !slices.Equal(d.Countries, j.Countries) || !slices.Equal(d.Regions, j.Regions) || !slices.Equal(d.Cities, j.Cities) ||
		d.WorkMode != j.WorkMode || !slices.Equal(d.Skills, j.Skills) ||
		d.Seniority != j.Seniority ||
		d.Category != j.Category ||
		isTech != j.IsTech ||
		requiresClearance != j.RequiresClearance ||
		d.PostingLanguage != j.PostingLanguage ||
		d.EmploymentType != j.EmploymentType ||
		d.EducationLevel != j.EducationLevel ||
		d.EnglishLevel != j.EnglishLevel ||
		experience != j.ExperienceYearsMin
	fingerprintMoved := fingerprint != j.RoleFingerprint.String
	slugMoved = d.PublicSlug != j.PublicSlug || d.CompanySlug != j.CompanySlug

	return db.UpdateJobDerivedParams{
		ID:                 j.ID,
		Countries:          d.Countries,
		Regions:            d.Regions,
		Cities:             d.Cities,
		WorkMode:           d.WorkMode,
		Skills:             d.Skills,
		Seniority:          d.Seniority,
		Category:           d.Category,
		IsTech:             isTech,
		RequiresClearance:  requiresClearance,
		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		EnglishLevel:       d.EnglishLevel,
		ExperienceYearsMin: experience,
		RoleFingerprint:    pgtype.Text{String: fingerprint, Valid: true},
		PublicSlug:         d.PublicSlug,
		CompanySlug:        d.CompanySlug,
	}, facetsMoved || fingerprintMoved || slugMoved, slugMoved
}
