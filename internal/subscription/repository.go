package subscription

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/db"
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

func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]db.ListSubscriptionsRow, error) {
	return r.q.ListSubscriptions(ctx, userID)
}

func (r *QueriesRepository) Create(ctx context.Context, p db.CreateSubscriptionParams) (db.Subscription, error) {
	row, err := r.q.CreateSubscription(ctx, p)
	// No row means the saved search is missing or not the caller's (the INSERT ...
	// SELECT found nothing to insert).
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Subscription{}, ErrSavedSearchNotFound
	}
	if isUniqueViolation(err) {
		return db.Subscription{}, ErrDuplicate
	}
	if err != nil {
		return db.Subscription{}, err
	}
	return row, nil
}

func (r *QueriesRepository) SetActive(ctx context.Context, p db.SetSubscriptionActiveParams) (db.Subscription, error) {
	row, err := r.q.SetSubscriptionActive(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Subscription{}, ErrNotFound
	}
	if err != nil {
		return db.Subscription{}, err
	}
	return row, nil
}

func (r *QueriesRepository) Delete(ctx context.Context, p db.DeleteSubscriptionParams) error {
	affected, err := r.q.DeleteSubscription(ctx, p)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
