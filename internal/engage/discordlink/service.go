package discordlink

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
)

var (
	// ErrNotLinked means the account has no binding. A state, not a failure: every caller
	// reports it as "not connected" rather than as an error.
	ErrNotLinked = errors.New("discordlink: account is not linked")

	// ErrAlreadyLinkedElsewhere means another freehire account already holds that Discord
	// account. Refused rather than reassigned: reassigning would silently take a paying
	// subscriber's access away and hand it to whoever asked last.
	ErrAlreadyLinkedElsewhere = errors.New("discordlink: that Discord account is linked to another freehire account")
)

// Candidate is one binding as reconciliation sees it: who it is on both sides, what we
// believe about the role, and the entitlement columns that decide it.
//
// It carries the raw instants rather than a resolved tier because resolving one is a
// DECISION and this is a row. The service resolves it against its own clock, so a test can
// move time without moving the database.
type Candidate struct {
	UserID        int64
	DiscordUserID string
	RoleGranted   bool
	ProUntil      time.Time
	UltraUntil    time.Time
}

// Store is the persistence this service needs. Narrow on purpose: it is the consumer that
// declares it, so the sqlc-backed implementation can grow without widening what the service
// is allowed to do.
type Store interface {
	Link(ctx context.Context, userID int64, discordUserID string) (Link, error)
	Get(ctx context.Context, userID int64) (Link, error)
	Unlink(ctx context.Context, userID int64) (bool, error)
	Plan(ctx context.Context, userID int64) (proUntil, ultraUntil time.Time, err error)
	ListToSync(ctx context.Context, limit int32) ([]Candidate, error)
	SetRoleGranted(ctx context.Context, userID int64, granted bool) error
}

// Discord is the part of the REST API this service uses. *Client implements it.
type Discord interface {
	ExchangeCode(ctx context.Context, code, redirectURI string) (string, error)
	CurrentUserID(ctx context.Context, accessToken string) (string, error)
	AddGuildMember(ctx context.Context, discordUserID, accessToken string) error
	GrantPaidRole(ctx context.Context, discordUserID string) error
	RevokePaidRole(ctx context.Context, discordUserID string) error
}

// Service is the use case: link an account, unlink it, and keep the role in step.
type Service struct {
	store   Store
	discord Discord
	now     func() time.Time
}

// NewService builds the service. now is injected so the tier boundary can be tested without
// waiting for a subscription to lapse.
func NewService(store Store, discord Discord, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, discord: discord, now: now}
}

// NewFromSettings builds the production service, reporting false when the feature is not
// configured. Both entrypoints — the server's route wiring and cmd/discord-sync — go through
// it, so they cannot drift apart about which credentials the client gets or which clock it
// reads.
//
// It returns a CONCRETE *Service and a separate bool rather than a nil interface, because a
// nil *Service assigned into an interface field is not a nil interface: the caller that
// stores this behind one has to see the bool to know there is nothing there.
func NewFromSettings(cfg config.Settings, q *db.Queries) (*Service, bool) {
	if !cfg.DiscordPaidAccessConfigured() {
		return nil, false
	}
	return NewService(
		NewPostgresStore(q),
		NewClient(ClientConfig{
			ClientID:     cfg.DiscordClientID,
			ClientSecret: cfg.DiscordClientSecret,
			BotToken:     cfg.DiscordBotToken,
			GuildID:      cfg.DiscordGuildID,
			PaidRoleID:   cfg.DiscordPaidRoleID,
		}),
		time.Now,
	), true
}

// Status reports the account's binding, or ErrNotLinked.
func (s *Service) Status(ctx context.Context, userID int64) (Link, error) {
	return s.store.Get(ctx, userID)
}

