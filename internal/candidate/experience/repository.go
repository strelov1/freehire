package experience

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/db"
)

// employmentRow is what Repository returns for one employment, independent of sqlc's
// per-query row type. Each of the six employment queries below selects a different
// column set than the full experience_employments table (period_start/period_end text
// are gone from all of them; ListExperienceEmploymentDatesForBackfill selects only
// those, for the backfill worker), so sqlc generates a distinct Go type per query
// instead of the one shared db.ExperienceEmployment these used to return. One shape
// here means employmentFromRow (store.go) has exactly one type to convert from,
// and fakeRepo (store_test.go) needs no sqlc-generated type at all.
type employmentRow struct {
	ID               uuid.UUID
	UserID           int64
	Kind             string
	Company          string
	Role             string
	Location         string
	PeriodStartYear  pgtype.Int4
	PeriodStartMonth pgtype.Int2
	PeriodEndYear    pgtype.Int4
	PeriodEndMonth   pgtype.Int2
	IsCurrent        bool
	Summary          string
	Stack            []string
	Link             string
	CreatedAt        pgtype.Timestamptz
	UpdatedAt        pgtype.Timestamptz
}

// PeriodToColumns splits d into the two nullable columns experience_employments stores a
// period boundary as — both invalid (NULL) for a nil date, month additionally invalid for
// a year-only date. Exported so cmd/backfill-experience-dates, which fills these same
// columns from parsed free text, shares this mapping instead of re-deriving it.
func PeriodToColumns(d *perioddate.PeriodDate) (year pgtype.Int4, month pgtype.Int2) {
	if d == nil {
		return year, month
	}
	year = pgtype.Int4{Int32: int32(d.Year), Valid: true}
	if d.Month != 0 {
		month = pgtype.Int2{Int16: int16(d.Month), Valid: true}
	}
	return year, month
}

// PeriodFromColumns is PeriodToColumns' inverse, used by employmentFromRow (store.go) to
// rebuild the domain's *perioddate.PeriodDate from the two columns a row carries.
func PeriodFromColumns(year pgtype.Int4, month pgtype.Int2) *perioddate.PeriodDate {
	if !year.Valid {
		return nil
	}
	d := &perioddate.PeriodDate{Year: int(year.Int32)}
	if month.Valid {
		d.Month = int(month.Int16)
	}
	return d
}

// queriesRepository adapts the generated *db.Queries to the owner-scoped Repository. It
// exists only to turn the generated params structs into the domain's argument shapes —
// it holds no logic, so there is nothing here a unit test would want to cover.
type queriesRepository struct{ q *db.Queries }

// NewQueriesRepository adapts *db.Queries to the Repository the Store needs.
func NewQueriesRepository(q *db.Queries) Repository { return queriesRepository{q: q} }

