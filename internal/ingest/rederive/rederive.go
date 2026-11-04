// Package rederive re-derives a stored job's deterministic columns from the raw fields the
// row already holds. Two callers share it so they cannot drift: cmd/backfill-derive walks the
// table after a dictionary change, and cmd/ingest runs it on a posting a re-listing REOPENS —
// a hydrating source re-lists a closed posting without fetching its body again, so without
// this the reopened row would come back carrying whatever dictionary was current when it
// closed.
package rederive

import (
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Row re-derives a job's facets, role_fingerprint, and company slug, and reports
// whether the derived values differ from what is stored (changed → a write is needed)
// and whether specifically the company slug moved (slugMoved → the companies catalogue needs
// re-keying). The fingerprint is computed from the freshly derived company_slug, so
// the result matches what cmd/ingest/store.go writes for the same raw fields. Pure —
// safe to call concurrently.
func Row(j db.Job, canon map[string]string) (params db.UpdateJobDerivedParams, changed, slugMoved bool) {
	storedCountries, storedRegions := StoredGeography(j)
	d := jobderive.Derive(jobderive.Input{
		Title:          j.Title,
		Company:        j.Company,
		Source:         j.Source,
		ExternalID:     j.ExternalID,
		Location:       j.Location,
		Description:    j.Description,
		WorkMode:       j.WorkMode, // preserves a set work_mode (jobderive precedence)
		EmploymentType: jobderive.RetainEmploymentType(j.EmploymentType, j.Title, j.Description),
		// The stored terms carry what a picklist said at ingest; the text cannot recover it.
		Engagement: j.Engagement,
		Countries:  storedCountries,
		Regions:    storedRegions,
		// Recovers the ingest-time IsTechHint from the stored row, which never re-crawls
		// to learn it fresh: without this, a Profession itdev/itops row whose title the
		// dictionary cannot resolve would have is_tech silently rederived back to
		// unknown on every pass (issue #2601).
		IsTechHint: sources.ProfessionConfirmsTech(j.Source, j.ExternalID),
	})
	// jobderive is pure, so it re-derives the spelling the SOURCE used. Resolving through the
	// alias registry here is what stops a backfill silently undoing every merge — and it must
	// happen before the fingerprint, which is computed from the company slug.
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
	viaRecruiter := pgconv.Bool(d.ViaRecruiter)
	equity := pgconv.Text(d.Equity)

	facetsMoved := !slices.Equal(d.Countries, j.Countries) || !slices.Equal(d.Regions, j.Regions) || !slices.Equal(d.Cities, j.Cities) || !slices.Equal(d.Metros, j.Metros) ||
		d.WorkMode != j.WorkMode || !slices.Equal(d.Skills, j.Skills) ||
		d.Seniority != j.Seniority ||
		d.Category != j.Category ||
		isTech != j.IsTech ||
		requiresClearance != j.RequiresClearance ||
		viaRecruiter != j.ViaRecruiter ||
		equity != j.Equity ||
		!slices.Equal(d.ClearanceLevels, j.ClearanceLevels) ||
		!slices.Equal(d.Perks, j.Perks) ||
		!slices.Equal(d.Engagement, j.Engagement) ||
		d.PostingLanguage != j.PostingLanguage ||
		d.EmploymentType != j.EmploymentType ||
		d.EducationLevel != j.EducationLevel ||
		d.EnglishLevel != j.EnglishLevel ||
		experience != j.ExperienceYearsMin
	fingerprintMoved := fingerprint != j.RoleFingerprint.String
	slugMoved = d.CompanySlug != j.CompanySlug

	return db.UpdateJobDerivedParams{
		ID:                 j.ID,
		Countries:          d.Countries,
		Regions:            d.Regions,
		Cities:             d.Cities,
		Metros:             d.Metros,
		WorkMode:           d.WorkMode,
		Skills:             d.Skills,
		Seniority:          d.Seniority,
		Category:           d.Category,
		IsTech:             isTech,
		RequiresClearance:  requiresClearance,
		ViaRecruiter:       viaRecruiter,
		Equity:             equity,
		ClearanceLevels:    d.ClearanceLevels,
		Perks:              d.Perks,
		Engagement:         d.Engagement,
		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		EnglishLevel:       d.EnglishLevel,
		ExperienceYearsMin: experience,
		RoleFingerprint:    pgtype.Text{String: fingerprint, Valid: true},
		CompanySlug:        d.CompanySlug,
	}, facetsMoved || fingerprintMoved || slugMoved, slugMoved
}

// StoredGeography is the stored country and region a row carries, fed back in as the
// structured signal the way work_mode is. A stored country is usually the ADAPTER's (SEEK
// states countryCode AU beside "Perth WA", whose "WA" the dictionary alone reads as
// Washington), and the pass cannot tell it from one the dictionary derived — re-deriving
// dropped it from every such row. So a row with a country keeps its geography; a row
// without one is re-derived in full, which is how a dictionary fix still fills a gap.
// Correcting a stored country is a re-crawl's job (INGEST_REFETCH_ALL), never this pass's.
func StoredGeography(j db.Job) (countries, regions []string) {
	if len(j.Countries) == 0 {
		return nil, nil
	}
	return j.Countries, j.Regions
}