// Link completes the OAuth flow: exchange the code, learn who they are on Discord, record
// the binding, put them on the server, and grant the role if they pay.
//
// The binding is recorded BEFORE anything is done on Discord. A process that dies in the
// middle then leaves a link that the next reconciliation finishes, rather than a role or a
// membership that nothing in our database knows about.
func (s *Service) Link(ctx context.Context, userID int64, code, redirectURI string) (Link, error) {
	token, err := s.discord.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		return Link{}, fmt.Errorf("exchange code: %w", err)
	}
	discordUserID, err := s.discord.CurrentUserID(ctx, token)
	if err != nil {
		return Link{}, fmt.Errorf("read discord identity: %w", err)
	}

	link, err := s.store.Link(ctx, userID, discordUserID)
	if err != nil {
		return Link{}, err
	}

	if err := s.discord.AddGuildMember(ctx, discordUserID, token); err != nil {
		return Link{}, fmt.Errorf("join guild: %w", err)
	}

	proUntil, ultraUntil, err := s.store.Plan(ctx, userID)
	if err != nil {
		return Link{}, fmt.Errorf("read plan: %w", err)
	}
	if !WarrantsPaidRole(plan.TierOf(proUntil, ultraUntil, s.now())) {
		return link, nil
	}
	if err := s.discord.GrantPaidRole(ctx, discordUserID); err != nil {
		return Link{}, fmt.Errorf("grant role: %w", err)
	}
	if err := s.store.SetRoleGranted(ctx, userID, true); err != nil {
		return Link{}, fmt.Errorf("record grant: %w", err)
	}
	link.RoleGranted = true
	return link, nil
}

// Unlink revokes the role and removes the binding.
//
// The binding goes even when the revoke fails, and that order is the point: a user must
// always be able to undo a link they made. An orphaned role is an operator's problem, found
// in the log; a link that cannot be removed is the user's problem, every time they open the
// page. Membership of the server is left alone — we invited them, and eviction is a
// moderation act rather than a billing one.
func (s *Service) Unlink(ctx context.Context, userID int64) error {
	link, err := s.store.Get(ctx, userID)
	if errors.Is(err, ErrNotLinked) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := s.discord.RevokePaidRole(ctx, link.DiscordUserID); err != nil &&
		!errors.Is(err, ErrUnknownMember) {
		log.Printf("discordlink: revoking the role for user %d failed, unlinking anyway: %v", userID, err)
	}
	if _, err := s.store.Unlink(ctx, userID); err != nil {
		return err
	}
	return nil
}

// Stats is what one reconciliation run did, for the worker's log line.
type Stats struct {
	Examined int
	Granted  int
	Revoked  int
	Failed   int
}

// Sync reconciles up to limit bindings.
//
// A per-account failure is counted and stepped over, never returned: one broken account must
// not cost everybody else their hourly reconciliation. Only a failure to READ the page — we
// cannot tell what to do at all — aborts.
func (s *Service) Sync(ctx context.Context, limit int32) (Stats, error) {
	candidates, err := s.store.ListToSync(ctx, limit)
	if err != nil {
		return Stats{}, fmt.Errorf("list bindings: %w", err)
	}

	var stats Stats
	now := s.now()
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.Examined++

		action := Reconcile(plan.TierOf(c.ProUntil, c.UltraUntil, now), c.RoleGranted)
		granted, err := s.moveRole(ctx, c, action)
		failed := err != nil
		if failed {
			log.Printf("discordlink: %s for user %d failed: %v", action, c.UserID, err)
		}

		// EVERY path stamps the row, including the one that did nothing and the one that
		// failed: the stamp is what moves a binding to the back of the reconciliation queue,
		// and a row left unstamped pins the front of it and starves everybody behind the
		// per-run bound. What differs is the value — a failure records the row as it stood,
		// so the next run tries again.
		if err := s.store.SetRoleGranted(ctx, c.UserID, granted); err != nil {
			log.Printf("discordlink: stamping user %d failed: %v", c.UserID, err)
			failed = true
		}

		switch {
		case failed:
			// At most one per candidate, however many of its steps went wrong.
			stats.Failed++
		case action == ActionGrant && granted:
			stats.Granted++
		case action == ActionRevoke:
			stats.Revoked++
		}
	}
	return stats, nil
}

// moveRole performs one action on Discord and reports what the stored record should now say.
//
// ErrUnknownMember is not a failure: the person has left the server, so the role is not held,
// which is exactly what recording "not granted" says. Any other error leaves the record as it
// stood, so the next run tries the same thing again.
func (s *Service) moveRole(ctx context.Context, c Candidate, action Action) (bool, error) {
	var err error
	switch action {
	case ActionGrant:
		err = s.discord.GrantPaidRole(ctx, c.DiscordUserID)
	case ActionRevoke:
		err = s.discord.RevokePaidRole(ctx, c.DiscordUserID)
	case ActionNone:
		return c.RoleGranted, nil
	}

	switch {
	case errors.Is(err, ErrUnknownMember):
		return false, nil
	case err != nil:
		return c.RoleGranted, err
	default:
		return action == ActionGrant, nil
	}
}