func (r queriesRepository) ListEmployments(ctx context.Context, userID int64) ([]employmentRow, error) {
	rows, err := r.q.ListExperienceEmployments(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]employmentRow, len(rows))
	for i, row := range rows {
		out[i] = employmentRow{
			ID: row.ID, UserID: row.UserID, Kind: row.Kind, Company: row.Company, Role: row.Role, Location: row.Location,
			PeriodStartYear: row.PeriodStartYear, PeriodStartMonth: row.PeriodStartMonth,
			PeriodEndYear: row.PeriodEndYear, PeriodEndMonth: row.PeriodEndMonth,
			IsCurrent: row.IsCurrent, Summary: row.Summary, Stack: row.Stack, Link: row.Link,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

func (r queriesRepository) GetEmployment(ctx context.Context, id uuid.UUID, userID int64) (employmentRow, error) {
	row, err := r.q.GetExperienceEmployment(ctx, db.GetExperienceEmploymentParams{ID: id, UserID: userID})
	if err != nil {
		return employmentRow{}, err
	}
	return employmentRow{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role, Location: row.Location,
		PeriodStartYear: row.PeriodStartYear, PeriodStartMonth: row.PeriodStartMonth,
		PeriodEndYear: row.PeriodEndYear, PeriodEndMonth: row.PeriodEndMonth,
		IsCurrent: row.IsCurrent, Summary: row.Summary, Stack: row.Stack, Link: row.Link,
	}, nil
}

func (r queriesRepository) FindEmployment(ctx context.Context, userID int64, company, role string) (employmentRow, error) {
	row, err := r.q.FindExperienceEmployment(ctx, db.FindExperienceEmploymentParams{
		UserID: userID, Company: company, Role: role,
	})
	if err != nil {
		return employmentRow{}, err
	}
	return employmentRow{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role, Location: row.Location,
		PeriodStartYear: row.PeriodStartYear, PeriodStartMonth: row.PeriodStartMonth,
		PeriodEndYear: row.PeriodEndYear, PeriodEndMonth: row.PeriodEndMonth,
		IsCurrent: row.IsCurrent, Summary: row.Summary, Stack: row.Stack, Link: row.Link,
	}, nil
}

func (r queriesRepository) CreateEmployment(ctx context.Context, userID int64, e Employment) (employmentRow, error) {
	startYear, startMonth := PeriodToColumns(e.Start)
	endYear, endMonth := PeriodToColumns(e.End)
	row, err := r.q.CreateExperienceEmployment(ctx, db.CreateExperienceEmploymentParams{
		UserID: userID, Kind: e.Kind, Company: e.Company, Role: e.Role, Location: e.Location,
		PeriodStartYear: startYear, PeriodStartMonth: startMonth,
		PeriodEndYear: endYear, PeriodEndMonth: endMonth,
		IsCurrent: e.Current, Summary: e.Summary, Stack: e.Stack, Link: e.Link,
	})
	if err != nil {
		return employmentRow{}, err
	}
	return employmentRow{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role, Location: row.Location,
		PeriodStartYear: row.PeriodStartYear, PeriodStartMonth: row.PeriodStartMonth,
		PeriodEndYear: row.PeriodEndYear, PeriodEndMonth: row.PeriodEndMonth,
		IsCurrent: row.IsCurrent, Summary: row.Summary, Stack: row.Stack, Link: row.Link,
	}, nil
}

func (r queriesRepository) UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e Employment) (employmentRow, error) {
	startYear, startMonth := PeriodToColumns(e.Start)
	endYear, endMonth := PeriodToColumns(e.End)
	row, err := r.q.UpdateExperienceEmployment(ctx, db.UpdateExperienceEmploymentParams{
		ID: id, UserID: userID, Kind: e.Kind, Company: e.Company, Role: e.Role, Location: e.Location,
		PeriodStartYear: startYear, PeriodStartMonth: startMonth,
		PeriodEndYear: endYear, PeriodEndMonth: endMonth,
		IsCurrent: e.Current, Summary: e.Summary, Stack: e.Stack, Link: e.Link,
	})
	if err != nil {
		return employmentRow{}, err
	}
	return employmentRow{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role, Location: row.Location,
		PeriodStartYear: row.PeriodStartYear, PeriodStartMonth: row.PeriodStartMonth,
		PeriodEndYear: row.PeriodEndYear, PeriodEndMonth: row.PeriodEndMonth,
		IsCurrent: row.IsCurrent, Summary: row.Summary, Stack: row.Stack, Link: row.Link,
	}, nil
}

// FillEmploymentBlanks passes no is_current: the query does not touch it, because a CV
// that still reads "Present" for a role the user has left must not resurrect it.
func (r queriesRepository) FillEmploymentBlanks(ctx context.Context, id uuid.UUID, userID int64, e Employment) (employmentRow, error) {
	startYear, startMonth := PeriodToColumns(e.Start)
	endYear, endMonth := PeriodToColumns(e.End)
	row, err := r.q.FillExperienceEmploymentBlanks(ctx, db.FillExperienceEmploymentBlanksParams{
		ID: id, UserID: userID, Company: e.Company, Role: e.Role, Location: e.Location,
		PeriodStartYear: startYear, PeriodStartMonth: startMonth,
		PeriodEndYear: endYear, PeriodEndMonth: endMonth,
		Summary: e.Summary, Stack: e.Stack, Link: e.Link,
	})
	if err != nil {
		return employmentRow{}, err
	}
	return employmentRow{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role, Location: row.Location,
		PeriodStartYear: row.PeriodStartYear, PeriodStartMonth: row.PeriodStartMonth,
		PeriodEndYear: row.PeriodEndYear, PeriodEndMonth: row.PeriodEndMonth,
		IsCurrent: row.IsCurrent, Summary: row.Summary, Stack: row.Stack, Link: row.Link,
	}, nil
}

func (r queriesRepository) DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) (int64, error) {
	return r.q.DeleteExperienceEmployment(ctx, db.DeleteExperienceEmploymentParams{ID: id, UserID: userID})
}

