package accountdelete

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Compile-time proof that QueriesRepository satisfies the port.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the production Repository over *db.Queries, keeping pgtype out of
// the service.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func (r *QueriesRepository) BlobKeys(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.q.ListUserBlobKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if key := pgconv.TextString(row); key != "" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (r *QueriesRepository) DeleteUser(ctx context.Context, userID int64) error {
	return r.q.DeleteUser(ctx, userID)
}
