package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

// QueriesCodeStore is the production CodeStore backed by sqlc-generated *db.Queries.
// The table's composite primary key (user_id, purpose) is what enforces "at most one
// outstanding code"; this type just translates types and the no-row sentinel.
type QueriesCodeStore struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesCodeStore constructs a QueriesCodeStore.
func NewQueriesCodeStore(q *db.Queries, pool ...*pgxpool.Pool) *QueriesCodeStore {
	var p *pgxpool.Pool
	if len(pool) > 0 {
		p = pool[0]
	}
	return &QueriesCodeStore{q: q, pool: p}
}

// Compile-time assertion that QueriesCodeStore satisfies CodeStore.
var _ CodeStore = (*QueriesCodeStore)(nil)

// WithTx returns a transactional copy of QueriesCodeStore.
func (s *QueriesCodeStore) WithTx(tx pgx.Tx) CodeStore {
	if tx == nil {
		return s
	}
	return &QueriesCodeStore{
		q:    s.q.WithTx(tx),
		pool: s.pool,
	}
}

// Begin opens a new database transaction using the underlying pool.
func (s *QueriesCodeStore) Begin(ctx context.Context) (pgx.Tx, error) {
	if s.pool == nil {
		return nil, nil
	}
	return s.pool.Begin(ctx)
}

// UpsertCode issues or replaces the outstanding code for one purpose.
func (s *QueriesCodeStore) UpsertCode(ctx context.Context, userID int64, purpose, codeHash string, expiresAt time.Time) error {
	return s.q.UpsertEmailCode(ctx, db.UpsertEmailCodeParams{
		UserID:    userID,
		Purpose:   purpose,
		CodeHash:  codeHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// Code returns the outstanding code, or ErrNoCode when nothing is pending.
func (s *QueriesCodeStore) Code(ctx context.Context, userID int64, purpose string) (StoredCode, error) {
	row, err := s.q.GetEmailCode(ctx, db.GetEmailCodeParams{UserID: userID, Purpose: purpose})
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredCode{}, ErrNoCode
	}
	if err != nil {
		return StoredCode{}, err
	}
	return StoredCode{
		Hash:      row.CodeHash,
		ExpiresAt: row.ExpiresAt.Time,
		Attempts:  row.Attempts,
		IssuedAt:  row.CreatedAt.Time,
	}, nil
}

// GetEmailCodeForUpdate returns the outstanding code locked for update, or ErrNoCode when nothing is pending.
func (s *QueriesCodeStore) GetEmailCodeForUpdate(ctx context.Context, userID int64, purpose string) (StoredCode, error) {
	row, err := s.q.GetEmailCodeForUpdate(ctx, db.GetEmailCodeForUpdateParams{UserID: userID, Purpose: purpose})
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredCode{}, ErrNoCode
	}
	if err != nil {
		return StoredCode{}, err
	}
	return StoredCode{
		Hash:      row.CodeHash,
		ExpiresAt: row.ExpiresAt.Time,
		Attempts:  row.Attempts,
		IssuedAt:  row.CreatedAt.Time,
	}, nil
}

// BumpAttempts counts one failed guess and returns the new total.
func (s *QueriesCodeStore) BumpAttempts(ctx context.Context, userID int64, purpose string) (int32, error) {
	n, err := s.q.BumpEmailCodeAttempts(ctx, db.BumpEmailCodeAttemptsParams{UserID: userID, Purpose: purpose})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoCode
	}
	return n, err
}

// DeleteCode consumes or burns the outstanding code. Idempotent.
func (s *QueriesCodeStore) DeleteCode(ctx context.Context, userID int64, purpose string) error {
	return s.q.DeleteEmailCode(ctx, db.DeleteEmailCodeParams{UserID: userID, Purpose: purpose})
}
