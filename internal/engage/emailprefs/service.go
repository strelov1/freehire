package emailprefs

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the persistence the service needs. *db.Queries satisfies it directly.
type Store interface {
	GetEmailPrefs(ctx context.Context, userID int64) (db.GetEmailPrefsRow, error)
	ListUserEmailSubscriptions(ctx context.Context, userID int64) ([]db.ListUserEmailSubscriptionsRow, error)
	SetEmailGroupSwitches(ctx context.Context, arg db.SetEmailGroupSwitchesParams) error
	SetActivityEnabled(ctx context.Context, arg db.SetActivityEnabledParams) error
	DeactivateEmailSubscription(ctx context.Context, arg db.DeactivateEmailSubscriptionParams) (int64, error)
}

// Prefs is everything the public page may show. Deliberately small: the token that
// opens it is a bearer credential somebody could have forwarded, so it unlocks the
// address the mail already went to, three switches, and the names of the searches
// whose digests those switches govern — and nothing else about the account.
type Prefs struct {
	Email    string
	Alerts   bool
	Activity bool
	News     bool
	Searches []Search
}

// Search is one saved-search digest the account subscribes to by email.
type Search struct {
	ID     int64
	Name   string
	Active bool
}

// Update is what the page sends back. The group switches are required; the search
// ids are the ones to turn OFF.
//
// There is no "turn this search on" field, and that is the design. A link that can
// only ever subtract cannot be used to sign somebody up for anything, which is the
// right shape for a credential that never expires and travels in an email.
type Update struct {
	Alerts   bool
	Activity bool
	News     bool
	// DeactivateSearches names email subscriptions to switch off. Ids that are not
	// this account's, or are already off, are ignored rather than reported — telling
	// an unauthenticated caller which ids exist is the one thing this must not do.
	DeactivateSearches []int64
}

// Service reads and writes one account's email preferences from a signed link.
type Service struct {
	store  Store
	signer *Signer
}

// NewService builds the service. secret is the session-signing secret; the salt in
// NewSigner is what keeps this key apart from it.
func NewService(store Store, secret string) *Service {
	return &Service{store: store, signer: NewSigner(secret)}
}

// resolve turns a token into the account it names, refusing one whose account no
// longer exists. Parse proves only that we minted the token — there is no database
// in the signer — so the lookup happens here, and it fails with the same sentinel as
// a forgery, because an unauthenticated caller must not learn which ids are real.
func (s *Service) resolve(ctx context.Context, token string) (int64, Group, Prefs, error) {
	userID, group, err := s.signer.Parse(token)
	if err != nil {
		return 0, "", Prefs{}, err
	}
	row, err := s.store.GetEmailPrefs(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", Prefs{}, fmt.Errorf("%w: no such account", ErrInvalidToken)
	}
	if err != nil {
		return 0, "", Prefs{}, fmt.Errorf("emailprefs: reading preferences: %w", err)
	}
	return userID, group, Prefs{
		Email:    row.Email,
		Alerts:   row.AlertsEnabled,
		Activity: row.ActivityEnabled,
		News:     row.NewsEnabled,
	}, nil
}

// Load returns what the page shows for one token.
func (s *Service) Load(ctx context.Context, token string) (Prefs, error) {
	userID, _, prefs, err := s.resolve(ctx, token)
	if err != nil {
		return Prefs{}, err
	}
	return s.withSearches(ctx, userID, prefs)
}

// LoadFor is Load for a caller the session already identified, so the signed-in
// settings page shows exactly what the emailed link would.
//
// The two entry points differ only in how the user is established. Sharing the rest
// is what stops the two views drifting into disagreeing about the same three
// booleans — which, on a preference page, reads as the product losing a setting.
func (s *Service) LoadFor(ctx context.Context, userID int64) (Prefs, error) {
	row, err := s.store.GetEmailPrefs(ctx, userID)
	if err != nil {
		return Prefs{}, fmt.Errorf("emailprefs: reading preferences: %w", err)
	}
	return s.withSearches(ctx, userID, Prefs{
		Email:    row.Email,
		Alerts:   row.AlertsEnabled,
		Activity: row.ActivityEnabled,
		News:     row.NewsEnabled,
	})
}

