// Command backfill-derive re-derives the six deterministic dictionary facets —
// countries, regions, work_mode, skills, seniority, category — on existing jobs in
// a single pass, replacing the three separate per-facet backfill commands
// (backfill-geo/-skills/-class). Ingest fills these on every crawl via
// jobderive.Derive; rows that predate a dictionary change — and closed jobs that
// never re-crawl — keep the stale values until this one-off worker rewrites them.
// It pages the whole table and exits. Idempotent for jobs whose facets come from
// the title/location/description dictionaries alone: re-deriving is a pure function
// of those fields, so a second run rewrites nothing.
//
// Slugs are deliberately not touched (re-slugging stays cmd/reslug). work_mode is
// preserved when already set: jobderive keeps a row's existing (possibly
// adapter-structured) work_mode over the parsed-location hint. The other
// structured-source facets are NOT preserved: an adapter that emits a grade,
// category, skills, or required-experience directly (e.g. getmatch) supplies those
// only at ingest, and this command re-derives seniority/category/skills/
// experience_years_min from the stored description columns — so running it
// overwrites such structured values with the dictionary's. This is intentional:
// the command's job is to propagate dictionary changes, which must keep updating
// those facets for the dictionary-derived majority. A boardless adapter like
// getmatch re-supplies the structured facets on its next full crawl.
package main

import (
	"context"
	"log"
	"os"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/worker"
)

// toInt4 maps an optional int (experience_years_min) to the pgtype the generated
// params expect; a nil pointer becomes SQL NULL.
func toInt4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}

// backfillBatchSize bounds how many jobs are read per keyset page.
const backfillBatchSize = 500

// facetStore is the slice of the data layer the backfill needs: page the table by
// keyset and rewrite a row's six facet columns. *db.Queries satisfies it; tests
// use a fake.
type facetStore interface {
	ListJobsByIDAfter(ctx context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error)
	UpdateJobFacets(ctx context.Context, arg db.UpdateJobFacetsParams) error
}

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

	scanned, updated, err := backfillAll(ctx, db.New(pool))
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}
	log.Printf("backfill-derive done: scanned=%d updated=%d", scanned, updated)
	return 0
}

// backfillAll re-derives every job's six facet columns and rewrites the rows whose
// derived facets differ from what is stored. It pages by keyset (id > last seen)
// so concurrent writes cannot skip or repeat rows.
func backfillAll(ctx context.Context, store facetStore) (scanned, updated int, err error) {
	var afterID int64
	for {
		jobs, err := store.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{
			AfterID:   afterID,
			BatchSize: backfillBatchSize,
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
			d := jobderive.Derive(jobderive.Input{
				Title:       j.Title,
				Company:     j.Company,
				Source:      j.Source,
				ExternalID:  j.ExternalID,
				Location:    j.Location,
				Description: j.Description,
				WorkMode:    j.WorkMode, // preserves a set work_mode (jobderive precedence)
			})
			experience := toInt4(d.ExperienceYearsMin)
			unchanged := slices.Equal(d.Countries, j.Countries) &&
				slices.Equal(d.Regions, j.Regions) &&
				slices.Equal(d.Cities, j.Cities) &&
				d.WorkMode == j.WorkMode &&
				slices.Equal(d.Skills, j.Skills) &&
				d.Seniority == j.Seniority &&
				d.Category == j.Category &&
				d.PostingLanguage == j.PostingLanguage &&
				d.EmploymentType == j.EmploymentType &&
				d.EducationLevel == j.EducationLevel &&
				d.EnglishLevel == j.EnglishLevel &&
				experience == j.ExperienceYearsMin
			if unchanged {
				continue
			}
			if err := store.UpdateJobFacets(ctx, db.UpdateJobFacetsParams{
				ID:        j.ID,
				Countries: d.Countries,
				Regions:   d.Regions,
				Cities:    d.Cities,
				WorkMode:  d.WorkMode,
				Skills:    d.Skills,
				Seniority: d.Seniority,
				Category:  d.Category,

				PostingLanguage:    d.PostingLanguage,
				EmploymentType:     d.EmploymentType,
				EducationLevel:     d.EducationLevel,
				EnglishLevel:       d.EnglishLevel,
				ExperienceYearsMin: experience,
			}); err != nil {
				return scanned, updated, err
			}
			updated++
		}

		if len(jobs) < backfillBatchSize {
			break
		}
	}

	return scanned, updated, nil
}
