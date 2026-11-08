package maildaily

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Memory is an in-process Store with the same semantics as the table: one claim per
// account per day, the first taker wins, and only the taker's source can release it. It is
// what the senders' unit tests embed in their fakes, so each states "this account's day
// is already taken" without a database.
type Memory struct {
	mu   sync.Mutex
	held map[int64]string
	// ClaimErr, when set, fails every claim; ReleaseErr every release.
	ClaimErr, ReleaseErr error
	// Releases counts the releases that reached the store, successful or not.
	Releases int
}

// Hold marks the account's day as taken by source, as another sender would have.
func (m *Memory) Hold(userID int64, source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held == nil {
		m.held = map[int64]string{}
	}
	m.held[userID] = source
}

// Holder is the source holding the account's day, or "" when it is free.
func (m *Memory) Holder(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.held[userID]
}

// ClaimMailDay implements Store.
func (m *Memory) ClaimMailDay(_ context.Context, arg db.ClaimMailDayParams) (pgtype.Date, error) {
	if m.ClaimErr != nil {
		return pgtype.Date{}, m.ClaimErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held == nil {
		m.held = map[int64]string{}
	}
	if _, taken := m.held[arg.UserID]; taken {
		return pgtype.Date{}, pgx.ErrNoRows
	}
	m.held[arg.UserID] = arg.Source
	return pgtype.Date{Valid: true}, nil
}

// ReleaseMailDay implements Store.
func (m *Memory) ReleaseMailDay(_ context.Context, arg db.ReleaseMailDayParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Releases++
	if m.ReleaseErr != nil {
		return m.ReleaseErr
	}
	if m.held[arg.UserID] == arg.Source {
		delete(m.held, arg.UserID)
	}
	return nil
}
