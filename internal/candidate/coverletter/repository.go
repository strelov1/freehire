package coverletter

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// queriesRepository adapts the generated *db.Queries to the owner-scoped Repository. It holds
// no logic beyond turning generated params structs and pgtype values into the domain's shapes,
// so there is nothing here a unit test would want to cover — the same division experience
// keeps between its store and its repository.
type queriesRepository struct{ q *db.Queries }

// NewQueriesRepository adapts *db.Queries to the Repository the Store needs.
func NewQueriesRepository(q *db.Queries) Repository { return queriesRepository{q: q} }

func (r queriesRepository) Get(ctx context.Context, userID, jobID int64) (Stored, error) {
	row, err := r.q.GetCoverLetter(ctx, db.GetCoverLetterParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Stored{}, err
	}
	return Stored{
		Letter: Letter{
			Body:     row.Body,
			Cited:    fromPgUUIDs(row.CitedAtomIds),
			Language: row.Language,
		},
		Model:     row.Model,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func (r queriesRepository) Upsert(ctx context.Context, userID, jobID int64, s Stored) error {
	return r.q.UpsertCoverLetter(ctx, db.UpsertCoverLetterParams{
		UserID:       userID,
		JobID:        jobID,
		Body:         s.Body,
		CitedAtomIds: toPgUUIDs(s.Cited),
		Language:     s.Language,
		Model:        s.Model,
	})
}

// toPgUUIDs converts to the driver's uuid type. The result is never nil: the column is NOT
// NULL and pgx sends a nil slice as SQL NULL, which a column DEFAULT does not cover.
func toPgUUIDs(ids []uuid.UUID) []pgtype.UUID {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		out = append(out, pgtype.UUID{Bytes: id, Valid: true})
	}
	return out
}

// fromPgUUIDs converts back, dropping invalid entries. An invalid element cannot arrive from a
// NOT NULL column, but reading one as the zero uuid would put a citation into the UI that
// resolves to nothing, and dropping it is the honest degradation.
func fromPgUUIDs(ids []pgtype.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if !id.Valid {
			continue
		}
		out = append(out, uuid.UUID(id.Bytes))
	}
	return out
}
