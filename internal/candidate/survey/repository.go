package survey

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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

// Get returns the user's survey answers, mapping no row to ErrNotFound.
func (r *QueriesRepository) Get(ctx context.Context, userID int64) (Responses, error) {
	row, err := r.q.GetCandidateSurvey(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Responses{}, ErrNotFound
	}
	if err != nil {
		return Responses{}, err
	}
	return answersFromRow(row), nil
}

// Upsert creates or replaces the user's survey answers.
func (r *QueriesRepository) Upsert(ctx context.Context, userID int64, a Responses) (Responses, error) {
	row, err := r.q.UpsertCandidateSurvey(ctx, db.UpsertCandidateSurveyParams{
		UserID:                userID,
		JobSearchStage:        pgconv.Text(derefString(a.JobSearchStage)),
		BiggestChallenge:      pgconv.Text(derefString(a.BiggestChallenge)),
		BiggestChallengeNote:  pgconv.Text(derefString(a.BiggestChallengeNote)),
		CurrentIncomeAmount:   pgconv.Int4(a.CurrentIncomeAmount),
		CurrentIncomeCurrency: pgconv.Text(derefString(a.CurrentIncomeCurrency)),
		CurrentIncomePeriod:   pgconv.Text(derefString(a.CurrentIncomePeriod)),
	})
	if err != nil {
		return Responses{}, err
	}
	return answersFromRow(row), nil
}

// answersFromRow maps the generated db row to the package domain type.
func answersFromRow(row db.CandidateSurvey) Responses {
	return Responses{
		JobSearchStage:        pgconv.TextPtr(row.JobSearchStage),
		BiggestChallenge:      pgconv.TextPtr(row.BiggestChallenge),
		BiggestChallengeNote:  pgconv.TextPtr(row.BiggestChallengeNote),
		CurrentIncomeAmount:   pgconv.IntPtr(row.CurrentIncomeAmount),
		CurrentIncomeCurrency: pgconv.TextPtr(row.CurrentIncomeCurrency),
		CurrentIncomePeriod:   pgconv.TextPtr(row.CurrentIncomePeriod),
	}
}

// derefString returns the empty string for a nil pointer, its content otherwise —
// pgconv.Text already treats "" as NULL, so this is the one-line bridge from Responses'
// *string fields to that convention.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
