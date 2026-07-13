package job

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

// Extras is the read-only projection data that rides on the jobs row but is NOT
// part of the Job aggregate's invariants: the engagement counters (materialized
// from user_jobs) and the collection slugs (denormalized from the company). Keeping
// them out of Job preserves a clean write surface — the New factory never sees them
// — while the read path still gets them in one load. Zero on a fresh New; populated
// only by FromRow. It lives here (not in jobview) so a load can return it without
// the domain importing the wire layer.
type Extras struct {
	ViewCount    int32
	AppliedCount int32
	Collections  []string
}

// FromRow is the anti-corruption mapping from a persistence row to the domain
// Job plus its read-only Extras. It is the hydration path used by the jobview
// projection shim; it never derives (a stored row already carries its facets), so it
// does not bypass the New construction invariant. It is the single place the domain
// depends on the db row shape.
func FromRow(r db.Job) (Job, Extras, error) {
	j, err := jobFromRow(r)
	if err != nil {
		return Job{}, Extras{}, err
	}
	return j, extrasFromRow(r), nil
}

// jobFromRow maps the aggregate-owned fields of a persistence row into a domain
// Job: pgtype scalars become domain types and the enrichment JSONB is decoded into
// the typed, raw (pre-fold) Enrichment.
func jobFromRow(r db.Job) (Job, error) {
	var e enrich.Enrichment
	if len(r.Enrichment) > 0 {
		if err := json.Unmarshal(r.Enrichment, &e); err != nil {
			return Job{}, fmt.Errorf("job: decode enrichment for job %d: %w", r.ID, err)
		}
	}
	return Job{f: Fields{
		Source:      r.Source,
		ExternalID:  r.ExternalID,
		URL:         r.URL,
		Title:       r.Title,
		Company:     r.Company,
		CompanySlug: r.CompanySlug,
		PublicSlug:  r.PublicSlug,
		Location:    r.Location,
		Remote:      r.Remote,
		Description: r.Description,
		PostedAt:    tsPtr(r.PostedAt),

		Countries: r.Countries,
		Regions:   r.Regions,
		Cities:    r.Cities,
		WorkMode:  r.WorkMode,
		Skills:    r.Skills,
		Seniority: r.Seniority,
		Category:  r.Category,
		IsTech:    boolPtr(r.IsTech),

		PostingLanguage:    r.PostingLanguage,
		EmploymentType:     r.EmploymentType,
		EducationLevel:     r.EducationLevel,
		EnglishLevel:       r.EnglishLevel,
		ExperienceYearsMin: int4Ptr(r.ExperienceYearsMin),

		ClosedAt:          tsPtr(r.ClosedAt),
		EnrichmentVersion: r.EnrichmentVersion,

		ID:            r.ID,
		ManuallyAdded: r.CreatedBy.Valid,
		Enrichment:    e,
		EnrichedAt:    tsPtr(r.EnrichedAt),
		CreatedAt:     tsPtr(r.CreatedAt),
		UpdatedAt:     tsPtr(r.UpdatedAt),
	}}, nil
}

// extrasFromRow pulls the read-only projection data (engagement counters +
// denormalized collection slugs) off a persistence row. Kept separate from
// jobFromRow so the aggregate mapping stays free of non-aggregate fields.
func extrasFromRow(r db.Job) Extras {
	return Extras{
		ViewCount:    r.ViewCount,
		AppliedCount: r.AppliedCount,
		Collections:  r.Collections,
	}
}

// tsPtr renders a nullable Postgres timestamp as *time.Time (nil when unset),
// keeping the aggregate free of pgtype.
func tsPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// boolPtr renders a nullable Postgres bool as *bool (nil when unset), keeping the
// tri-state is_tech signal out of pgtype in the aggregate.
func boolPtr(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	v := b.Bool
	return &v
}

// int4Ptr renders a nullable Postgres int4 as *int (nil when unset).
func int4Ptr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}
