package linkedinauth

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// How much of a credential's life is left when this package acts on it.
//
// Both windows are two weeks, and the same number for two different jobs on purpose: whichever
// path a deployment ends up on, an operator has the same fortnight of notice. The renewal
// window has to be wide enough that a fortnight of failed daily runs — LinkedIn down, the host
// down, the timer disabled — still leaves a working token, and the warning window has to be
// wide enough to survive somebody's holiday.
const (
	// RenewBefore is how close to expiry a renewable token is exchanged for a fresh one.
	RenewBefore = 14 * 24 * time.Hour

	// WarnBefore is how close to expiry a token that CANNOT be renewed starts asking for a
	// person. It also covers the refresh token's own yearly expiry, which no renewal extends.
	WarnBefore = 14 * 24 * time.Hour
)

// State is what one renewal run concluded. It is returned rather than logged inside, because
// the caller decides both the exit code and whether a human is told — and those two decisions
// differ between the states in ways a log line cannot express.
type State string

const (
	// StateMissing: no credential is stored. The channel has never been signed in, which is
	// how this feature ships and what turning it off looks like. Not an error.
	StateMissing State = "missing"

	// StateHealthy: nothing to do today.
	StateHealthy State = "healthy"

	// StateRenewed: the access token was exchanged for a fresh one and stored.
	StateRenewed State = "renewed"

	// StateWarn: the credential dies soon and this worker cannot prevent it — either LinkedIn
	// issued no refresh token, or the refresh token itself is running out. A person must sign
	// in again. Reported loudly while the channel is still WORKING, which is the whole point:
	// after expiry the same fact is only visible as a digest that stopped.
	StateWarn State = "warn"

	// StateExpired: the credential is already dead and could not be renewed. The channel is
	// broken now, not soon.
	StateExpired State = "expired"
)

// Outcome is one run's conclusion, with everything a log line or a warning message needs so
// the caller does not have to re-read the token to describe it.
type Outcome struct {
	State State

	// ExpiresAt is the access token's expiry AFTER this run — the renewed one when a renewal
	// happened, the stored one otherwise. Zero when there is no credential at all.
	ExpiresAt time.Time

	// RefreshExpiresAt is when the grant behind it runs out and a person must sign in
	// regardless. Zero when there is no refresh token, which is also the case where every
	// expiry is a sign-in.
	RefreshExpiresAt time.Time

	// Reason is the human-readable half of a warning: why this state, and what to do. Empty
	// for the states that ask nothing of anybody.
	Reason string
}

// Store is the persistence Renewer needs. An interface so the decision table below is testable
// against a fake — it is a rule about time and expiry, and a rule about time that can only be
// exercised against a live database is a rule nobody re-reads.
type Store interface {
	// Load returns the stored credential. The bool is false when the channel has never been
	// signed in, which is a state and not an error.
	Load(ctx context.Context) (Token, bool, error)

	// Save records a credential a person has just granted.
	Save(ctx context.Context, t Token) error

	// Renew writes a renewed credential, replacing the one whose access token is previous. It
	// reports false when nothing matched — meaning a sign-in landed while this renewal was in
	// flight, whose token is newer than the one this run holds and must not be overwritten.
	Renew(ctx context.Context, previous string, t Token) (bool, error)
}

// Renewer keeps one channel's credential alive for as long as it can, and says so in time when
// it cannot.
type Renewer struct {
	creds Credentials
	store Store
	httpc *http.Client
	now   func() time.Time
}

// NewRenewer builds a Renewer. now is injectable for the same reason it is everywhere else in
// this feature: every branch below is a comparison against a clock.
func NewRenewer(creds Credentials, store Store, httpc *http.Client, nowFn func() time.Time) *Renewer {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Renewer{creds: creds, store: store, httpc: httpc, now: nowFn}
}

// Run examines the stored credential and renews it if it can.
//
// The order of the branches is the order of urgency, and each one returns rather than falling
// through: an expired token is not also "expiring soon", and a token just renewed is not then
// re-examined against the warning window with its old expiry.
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
			// A failed renewal is reported as the warning it is rather than swallowed: there
			// may be days left, and those days are exactly when a person can still act.
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

	// Checked after the access token and not before: a refresh token in its last fortnight
	// still renews, so this is a reminder about a sign-in that is due, not a broken channel.
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

	// LinkedIn returns the refresh token on every exchange, but a response that omitted it
	// would otherwise store an empty one and silently turn a renewable credential into a
	// warning tomorrow. Carrying the old one forward keeps the worse outcome — a refresh
	// token that is merely stale, which fails loudly at the next exchange.
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = tok.RefreshToken
		fresh.RefreshExpiresAt = tok.RefreshExpiresAt
	}

	stored, err := r.store.Renew(ctx, tok.AccessToken, fresh)
	if err != nil {
		return Outcome{}, fmt.Errorf("store renewed credential: %w", err)
	}
	if !stored {
		// Somebody signed in while this was in flight. Their token is newer than the one this
		// run renewed, so the correct action is to leave it alone and say nothing happened.
		return Outcome{State: StateHealthy, ExpiresAt: tok.ExpiresAt, RefreshExpiresAt: tok.RefreshExpiresAt}, nil
	}
	return Outcome{
		State:            StateRenewed,
		ExpiresAt:        fresh.ExpiresAt,
		RefreshExpiresAt: fresh.RefreshExpiresAt,
	}, nil
}
