// Package subscription is the per-user filter-subscription use case: a signed-in
// user subscribes one of their saved searches to a delivery channel, lists their
// subscriptions, pauses/resumes one, or unsubscribes. It owns channel validation
// and maps the relevant Postgres conditions (unique violation, no row) onto
// package sentinels. The matching/delivery worker reads the same tables directly
// (internal/notify); this package is the HTTP-facing use case.
package subscription

import (
	"context"
	"errors"

	"github.com/strelov1/freehire/internal/db"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidChannel is an unsupported delivery channel (mapped to 400).
	ErrInvalidChannel = errors.New("subscription: unsupported channel")
	// ErrSavedSearchNotFound is a saved_search_id that is missing or not the
	// caller's (mapped to 404).
	ErrSavedSearchNotFound = errors.New("subscription: saved search not found")
	// ErrDuplicate is a second subscription for the same saved search and channel
	// (the UNIQUE (saved_search_id, channel) constraint; mapped to 409).
	ErrDuplicate = errors.New("subscription: already subscribed on this channel")
	// ErrNotFound is a missing or non-owned subscription (mapped to 404).
	ErrNotFound = errors.New("subscription: not found")
)

// ChannelTelegram is the only channel implemented today; the schema and this
// validation leave room for webhook/email without a migration.
const ChannelTelegram = "telegram"

// validChannels is the allowlist enforced on create.
var validChannels = map[string]bool{ChannelTelegram: true}

// Repository is the persistence contract, user-scoped. Create maps a unique
// violation to ErrDuplicate and a missing/non-owned saved search to
// ErrSavedSearchNotFound; SetActive maps a missing owner-scoped row to ErrNotFound;
// Delete maps "no row affected" to ErrNotFound.
type Repository interface {
	List(ctx context.Context, userID int64) ([]db.ListSubscriptionsRow, error)
	Create(ctx context.Context, p db.CreateSubscriptionParams) (db.Subscription, error)
	SetActive(ctx context.Context, p db.SetSubscriptionActiveParams) (db.Subscription, error)
	Delete(ctx context.Context, p db.DeleteSubscriptionParams) error
}

// Service implements the subscription use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns the user's subscriptions, newest first.
func (s *Service) List(ctx context.Context, userID int64) ([]db.ListSubscriptionsRow, error) {
	return s.repo.List(ctx, userID)
}

// Create subscribes one of the user's saved searches to a channel. The channel is
// validated against the allowlist; the destination is left NULL for telegram (the
// recipient is the user's linked chat). Ownership of the saved search is enforced
// in SQL (a non-owned id surfaces as ErrSavedSearchNotFound).
func (s *Service) Create(ctx context.Context, userID, savedSearchID int64, channel string) (db.Subscription, error) {
	if !validChannels[channel] {
		return db.Subscription{}, ErrInvalidChannel
	}
	return s.repo.Create(ctx, db.CreateSubscriptionParams{
		Channel:       channel,
		SavedSearchID: savedSearchID,
		UserID:        userID,
	})
}

// SetActive pauses or resumes a subscription, scoped to its owner.
func (s *Service) SetActive(ctx context.Context, userID, id int64, active bool) (db.Subscription, error) {
	return s.repo.SetActive(ctx, db.SetSubscriptionActiveParams{
		Active: active,
		ID:     id,
		UserID: userID,
	})
}

// Delete unsubscribes, scoped to its owner.
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, db.DeleteSubscriptionParams{ID: id, UserID: userID})
}
