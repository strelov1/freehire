package answerbank

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Queries is the generated surface this repository needs — an interface rather than
// *db.Queries so a caller can substitute one, the same shape
// internal/candidate/experience's own repository takes.
type Queries interface {
	UpsertScreeningAnswer(ctx context.Context, arg db.UpsertScreeningAnswerParams) error
	ListScreeningAnswers(ctx context.Context, userID int64) ([]db.ScreeningAnswerBank, error)
	DeleteScreeningAnswer(ctx context.Context, arg db.DeleteScreeningAnswerParams) (int64, error)
}

// QueriesRepository adapts the generated queries to Repository.
type QueriesRepository struct{ q Queries }

// NewQueriesRepository builds a Repository over the generated queries.
func NewQueriesRepository(q Queries) *QueriesRepository { return &QueriesRepository{q: q} }

var _ Repository = (*QueriesRepository)(nil)

func (r *QueriesRepository) Upsert(ctx context.Context, userID int64, a Answer) error {
	return r.q.UpsertScreeningAnswer(ctx, db.UpsertScreeningAnswerParams{
		UserID: userID, Topic: a.Topic, Question: a.Question,
		Answer: a.Answer, Provenance: a.Provenance,
	})
}

func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]Answer, error) {
	rows, err := r.q.ListScreeningAnswers(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Answer, 0, len(rows))
	for _, row := range rows {
		out = append(out, Answer{
			ID: row.ID, Topic: row.Topic, Question: row.Question,
			Answer: row.Answer, Provenance: row.Provenance,
			UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *QueriesRepository) Delete(ctx context.Context, userID, id int64) (int64, error) {
	return r.q.DeleteScreeningAnswer(ctx, db.DeleteScreeningAnswerParams{ID: id, UserID: userID})
}
