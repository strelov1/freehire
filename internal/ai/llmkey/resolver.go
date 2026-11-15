package llmkey

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Queries is the slice of the generated query surface the resolver needs. *db.Queries
// satisfies it; tests supply a fake.
type Queries interface {
	GetUserLLMKey(ctx context.Context, id int64) (db.GetUserLLMKeyRow, error)
	ClaimUserLLMKey(ctx context.Context, arg db.ClaimUserLLMKeyParams) (db.ClaimUserLLMKeyRow, error)
	ClearUserLLMKey(ctx context.Context, arg db.ClearUserLLMKeyParams) error
}

// Resolver hands out the credential an account is known by on the gateway, minting one on
// first use. Failure is the empty string, not an error, so `client.As(resolver.For(ctx, id),
// tag)` is always safe: losing attribution costs a report line, failing the call costs the
// user their answer.
type Resolver struct {
	q       Queries
	gateway *Client
}

// NewResolver wires the resolver. A nil gateway means nothing is ever minted and every
// lookup reports "".
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
		// An unreadable store is not an account without a credential; minting on it would
		// issue an unstorable key per call while the database is down.
		return ""
	}
	if stored.Secret != "" {
		return stored.Secret
	}
	return r.mint(ctx, userID).Secret
}

// Stored returns the credential this account already has, or ""; it never mints. Usage
// reads use it so that merely opening a page does not create a credential. Nil-safe.
func (r *Resolver) Stored(ctx context.Context, userID int64) Credential {
	stored, _ := r.read(ctx, userID)
	return stored
}

// read returns the stored credential and whether the store could be read. "None yet"
// invites minting; "database did not answer" must not.
func (r *Resolver) read(ctx context.Context, userID int64) (Credential, bool) {
	if r == nil || r.q == nil {
		return Credential{}, false
	}
	stored, err := r.q.GetUserLLMKey(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, true // a real account with no credential yet
	}
	if err != nil {
		log.Printf("llmkey: read credential for user %d: %v", userID, err)
		return Credential{}, false
	}
	return Credential{ID: stored.LlmKeyID, Secret: stored.LlmKey}, true
}

// mint issues a credential and stores it. Two concurrent first calls both mint, the
// database admits one, and the loser adopts the winner's value and revokes its own.
func (r *Resolver) mint(ctx context.Context, userID int64) Credential {
	minted, err := r.gateway.Mint(ctx, userID)
	if err != nil {
		log.Printf("llmkey: mint credential for user %d: %v", userID, err)
		return Credential{}
	}
	// Mint already refuses a half credential; "" is the stored signal for "not minted", so
	// writing it would strand the account.
	if minted.Secret == "" || minted.ID == "" {
		return Credential{}
	}

	claimed, err := r.q.ClaimUserLLMKey(ctx, db.ClaimUserLLMKeyParams{
		ID:       userID,
		LlmKey:   pgtype.Text{String: minted.Secret, Valid: true},
		LlmKeyID: pgtype.Text{String: minted.ID, Valid: true},
	})
	switch {
	case err == nil:
		return Credential{ID: claimed.LlmKeyID, Secret: claimed.LlmKey}
	case errors.Is(err, pgx.ErrNoRows):
		// Somebody else claimed first; ours is unreferenced and must go.
		r.revoke(ctx, minted.ID)
		winner, err := r.q.GetUserLLMKey(ctx, userID)
		if err != nil {
			log.Printf("llmkey: re-read credential for user %d: %v", userID, err)
			return Credential{}
		}
		return Credential{ID: winner.LlmKeyID, Secret: winner.LlmKey}
	default:
		// A live credential the database will never point at again: revoke it.
		log.Printf("llmkey: store credential for user %d: %v", userID, err)
		r.revoke(ctx, minted.ID)
		return Credential{}
	}
}

// Forget drops a credential the gateway refused so the next call mints a replacement.
// Nil-safe.
//
// The row is cleared first so the next call works. The gateway side blocks rather than
// deletes: a forgotten credential and one Revoke just blocked for account deletion both
// answer 401, and deleting would erase the spend history Revoke kept. Blocking an
// already-blocked or unknown key is a safe no-op (Client.Block).
func (r *Resolver) Forget(ctx context.Context, userID int64, secret string) {
	if r == nil || r.q == nil {
		return
	}
	// Read before clearing: the row is the only place the gateway id is written down. An
	// unreadable store stops the whole operation, since clearing anyway would leave a live
	// key nothing can name. The next refusal retries.
	stored, ok := r.read(ctx, userID)
	if !ok {
		log.Printf("llmkey: cannot read credential for user %d; leaving it in place", userID)
		return
	}

	err := r.q.ClearUserLLMKey(ctx, db.ClearUserLLMKeyParams{
		ID:     userID,
		LlmKey: pgtype.Text{String: secret, Valid: true},
	})
	if err != nil {
		log.Printf("llmkey: clear credential for user %d: %v", userID, err)
	}
	// Block only what the refusal was about: a concurrent call may have re-minted, and a
	// stale 401 must not revoke the new key.
	if stored.Secret != secret || stored.ID == "" {
		return
	}
	if err := r.gateway.Block(ctx, stored.ID); err != nil {
		log.Printf("llmkey: block refused credential: %v", err)
	}
}

// Revoke stops this account's credential from spending, without touching the row. It
// blocks rather than deletes, so the gateway keeps the cost history; the blocked key's
// numeric id maps to nobody once the account row is gone. The row is left for the deletion
// cascade. Nil-safe.
func (r *Resolver) Revoke(ctx context.Context, userID int64) error {
	if r == nil {
		return nil
	}
	// An unreadable store must fail here, unlike elsewhere: treating it as "no credential"
	// would report a deletion as successful with a live key still spending. Deleting the row
	// makes the id unrecoverable, so this is the last chance to revoke.
	stored, ok := r.read(ctx, userID)
	if !ok {
		return fmt.Errorf("%w: cannot read the credential to revoke", ErrUpstream)
	}
	if stored.ID == "" {
		return nil
	}
	return r.gateway.Block(ctx, stored.ID)
}

// revoke deletes a credential at the gateway, best-effort: the value is already abandoned,
// so a failure only warrants a log line.
func (r *Resolver) revoke(ctx context.Context, keyID string) {
	if err := r.gateway.Delete(ctx, keyID); err != nil {
		log.Printf("llmkey: revoke credential: %v", err)
	}
}
