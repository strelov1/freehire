// Package subscription is the per-user filter-subscription use case: a signed-in
// user subscribes one of their saved searches to a delivery channel, lists their
// subscriptions, pauses/resumes one, or unsubscribes. It owns channel validation
// and maps the relevant Postgres conditions (unique violation, no row) onto
// package sentinels. The matching/delivery worker reads the same tables directly
// (internal/engage/notify); this package is the HTTP-facing use case.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/search/search"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidChannel is an unsupported delivery channel (mapped to 400).
	ErrInvalidChannel = errors.New("subscription: unsupported channel")
	// ErrUnfilteredSearch is a subscription to a saved search that carries no filter (mapped to
	// 400). Saving an unfiltered board is fine, but subscribing to one means an alert for every
	// posting in the catalogue, which floods the delivery queue.
	//
	// "No filter" is decided by search.Narrows, not by a blank query string: a retired facet or
	// mistyped param is non-empty yet reads as no filter to the matcher.
	ErrUnfilteredSearch = errors.New("subscription: saved search has no filter")
	// ErrNoPushDevice is a push subscription for an account with no registered device (mapped to
	// 409). Such a subscription only queues matches that can never leave. Registering a device
	// is the mobile app's job.
	ErrNoPushDevice = errors.New("subscription: no registered push device")
	// ErrSavedSearchNotFound is a saved_search_id that is missing or not the
	// caller's (mapped to 404).
	ErrSavedSearchNotFound = errors.New("subscription: saved search not found")
	// ErrDuplicate is a second subscription for the same saved search and channel
	// (the UNIQUE (saved_search_id, channel) constraint; mapped to 409).
	ErrDuplicate = errors.New("subscription: already subscribed on this channel")
	// ErrNotFound is a missing or non-owned subscription (mapped to 404).
	ErrNotFound = errors.New("subscription: not found")
)

// Supported delivery channels. The values live in internal/engage/notify (the vocabulary shared
// with the routing worker); these aliases keep them addressable here.
const (
	ChannelTelegram = notify.ChannelTelegram
	ChannelEmail    = notify.ChannelEmail
)

// Subscription is a stored filter subscription, decoupled from the generated db row. Internal
// columns (user_id, destination, start_at) are dropped as they are never on the wire.
type Subscription struct {
	ID            int64
	SavedSearchID int64
	Channel       string
	Active        bool
	CreatedAt     *time.Time
}

// SubscriptionListItem is a subscription joined to its saved search's name (to label each
// toggle) and stored query (for surfaces that cannot render the filter panel, like MCP).
type SubscriptionListItem struct {
	Subscription
	SavedSearchName  string
	SavedSearchQuery string
}

// Repository is the persistence contract, user-scoped. Create maps a unique violation to
// ErrDuplicate and a missing/non-owned saved search to ErrSavedSearchNotFound; SetActive and
// Delete map a missing owner-scoped row to ErrNotFound.
type Repository interface {
	List(ctx context.Context, userID int64) ([]SubscriptionListItem, error)
	Create(ctx context.Context, userID, savedSearchID int64, channel string) (Subscription, error)
	// SavedSearchQuery returns the owner-scoped saved search's stored query, so an unfiltered
	// one can be refused. A missing or non-owned id is ErrSavedSearchNotFound, like Create.
	SavedSearchQuery(ctx context.Context, userID, savedSearchID int64) (string, error)
	// HasPushDevice reports whether the account has a registered push device, required for the
	// push channel.
	HasPushDevice(ctx context.Context, userID int64) (bool, error)
	// Target returns an owner-scoped subscription's channel and saved-search query, so a
	// resume can re-apply the gates. A missing or non-owned id surfaces as ErrNotFound.
	Target(ctx context.Context, userID, id int64) (channel, query string, err error)
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

// Create subscribes one of the user's saved searches to a channel. The channel is validated
// against the allowlist; the destination stays NULL (the recipient is resolved at delivery).
// Ownership of the saved search is enforced in SQL.
//
// The saved search is read the way the delivery worker reads it (internal/engage/notify's
// matchQuery), so "narrows nothing" is judged by the vocabulary that will actually run. The
// gate lives here, not in that worker, so that refusing at delivery never drops mail people
// already receive.
func (s *Service) Create(ctx context.Context, userID, savedSearchID int64, channel string) (Subscription, error) {
	if !notify.ValidChannel(channel) {
		return Subscription{}, ErrInvalidChannel
	}
	query, err := s.repo.SavedSearchQuery(ctx, userID, savedSearchID)
	if err != nil {
		return Subscription{}, err
	}
	if err := s.admit(ctx, userID, query, channel); err != nil {
		return Subscription{}, err
	}
	return s.repo.Create(ctx, userID, savedSearchID, channel)
}

// admit applies the gates a subscription must pass to be active: a query the matcher can run
// and that narrows the catalogue, and a channel that can reach the account. Create and resume
// share it.
func (s *Service) admit(ctx context.Context, userID int64, query, channel string) error {
	// The parse error is dropped as the matcher drops it: ParseQuery still returns every pair it
	// could read, and a query nothing survives is refused by Narrows.
	vals, _ := url.ParseQuery(query)
	// A saved search may predate syntax validation; refuse what the matcher would reject.
	if _, err := search.ParseQuery(vals.Get("q")); err != nil {
		return err
	}
	if search.HasInvalidVisaSponsorship(vals) {
		return fmt.Errorf("%w: visa_sponsorship must be true or false", search.ErrBadQuery)
	}
	if !search.Narrows(vals) {
		return ErrUnfilteredSearch
	}
	if channel == notify.ChannelPush {
		ok, err := s.repo.HasPushDevice(ctx, userID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNoPushDevice
		}
	}
	return nil
}

// SetActive pauses or resumes a subscription, scoped to its owner. A resume passes the same
// gates as a new subscription, since the worker pauses one whose query it cannot run.
func (s *Service) SetActive(ctx context.Context, userID, id int64, active bool) (Subscription, error) {
	if active {
		channel, query, err := s.repo.Target(ctx, userID, id)
		if err != nil {
			return Subscription{}, err
		}
		if err := s.admit(ctx, userID, query, channel); err != nil {
			return Subscription{}, err
		}
	}
	return s.repo.SetActive(ctx, userID, id, active)
}

// Delete unsubscribes, scoped to its owner.
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}