func (r queriesRepository) ListAtoms(ctx context.Context, userID int64) ([]db.ExperienceAtom, error) {
	return r.q.ListExperienceAtoms(ctx, userID)
}

func (r queriesRepository) GetAtom(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceAtom, error) {
	return r.q.GetExperienceAtom(ctx, db.GetExperienceAtomParams{ID: id, UserID: userID})
}

func (r queriesRepository) InsertAtomIfNew(ctx context.Context, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error) {
	return r.q.InsertExperienceAtomIfNew(ctx, db.InsertExperienceAtomIfNewParams{
		UserID: userID, EmploymentID: a.EmploymentID, Claim: a.Claim, ClaimKey: claimKey,
		Context: a.Context, Metrics: a.Metrics, Skills: a.Skills,
		Provenance: string(a.Provenance), SourceRef: a.SourceRef,
	})
}

func (r queriesRepository) UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error) {
	return r.q.UpdateExperienceAtom(ctx, db.UpdateExperienceAtomParams{
		ID: id, UserID: userID, EmploymentID: a.EmploymentID, Claim: a.Claim, ClaimKey: claimKey,
		Context: a.Context, Metrics: a.Metrics, Skills: a.Skills, Provenance: string(a.Provenance),
	})
}

func (r queriesRepository) UpdateAtomKeepingProvenance(ctx context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error) {
	return r.q.UpdateExperienceAtomKeepingProvenance(ctx, db.UpdateExperienceAtomKeepingProvenanceParams{
		ID: id, UserID: userID, EmploymentID: a.EmploymentID, Claim: a.Claim, ClaimKey: claimKey,
		Context: a.Context, Metrics: a.Metrics, Skills: a.Skills,
	})
}

func (r queriesRepository) DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) (int64, error) {
	return r.q.DeleteExperienceAtom(ctx, db.DeleteExperienceAtomParams{ID: id, UserID: userID})
}

func (r queriesRepository) MergeAtoms(ctx context.Context, userID int64, keepID, loserID uuid.UUID, keepUpdatedAt, loserUpdatedAt time.Time, a Atom) (db.ExperienceAtom, error) {
	row, err := r.q.MergeExperienceAtoms(ctx, db.MergeExperienceAtomsParams{
		Context: a.Context, Metrics: a.Metrics, Skills: a.Skills,
		Provenance: string(a.Provenance), KeepID: keepID, UserID: userID, LoserID: loserID,
		KeepUpdatedAt:  pgtype.Timestamptz{Time: keepUpdatedAt, Valid: true},
		LoserUpdatedAt: pgtype.Timestamptz{Time: loserUpdatedAt, Valid: true},
	})
	if err != nil {
		return db.ExperienceAtom{}, err
	}
	// The query's final SELECT reads through a CTE alias rather than the experience_atoms
	// table directly, so sqlc cannot trace its columns back to the sqlc.yaml uuid.UUID
	// overrides and generates raw pgtype.UUID fields instead of the ExperienceAtom model.
	// Same data; converted here rather than changing the Repository/Store contract.
	atom := db.ExperienceAtom{
		ID:         uuid.UUID(row.ID.Bytes),
		UserID:     row.UserID,
		Claim:      row.Claim,
		ClaimKey:   row.ClaimKey,
		Context:    row.Context,
		Metrics:    row.Metrics,
		Skills:     row.Skills,
		Provenance: row.Provenance,
		SourceRef:  row.SourceRef,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
	if row.EmploymentID.Valid {
		id := uuid.UUID(row.EmploymentID.Bytes)
		atom.EmploymentID = &id
	}
	return atom, nil
}
