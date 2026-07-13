package subscription

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository, mapping Postgres
// conditions onto package sentinels.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]SubscriptionListItem, error) {
	rows, err := r.q.ListSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SubscriptionListItem, len(rows))
	for i, row := range rows {
		out[i] = SubscriptionListItem{
			Subscription: Subscription{
				ID:            row.ID,
				SavedSearchID: row.SavedSearchID,
				Channel:       row.Channel,
				Active:        row.Active,
				CreatedAt:     pgconv.TimePtr(row.CreatedAt),
			},
			SavedSearchName: row.SavedSearchName,
		}
	}
	return out, nil
}

func (r *QueriesRepository) Create(ctx context.Context, userID, savedSearchID int64, channel string) (Subscription, error) {
	row, err := r.q.CreateSubscription(ctx, db.CreateSubscriptionParams{
		Channel:       channel,
		SavedSearchID: savedSearchID,
		UserID:        userID,
	})
	// No row means the saved search is missing or not the caller's (the INSERT ...
	// SELECT found nothing to insert).
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrSavedSearchNotFound
	}
	if pgerr.IsUniqueViolation(err) {
		return Subscription{}, ErrDuplicate
	}
	if err != nil {
		return Subscription{}, err
	}
	return fromRow(row), nil
}

func (r *QueriesRepository) SetActive(ctx context.Context, userID, id int64, active bool) (Subscription, error) {
	row, err := r.q.SetSubscriptionActive(ctx, db.SetSubscriptionActiveParams{
		Active: active,
		ID:     id,
		UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	return fromRow(row), nil
}

func (r *QueriesRepository) Delete(ctx context.Context, userID, id int64) error {
	affected, err := r.q.DeleteSubscription(ctx, db.DeleteSubscriptionParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// fromRow maps the generated db row to the package domain type, dropping the internal
// columns (user_id, destination, start_at) the use case does not need.
func fromRow(row db.Subscription) Subscription {
	return Subscription{
		ID:            row.ID,
		SavedSearchID: row.SavedSearchID,
		Channel:       row.Channel,
		Active:        row.Active,
		CreatedAt:     pgconv.TimePtr(row.CreatedAt),
	}
}
