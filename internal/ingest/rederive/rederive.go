// Package rederive re-derives a stored job's deterministic columns from the raw fields the
// row already holds. Two callers share it so they cannot drift: cmd/backfill-derive walks the
// table after a dictionary change, and cmd/ingest runs it on a posting a re-listing REOPENS —
// a hydrating source re-lists a closed posting without fetching its body again, so without
// this the reopened row would come back carrying whatever dictionary was current when it
// closed.
package rederive

import (
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/job/reqextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Row re-derives a job's facets, requirements_derived, role_fingerprint, and company slug, and reports
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

	fingerprintMoved := fingerprint != j.RoleFingerprint.String
	slugMoved = d.CompanySlug != j.CompanySlug

	params = db.UpdateJobDerivedParams{
		ID:                  j.ID,
		Countries:           d.Countries,
		Regions:             d.Regions,
		Cities:              d.Cities,
		Metros:              d.Metros,
		WorkMode:            d.WorkMode,
		Skills:              d.Skills,
		Seniority:           d.Seniority,
		Category:            d.Category,
		IsTech:              isTech,
		RequiresClearance:   requiresClearance,
		ViaRecruiter:        viaRecruiter,
		Equity:              equity,
		ClearanceLevels:     d.ClearanceLevels,
		Perks:               d.Perks,
		Engagement:          d.Engagement,
		RequirementsDerived: reqextract.DeriveJSON(j.Description),
		PostingLanguage:     d.PostingLanguage,
		EmploymentType:      d.EmploymentType,
		EducationLevel:      d.EducationLevel,
		EnglishLevel:        d.EnglishLevel,
		ExperienceYearsMin:  experience,
		RoleFingerprint:     pgtype.Text{String: fingerprint, Valid: true},
		CompanySlug:         d.CompanySlug,
	}
	return params, FacetsDiffer(params, j) || fingerprintMoved || slugMoved, slugMoved
}

// FacetsDiffer reports whether any derived facet in params differs from the stored row,
// leaving aside the company slug and fingerprint — a caller that keeps those as stored asks
// this to learn whether a write is still owed.
func FacetsDiffer(p db.UpdateJobDerivedParams, j db.Job) bool {
	return !slices.Equal(p.Countries, j.Countries) || !slices.Equal(p.Regions, j.Regions) ||
		!slices.Equal(p.Cities, j.Cities) || !slices.Equal(p.Metros, j.Metros) ||
		p.WorkMode != j.WorkMode || !slices.Equal(p.Skills, j.Skills) ||
		p.Seniority != j.Seniority ||
		p.Category != j.Category ||
		p.IsTech != j.IsTech ||
		p.RequiresClearance != j.RequiresClearance ||
		p.ViaRecruiter != j.ViaRecruiter ||
		p.Equity != j.Equity ||
		!slices.Equal(p.ClearanceLevels, j.ClearanceLevels) ||
		!slices.Equal(p.Perks, j.Perks) ||
		!slices.Equal(p.Engagement, j.Engagement) ||
		p.PostingLanguage != j.PostingLanguage ||
		p.EmploymentType != j.EmploymentType ||
		p.EducationLevel != j.EducationLevel ||
		p.EnglishLevel != j.EnglishLevel ||
		p.ExperienceYearsMin != j.ExperienceYearsMin ||
		requirementsMoved(p.RequirementsDerived, j.RequirementsDerived)
}

// requirementsMoved compares the derived and stored requirements lists as decoded JSON: Postgres
// re-spaces jsonb on read, so a byte compare would flag every stored list as changed.
func requirementsMoved(derived, stored []byte) bool {
	type req struct {
		Text     string `json:"text"`
		Priority string `json:"priority"`
	}
	var d, st []req
	if json.Unmarshal(derived, &d) != nil || json.Unmarshal(stored, &st) != nil {
		return true
	}
	return !slices.Equal(d, st)
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
