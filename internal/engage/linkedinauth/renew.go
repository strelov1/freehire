package linkedinauth

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// How much of a credential's life is left when this package acts on it.
//
// Both windows are two weeks so an operator gets the same notice on either path: wide
// enough that a fortnight of failed daily runs still leaves a working token, and to
// survive somebody's holiday.
const (
	// RenewBefore is how close to expiry a renewable token is exchanged for a fresh one.
	RenewBefore = 14 * 24 * time.Hour

	// WarnBefore is how close to expiry a token that CANNOT be renewed starts asking for a
	// person. It also covers the refresh token's own expiry, which no renewal extends.
	WarnBefore = 14 * 24 * time.Hour
)

// State is what one renewal run concluded. It is returned, not logged, because the caller
// decides the exit code and whether a human is told, and those differ per state.
type State string

const (
	// StateMissing: no credential is stored (never signed in, or turned off). Not an error.
	StateMissing State = "missing"

	// StateHealthy: nothing to do today.
	StateHealthy State = "healthy"

	// StateRenewed: the access token was exchanged for a fresh one and stored.
	StateRenewed State = "renewed"

	// StateWarn: the credential dies soon and this worker cannot prevent it (no refresh
	// token, or the refresh token is running out). A person must sign in again. Reported
	// while the channel still WORKS, since after expiry it only shows as a stopped digest.
	StateWarn State = "warn"

	// StateExpired: the credential is already dead and could not be renewed. The channel is
	// broken now, not soon.
	StateExpired State = "expired"
)

// Outcome is one run's conclusion, with what a log line or warning needs.
type Outcome struct {
	State State

	// ExpiresAt is the access token's expiry AFTER this run. Zero when there is no credential.
	ExpiresAt time.Time

	// RefreshExpiresAt is when the grant runs out and a person must sign in regardless.
	// Zero when there is no refresh token.
	RefreshExpiresAt time.Time

	// Reason is the human-readable half of a warning: why, and what to do. Empty when
	// nothing is asked of anybody.
	Reason string
}

// Store is the persistence Renewer needs, an interface so the decision table can be
// tested against a fake.
type Store interface {
	// Load returns the stored credential. The bool is false when never signed in.
	Load(ctx context.Context) (Token, bool, error)

	// Save records a credential a person has just granted.
	Save(ctx context.Context, t Token) error

	// Renew writes a renewed credential, replacing the one whose access token is previous.
	// It reports false when nothing matched: a sign-in landed mid-renewal and its newer
	// token must not be overwritten.
	Renew(ctx context.Context, previous string, t Token) (bool, error)
}

// Renewer keeps one channel's credential alive, and says so in time when it cannot.
type Renewer struct {
	creds Credentials
	store Store
	httpc *http.Client
	now   func() time.Time
}

// NewRenewer builds a Renewer. nowFn is injectable (nil means time.Now).
func NewRenewer(creds Credentials, store Store, httpc *http.Client, nowFn func() time.Time) *Renewer {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Renewer{creds: creds, store: store, httpc: httpc, now: nowFn}
}

// Run examines the stored credential and renews it if it can.
//
// The branches are in order of urgency and each returns rather than falling through: an
// expired token is not also "expiring soon", and a renewed one is not re-checked against
// its old expiry.
func (r *Renewer) Run(ctx context.Context) (Outcome, error) {
	tok, ok, err := r.store.Load(ctx)
	if err != nil {
		return Outcome{}, fmt.Errorf("load %s credential: %w", Channel, err)
	}
	if !ok {
		return Outcome{State: StateMissing}, nil
	}

	now := r.now().UTC()
	left := tok.ExpiresAt.Sub(now)

	switch {
	case left <= 0 && !tok.Renewable():
		return Outcome{
			State:     StateExpired,
			ExpiresAt: tok.ExpiresAt,
			Reason: fmt.Sprintf("the access token expired on %s and there is no refresh token; "+
				"the channel is not publishing until somebody signs in again", tok.ExpiresAt.Format(time.RFC3339)),
		}, nil

	case left <= RenewBefore && tok.Renewable():
		renewed, err := r.renew(ctx, tok)
		if err != nil {
			// Reported as a warning, not swallowed: the days left are when a person can act.
			return Outcome{
				State:            StateWarn,
				ExpiresAt:        tok.ExpiresAt,
				RefreshExpiresAt: tok.RefreshExpiresAt,
				Reason: fmt.Sprintf("renewal failed and the access token expires %s: %v",
					tok.ExpiresAt.Format(time.RFC3339), err),
			}, nil
		}
		return renewed, nil

	case left <= WarnBefore:
		return Outcome{
			State:            StateWarn,
			ExpiresAt:        tok.ExpiresAt,
			RefreshExpiresAt: tok.RefreshExpiresAt,
			Reason: fmt.Sprintf("the access token expires %s and LinkedIn issued no refresh token, "+
				"so it cannot be renewed automatically — sign in again with cmd/linkedin-auth",
				tok.ExpiresAt.Format(time.RFC3339)),
		}, nil

	// Checked after the access token: a refresh token in its last fortnight still renews,
	// so this is a reminder, not a broken channel.
	case tok.Renewable() && !tok.RefreshExpiresAt.IsZero() && tok.RefreshExpiresAt.Sub(now) <= WarnBefore:
		return Outcome{
			State:            StateWarn,
			ExpiresAt:        tok.ExpiresAt,
			RefreshExpiresAt: tok.RefreshExpiresAt,
			Reason: fmt.Sprintf("the refresh token expires %s and no renewal extends it — "+
				"sign in again with cmd/linkedin-auth before then",
				tok.RefreshExpiresAt.Format(time.RFC3339)),
		}, nil
	}

	return Outcome{
		State:            StateHealthy,
		ExpiresAt:        tok.ExpiresAt,
		RefreshExpiresAt: tok.RefreshExpiresAt,
	}, nil
}

// renew performs the exchange and stores the result.
func (r *Renewer) renew(ctx context.Context, tok Token) (Outcome, error) {
	fresh, err := r.creds.Refresh(ctx, r.httpc, tok.RefreshToken)
	if err != nil {
		return Outcome{}, err
	}

	// Carry the old refresh token forward if a response omits it, so a renewable
	// credential is not silently stored without one; a stale token fails loudly next time.
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = tok.RefreshToken
		fresh.RefreshExpiresAt = tok.RefreshExpiresAt
	}

	stored, err := r.store.Renew(ctx, tok.AccessToken, fresh)
	if err != nil {
		return Outcome{}, fmt.Errorf("store renewed credential: %w", err)
	}
	if !stored {
		// Somebody signed in mid-flight; their token is newer, so leave it alone.
		return Outcome{State: StateHealthy, ExpiresAt: tok.ExpiresAt, RefreshExpiresAt: tok.RefreshExpiresAt}, nil
	}
	return Outcome{
		State:            StateRenewed,
		ExpiresAt:        fresh.ExpiresAt,
		RefreshExpiresAt: fresh.RefreshExpiresAt,
	}, nil
}
