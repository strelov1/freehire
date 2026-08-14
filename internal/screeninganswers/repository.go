package screeninganswers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository. It maps the no-row condition on
// Get to ErrNotFound; Upsert needs no mapping (the PRIMARY KEY (user_id) makes it
// conflict-free).
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Get returns the user's screening answers, mapping no row to ErrNotFound.
func (r *QueriesRepository) Get(ctx context.Context, userID int64) (Answers, error) {
	row, err := r.q.GetScreeningAnswers(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Answers{}, ErrNotFound
	}
	if err != nil {
		return Answers{}, err
	}
	return answersFromRow(row), nil
}

// Upsert creates or replaces the user's screening answers.
func (r *QueriesRepository) Upsert(ctx context.Context, userID int64, a Answers) (Answers, error) {
	row, err := r.q.UpsertScreeningAnswers(ctx, db.UpsertScreeningAnswersParams{
		UserID:                userID,
		AuthorizedCountries:   a.AuthorizedCountries,
		VisaSponsorshipNeeded: pgconv.Bool(a.VisaSponsorshipNeeded),
		DesiredSalaryAmount:   pgconv.Int4(a.DesiredSalaryAmount),
		DesiredSalaryCurrency: pgconv.Text(derefString(a.DesiredSalaryCurrency)),
		DesiredSalaryPeriod:   pgconv.Text(derefString(a.DesiredSalaryPeriod)),
		NoticePeriodDays:      pgconv.Int4(a.NoticePeriodDays),
		WillingToRelocate:     pgconv.Bool(a.WillingToRelocate),
		Age18OrOlder:          pgconv.Bool(a.Age18OrOlder),
	})
	if err != nil {
		return Answers{}, err
	}
	return answersFromRow(row), nil
}

// answersFromRow maps the generated db row to the package domain type.
func answersFromRow(row db.ScreeningAnswer) Answers {
	return Answers{
		AuthorizedCountries:   row.AuthorizedCountries,
		VisaSponsorshipNeeded: pgconv.BoolPtr(row.VisaSponsorshipNeeded),
		DesiredSalaryAmount:   pgconv.IntPtr(row.DesiredSalaryAmount),
		DesiredSalaryCurrency: pgconv.TextPtr(row.DesiredSalaryCurrency),
		DesiredSalaryPeriod:   pgconv.TextPtr(row.DesiredSalaryPeriod),
		NoticePeriodDays:      pgconv.IntPtr(row.NoticePeriodDays),
		WillingToRelocate:     pgconv.BoolPtr(row.WillingToRelocate),
		Age18OrOlder:          pgconv.BoolPtr(row.Age18OrOlder),
	}
}

// derefString returns the empty string for a nil pointer, its content otherwise —
// pgconv.Text already treats "" as NULL, so this is the one-line bridge from Answers'
// *string fields to that convention.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
