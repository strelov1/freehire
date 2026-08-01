package llmkey

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// Queries is the slice of the generated query surface the resolver needs. *db.Queries
// satisfies it; tests supply a fake.
type Queries interface {
	GetUserLLMKey(ctx context.Context, id int64) (string, error)
	ClaimUserLLMKey(ctx context.Context, arg db.ClaimUserLLMKeyParams) (string, error)
	ClearUserLLMKey(ctx context.Context, arg db.ClearUserLLMKeyParams) error
}

// Resolver hands out the credential an account is known by on the gateway, minting one
// the first time that account asks for anything.
//
// It reports failure as the empty string rather than as an error, and that is the whole
// design: a caller writes `client.As(resolver.For(ctx, userID), tag)` and gets correct
// behaviour in every case without remembering to handle any of them. Attribution is
// bookkeeping — losing it costs us a line in a report, while failing the call over it
// would cost somebody the answer they asked for.
type Resolver struct {
	q       Queries
	gateway *Client
}

// NewResolver wires the resolver. A nil gateway is an unconfigured deployment: every
// lookup reports "" and nothing is ever minted.
func NewResolver(q Queries, gateway *Client) *Resolver {
	return &Resolver{q: q, gateway: gateway}
}

// For returns the credential this account's calls should travel on, or "" to spend on the
// service credential instead. Nil-safe.
func (r *Resolver) For(ctx context.Context, userID int64) string {
	if r == nil || r.gateway == nil || r.q == nil {
		return ""
	}
	stored, ok := r.read(ctx, userID)
	if !ok {
		// An unreadable store is NOT an account without a credential. Minting on it
		// would issue a fresh key on every call for as long as the database is unwell,
		// each one unstorable and each one left at the gateway.
		return ""
	}
	if stored != "" {
		return stored
	}
	return r.mint(ctx, userID)
}

// Stored returns the credential this account already has, or "" — and never mints one.
//
// It is what a read of somebody's own usage uses. Minting there would create a credential
// for every visitor who opened a page out of curiosity, and turn "accounts with a key"
// from a count of who has spent into a count of who has looked. Nil-safe.
func (r *Resolver) Stored(ctx context.Context, userID int64) string {
	stored, _ := r.read(ctx, userID)
	return stored
}

// read returns the stored credential and whether the store could be read at all. The two
// are separate answers: "this account has none" invites minting one, while "the database
// did not answer" must not — telling them apart is what keeps an unwell database from
// issuing a key per request.
func (r *Resolver) read(ctx context.Context, userID int64) (string, bool) {
	if r == nil || r.q == nil {
		return "", false
	}
	stored, err := r.q.GetUserLLMKey(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", true // a real account with no credential yet
	}
	if err != nil {
		log.Printf("llmkey: read credential for user %d: %v", userID, err)
		return "", false
	}
	return stored, true
}

// mint issues a credential and stores it, resolving the race two concurrent first calls
// create: both mint, the database admits one, and the loser adopts the winner's value and
// revokes its own.
func (r *Resolver) mint(ctx context.Context, userID int64) string {
	minted, err := r.gateway.Mint(ctx, userID)
	if err != nil {
		log.Printf("llmkey: mint credential for user %d: %v", userID, err)
		return ""
	}
	// Mint already refuses an empty secret; this is the second lock on the same door,
	// because "" is the stored signal for "not minted" and writing it would strand the
	// account in a state no later call can tell from a fresh one.
	if minted == "" {
		return ""
	}

	claimed, err := r.q.ClaimUserLLMKey(ctx, db.ClaimUserLLMKeyParams{
		ID:     userID,
		LlmKey: pgtype.Text{String: minted, Valid: true},
	})
	switch {
	case err == nil:
		return claimed
	case errors.Is(err, pgx.ErrNoRows):
		// Somebody else claimed first. Their credential is the account's; ours was born
		// unreferenced and has to go, or it sits at the gateway forever spending nothing
		// and appearing in every listing.
		r.revoke(ctx, minted)
		winner, err := r.q.GetUserLLMKey(ctx, userID)
		if err != nil {
			log.Printf("llmkey: re-read credential for user %d: %v", userID, err)
			return ""
		}
		return winner
	default:
		// We hold a live credential the database will never point at again. Revoking it
		// is the only way it does not spend under an account nothing can connect it to.
		log.Printf("llmkey: store credential for user %d: %v", userID, err)
		r.revoke(ctx, minted)
		return ""
	}
}

// Forget drops a credential the gateway no longer recognises so the next call mints a
// replacement. Nil-safe.
//
// The row is cleared first: that is what makes the very next call work. Revoking at the
// gateway is mopping up something it has usually already forgotten, which is exactly why
// this is called at all.
func (r *Resolver) Forget(ctx context.Context, userID int64, secret string) {
	if r == nil || r.q == nil {
		return
	}
	err := r.q.ClearUserLLMKey(ctx, db.ClearUserLLMKeyParams{
		ID:     userID,
		LlmKey: pgtype.Text{String: secret, Valid: true},
	})
	if err != nil {
		log.Printf("llmkey: clear credential for user %d: %v", userID, err)
	}
	r.revoke(ctx, secret)
}

// Revoke stops this account's credential from spending, without erasing it and without
// touching the row.
//
// It BLOCKS rather than deletes, and that is the point: the gateway's record of what this
// credential spent is the cost history, and deleting the key takes that history with it.
// A departing member must stop being able to spend; they do not have to take last
// quarter's numbers with them.
//
// What is left behind on the gateway is a blocked key labelled with an internal numeric
// id — an identifier that maps to nobody once the account row is gone, which is what the
// deletion itself accomplishes.
//
// The row is deliberately left alone: it is about to be erased by the cascade, and
// clearing it first would only cost a write. Nil-safe.
func (r *Resolver) Revoke(ctx context.Context, userID int64) error {
	if r == nil {
		return nil
	}
	secret := r.Stored(ctx, userID)
	if secret == "" {
		return nil
	}
	return r.gateway.Block(ctx, secret)
}

// revoke deletes a credential at the gateway, best-effort. Every caller has already
// decided the value is not to be used again, so a failure here leaves a key that spends
// nothing — worth a log line and nothing more.
func (r *Resolver) revoke(ctx context.Context, secret string) {
	if err := r.gateway.Delete(ctx, secret); err != nil {
		log.Printf("llmkey: revoke credential: %v", err)
	}
}
