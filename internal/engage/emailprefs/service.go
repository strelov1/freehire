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

// Prefs is everything the public page may show. Deliberately small: the token is a
// bearer credential that may have been forwarded, so it exposes only the mail
// address, three switches and the names of the governed searches.
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
// There is deliberately no "turn this search on" field: a never-expiring emailed
// credential must only ever subtract.
type Update struct {
	Alerts   bool
	Activity bool
	News     bool
	// DeactivateSearches names email subscriptions to switch off. Ids that are not
	// this account's, or already off, are ignored, never reported, so an
	// unauthenticated caller cannot learn which ids exist.
	DeactivateSearches []int64
}

// Service reads and writes one account's email preferences from a signed link.
type Service struct {
	store  Store
	signer *Signer
}

// NewService builds the service. secret is the session-signing secret.
func NewService(store Store, secret string) *Service {
	return &Service{store: store, signer: NewSigner(secret)}
}

// resolve turns a token into the account it names. Parse proves only that we minted
// the token, so the lookup happens here; a missing account fails with the same
// sentinel as a forgery so a caller cannot learn which ids are real.
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

// LoadFor is Load for a caller the session already identified. The two entry points
// differ only in how the user is established, so the two views cannot disagree.
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

// Silenced reports whether this account has turned g off.
//
// For the two mails with no selection query to gate (a report outcome and a referral
// ping, sent straight from a request path). Queue-driven mail is gated in SQL
// instead, so a later delivery path inherits it.
//
// A read failure returns false: failing open is right for a notice somebody is
// waiting on, and the worst case is one unwanted mail whose footer link stops the rest.
func (s *Service) Silenced(ctx context.Context, userID int64, g Group) bool {
	row, err := s.store.GetEmailPrefs(ctx, userID)
	if err != nil {
		return false
	}
	switch g {
	case GroupAlerts:
		return !row.AlertsEnabled
	case GroupActivity:
		return !row.ActivityEnabled
	case GroupNews:
		return !row.NewsEnabled
	default:
		return false
	}
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
// The two writes are separate statements because the activity switch and the other
// two have different owners (the authenticated settings page owns the whole rule),
// and a single full-replace writer would let each clobber the other.
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
		// Owner-scoped and deactivate-only in SQL, so a foreign id affects no rows.
		// The count is discarded on purpose: reporting it would reveal ownership.
		if _, err := s.store.DeactivateEmailSubscription(ctx, db.DeactivateEmailSubscriptionParams{
			ID: id, UserID: userID,
		}); err != nil {
			return fmt.Errorf("emailprefs: turning off subscription %d: %w", id, err)
		}
	}
	return nil
}

// OneClick is the RFC 8058 target: it turns off exactly the group the token names
// and returns which one.
//
// Scoped to one group because a mail client's unsubscribe button acts with no
// confirmation screen; the confirmation page offers the rest. Idempotent, so a
// retrying client sees success.
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
		// Unreachable (Parse refuses non-silenceable groups); fails loudly so a new
		// group cannot silently turn nothing off.
		return "", fmt.Errorf("%w: group %q has no switch", ErrInvalidToken, group)
	}
	if err != nil {
		return "", fmt.Errorf("emailprefs: turning off %s: %w", group, err)
	}
	return group, nil
}