// SaveFor is Save for a caller the session already identified.
func (s *Service) SaveFor(ctx context.Context, userID int64, in Update) error {
	return s.apply(ctx, userID, in)
}

func (s *Service) withSearches(ctx context.Context, userID int64, prefs Prefs) (Prefs, error) {
	rows, err := s.store.ListUserEmailSubscriptions(ctx, userID)
	if err != nil {
		return Prefs{}, fmt.Errorf("emailprefs: reading subscriptions: %w", err)
	}
	for _, r := range rows {
		prefs.Searches = append(prefs.Searches, Search{ID: r.ID, Name: r.Name, Active: r.Active})
	}
	return prefs, nil
}

// Save applies what somebody set on the page.
//
// The two writes are deliberately separate statements rather than one: the activity
// switch and the other two are written by different callers with different scopes —
// the authenticated settings page owns the whole rule, this page owns three
// booleans — and a single full-replace writer would let each clobber the other.
func (s *Service) Save(ctx context.Context, token string, in Update) error {
	userID, _, _, err := s.resolve(ctx, token)
	if err != nil {
		return err
	}
	return s.apply(ctx, userID, in)
}

// apply is the write both entry points share.
func (s *Service) apply(ctx context.Context, userID int64, in Update) error {
	if err := s.store.SetEmailGroupSwitches(ctx, db.SetEmailGroupSwitchesParams{
		UserID:             userID,
		AlertsEmailEnabled: in.Alerts,
		NewsEmailEnabled:   in.News,
	}); err != nil {
		return fmt.Errorf("emailprefs: writing the group switches: %w", err)
	}
	if err := s.store.SetActivityEnabled(ctx, db.SetActivityEnabledParams{
		UserID: userID, Enabled: in.Activity,
	}); err != nil {
		return fmt.Errorf("emailprefs: writing the activity switch: %w", err)
	}
	for _, id := range in.DeactivateSearches {
		// Owner-scoped and deactivate-only in SQL, so an id belonging to somebody
		// else simply affects no rows. The count is discarded on purpose: reporting
		// "that one was not yours" would answer a question this caller may not ask.
		if _, err := s.store.DeactivateEmailSubscription(ctx, db.DeactivateEmailSubscriptionParams{
			ID: id, UserID: userID,
		}); err != nil {
			return fmt.Errorf("emailprefs: turning off subscription %d: %w", id, err)
		}
	}
	return nil
}

// OneClick is the RFC 8058 target: it turns off exactly the group the token names
// and returns which one, so the response can say what it did.
//
// Scoped to one group rather than everything, because a mail client's own
// unsubscribe button acts with no confirmation screen — a tap on one campaign must
// not silence the job alerts somebody actually wants. The confirmation page offers
// the rest.
//
// Idempotent: turning off a group that is already off writes the same row again and
// succeeds, so a client that retries reports success rather than an error.
func (s *Service) OneClick(ctx context.Context, token string) (Group, error) {
	userID, group, prefs, err := s.resolve(ctx, token)
	if err != nil {
		return "", err
	}
	switch group {
	case GroupActivity:
		err = s.store.SetActivityEnabled(ctx, db.SetActivityEnabledParams{UserID: userID, Enabled: false})
	case GroupAlerts:
		err = s.store.SetEmailGroupSwitches(ctx, db.SetEmailGroupSwitchesParams{
			UserID: userID, AlertsEmailEnabled: false, NewsEmailEnabled: prefs.News,
		})
	case GroupNews:
		err = s.store.SetEmailGroupSwitches(ctx, db.SetEmailGroupSwitchesParams{
			UserID: userID, AlertsEmailEnabled: prefs.Alerts, NewsEmailEnabled: false,
		})
	default:
		// Unreachable: Parse refuses any group that is not silenceable. Handled
		// rather than ignored so a fourth group added later fails loudly here
		// instead of silently turning nothing off.
		return "", fmt.Errorf("%w: group %q has no switch", ErrInvalidToken, group)
	}
	if err != nil {
		return "", fmt.Errorf("emailprefs: turning off %s: %w", group, err)
	}
	return group, nil
}
