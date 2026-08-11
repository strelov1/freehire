package experience

import (
	"context"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/db"
)

// queriesRepository adapts the generated *db.Queries to the owner-scoped Repository. It
// exists only to turn the generated params structs into the domain's argument shapes —
// it holds no logic, so there is nothing here a unit test would want to cover.
type queriesRepository struct{ q *db.Queries }

// NewQueriesRepository adapts *db.Queries to the Repository the Store needs.
func NewQueriesRepository(q *db.Queries) Repository { return queriesRepository{q: q} }

func (r queriesRepository) ListEmployments(ctx context.Context, userID int64) ([]db.ExperienceEmployment, error) {
	return r.q.ListExperienceEmployments(ctx, userID)
}

func (r queriesRepository) GetEmployment(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceEmployment, error) {
	return r.q.GetExperienceEmployment(ctx, db.GetExperienceEmploymentParams{ID: id, UserID: userID})
}

func (r queriesRepository) FindEmployment(ctx context.Context, userID int64, company, role string) (db.ExperienceEmployment, error) {
	return r.q.FindExperienceEmployment(ctx, db.FindExperienceEmploymentParams{
		UserID: userID, Company: company, Role: role,
	})
}

func (r queriesRepository) CreateEmployment(ctx context.Context, userID int64, e Employment) (db.ExperienceEmployment, error) {
	return r.q.CreateExperienceEmployment(ctx, db.CreateExperienceEmploymentParams{
		UserID: userID, Kind: e.Kind, Company: e.Company, Role: e.Role, Location: e.Location,
		PeriodStart: e.Start, PeriodEnd: e.End, IsCurrent: e.Current, Summary: e.Summary,
		Stack: e.Stack, Link: e.Link,
	})
}

func (r queriesRepository) UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e Employment) (db.ExperienceEmployment, error) {
	return r.q.UpdateExperienceEmployment(ctx, db.UpdateExperienceEmploymentParams{
		ID: id, UserID: userID, Kind: e.Kind, Company: e.Company, Role: e.Role, Location: e.Location,
		PeriodStart: e.Start, PeriodEnd: e.End, IsCurrent: e.Current, Summary: e.Summary,
		Stack: e.Stack, Link: e.Link,
	})
}

// FillEmploymentBlanks passes no is_current: the query does not touch it, because a CV
// that still reads "Present" for a role the user has left must not resurrect it.
func (r queriesRepository) FillEmploymentBlanks(ctx context.Context, id uuid.UUID, userID int64, e Employment) (db.ExperienceEmployment, error) {
	return r.q.FillExperienceEmploymentBlanks(ctx, db.FillExperienceEmploymentBlanksParams{
		ID: id, UserID: userID, Company: e.Company, Role: e.Role, Location: e.Location,
		PeriodStart: e.Start, PeriodEnd: e.End, Summary: e.Summary, Stack: e.Stack, Link: e.Link,
	})
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

func (r queriesRepository) DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) (int64, error) {
	return r.q.DeleteExperienceAtom(ctx, db.DeleteExperienceAtomParams{ID: id, UserID: userID})
}

func (r queriesRepository) MergeAtoms(ctx context.Context, userID int64, keepID, loserID uuid.UUID, a Atom) (db.ExperienceAtom, error) {
	row, err := r.q.MergeExperienceAtoms(ctx, db.MergeExperienceAtomsParams{
		Context: a.Context, Metrics: a.Metrics, Skills: a.Skills,
		Provenance: string(a.Provenance), KeepID: keepID, UserID: userID, LoserID: loserID,
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
