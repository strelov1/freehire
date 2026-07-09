package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

// ErrNotFound is returned by Repository.Load when no job matches the identity.
var ErrNotFound = errors.New("job: not found")

// Extras is the read-only projection data that rides on the jobs row but is NOT
// part of the Job aggregate's invariants: the engagement counters (materialized
// from user_jobs) and the collection slugs (denormalized from the company). Keeping
// them out of Job preserves a clean write surface — New/Close/Reopen never see them
// — while the read path still gets them in one load. Zero on a fresh New; populated
// only by a repository Load. It lives here (not in jobview) so the repository can
// return it without the domain importing the wire layer.
type Extras struct {
	ViewCount    int32
	AppliedCount int32
	Collections  []string
}

// Repository is the persistence port for the Job aggregate: load by dedup
// identity and soft-close by it. The domain never sees db.Job — the adapter maps
// between the persistence row and the aggregate. Load also returns the read-only
// Extras that ride on the same row. (Upsert lives with the write path that computes
// the content-hash/role-fingerprint signals; it is added when the ingest path
// switches to the factory.)
type Repository interface {
	Load(ctx context.Context, source, externalID string) (Job, Extras, error)
	Close(ctx context.Context, source, externalID string) (int64, error)
}

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository port, mirroring the
// jobtracking package's convention.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository wraps q as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Load returns the domain Job and its read-only Extras for the given dedup
// identity, or ErrNotFound.
func (r *QueriesRepository) Load(ctx context.Context, source, externalID string) (Job, Extras, error) {
	row, err := r.q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
		Source:     source,
		ExternalID: externalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, Extras{}, ErrNotFound
	}
	if err != nil {
		return Job{}, Extras{}, err
	}
	return FromRow(row)
}

// Close soft-closes the job identified by (source, external_id) and reports how
// many rows it closed (0 when already closed or absent) — the persistence side of
// the aggregate's Close decision.
func (r *QueriesRepository) Close(ctx context.Context, source, externalID string) (int64, error) {
	return r.q.CloseJobBySourceExternalID(ctx, db.CloseJobBySourceExternalIDParams{
		Source:     source,
		ExternalID: externalID,
	})
}

// FromRow is the anti-corruption mapping from a persistence row to the domain
// Job plus its read-only Extras. It is the hydration path shared by the repository
// Load and the jobview projection shim; it never derives (a stored row already
// carries its facets), so it does not bypass the New construction invariant. It is
// the single place the domain depends on the db row shape.
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

// int4Ptr renders a nullable Postgres int4 as *int (nil when unset).
func int4Ptr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}
