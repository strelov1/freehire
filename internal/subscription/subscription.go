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
	"time"

	"github.com/strelov1/freehire/internal/notify"
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

// Supported delivery channels. The values live once in internal/notify (the
// delivery-channel vocabulary shared with the routing worker); these aliases keep
// them addressable from the HTTP-facing use case. The schema (channel + destination
// columns and the UNIQUE (saved_search, channel) constraint) accommodates both
// without a migration; the notify worker routes each channel to its Notifier.
const (
	ChannelTelegram = notify.ChannelTelegram
	ChannelEmail    = notify.ChannelEmail
)

// validChannels is the create-time allowlist, derived from the notify
// delivery-channel vocabulary so the two never drift.
var validChannels = func() map[string]bool {
	m := make(map[string]bool, len(notify.Channels))
	for _, c := range notify.Channels {
		m[c] = true
	}
	return m
}()

// Subscription is a stored filter subscription: the package domain type, decoupled from
// the generated db row. The internal columns (user_id, the destination and start_at
// cursor) are dropped — they are never on the wire — while created_at is kept as *time.Time
// because the handler serializes it.
type Subscription struct {
	ID            int64
	SavedSearchID int64
	Channel       string
	Active        bool
	CreatedAt     *time.Time
}

// SubscriptionListItem is a subscription joined to its saved search's display name, so the
// "My subscriptions" view can label each toggle.
type SubscriptionListItem struct {
	Subscription
	SavedSearchName string
}

// Repository is the persistence contract, user-scoped. Create maps a unique
// violation to ErrDuplicate and a missing/non-owned saved search to
// ErrSavedSearchNotFound; SetActive maps a missing owner-scoped row to ErrNotFound;
// Delete maps "no row affected" to ErrNotFound. Implementations map the generated db
// rows to Subscription/SubscriptionListItem, so the use case never sees db.*.
type Repository interface {
	List(ctx context.Context, userID int64) ([]SubscriptionListItem, error)
	Create(ctx context.Context, userID, savedSearchID int64, channel string) (Subscription, error)
	SetActive(ctx context.Context, userID, id int64, active bool) (Subscription, error)
	Delete(ctx context.Context, userID, id int64) error
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
func (s *Service) List(ctx context.Context, userID int64) ([]SubscriptionListItem, error) {
	return s.repo.List(ctx, userID)
}

// Create subscribes one of the user's saved searches to a channel. The channel is
// validated against the allowlist; the destination is left NULL for both channels
// (the recipient is resolved live at delivery — the linked chat for telegram, the
// account email for email). Ownership of the saved search is enforced in SQL (a
// non-owned id surfaces as ErrSavedSearchNotFound).
func (s *Service) Create(ctx context.Context, userID, savedSearchID int64, channel string) (Subscription, error) {
	if !validChannels[channel] {
		return Subscription{}, ErrInvalidChannel
	}
	return s.repo.Create(ctx, userID, savedSearchID, channel)
}

// SetActive pauses or resumes a subscription, scoped to its owner.
func (s *Service) SetActive(ctx context.Context, userID, id int64, active bool) (Subscription, error) {
	return s.repo.SetActive(ctx, userID, id, active)
}

// Delete unsubscribes, scoped to its owner.
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}
