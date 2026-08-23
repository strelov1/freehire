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
	if stored.Secret != "" {
		return stored.Secret
	}
	return r.mint(ctx, userID).Secret
}

// Stored returns the credential this account already has, or "" — and never mints one.
//
// It is what a read of somebody's own usage uses. Minting there would create a credential
// for every visitor who opened a page out of curiosity, and turn "accounts with a key"
// from a count of who has spent into a count of who has looked. Nil-safe.
func (r *Resolver) Stored(ctx context.Context, userID int64) Credential {
	stored, _ := r.read(ctx, userID)
	return stored
}

// read returns the stored credential and whether the store could be read at all. The two
// are separate answers: "this account has none" invites minting one, while "the database
// did not answer" must not — telling them apart is what keeps an unwell database from
// issuing a key per request.
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

// mint issues a credential and stores it, resolving the race two concurrent first calls
// create: both mint, the database admits one, and the loser adopts the winner's value and
// revokes its own.
func (r *Resolver) mint(ctx context.Context, userID int64) Credential {
	minted, err := r.gateway.Mint(ctx, userID)
	if err != nil {
		log.Printf("llmkey: mint credential for user %d: %v", userID, err)
		return Credential{}
	}
	// Mint already refuses a half credential; this is the second lock on the same door,
	// because "" is the stored signal for "not minted" and writing it would strand the
	// account in a state no later call can tell from a fresh one.
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
		// Somebody else claimed first. Their credential is the account's; ours was born
		// unreferenced and has to go, or it sits at the gateway forever spending nothing
		// and appearing in every listing.
		r.revoke(ctx, minted.ID)
		winner, err := r.q.GetUserLLMKey(ctx, userID)
		if err != nil {
			log.Printf("llmkey: re-read credential for user %d: %v", userID, err)
			return Credential{}
		}
		return Credential{ID: winner.LlmKeyID, Secret: winner.LlmKey}
	default:
		// We hold a live credential the database will never point at again. Revoking it
		// is the only way it does not spend under an account nothing can connect it to.
		log.Printf("llmkey: store credential for user %d: %v", userID, err)
		r.revoke(ctx, minted.ID)
		return Credential{}
	}
}

// Forget drops a credential the gateway refused so the next call mints a replacement.
// Nil-safe.
//
// The row is cleared first: that is what makes the very next call work. The gateway side
// BLOCKS rather than deletes, and deliberately does not distinguish why the refusal
// happened: a credential the gateway has plainly forgotten (the ordinary case) and one
// this same account's Revoke just BLOCKED moments ago for account deletion look
// identical from here — both answer 401. Deleting on that ambiguity would occasionally
// erase the very spend history Revoke blocked the key specifically to keep, if a
// request already in flight (a second tab, a running assistant turn) gets refused in the
// window between the block and the row's own removal by the deletion cascade. Blocking an
// already-blocked or already-unknown key is a safe no-op either way (Client.Block), so
// there is no case where the gentler action costs anything Delete would have bought.
func (r *Resolver) Forget(ctx context.Context, userID int64, secret string) {
	if r == nil || r.q == nil {
		return
	}
	// Read before clearing. The block below addresses the credential by the gateway's
	// own id, and the row about to be emptied is the only place that id is written down;
	// clearing first would leave a live key nothing can name.
	//
	// An unreadable store therefore stops the whole operation rather than only the block.
	// Clearing anyway would erase the one record of that id while the credential kept
	// spending — permanently, since nothing could ever name it again. Leaving the row is
	// safe: the caller's request already completed on the service credential, and the
	// next refusal retries this.
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
	// Only block what the refusal was actually about. A concurrent call may already have
	// re-minted, in which case the row now holds a different credential and blocking it
	// would revoke a good key on the strength of a stale one's 401.
	if stored.Secret != secret || stored.ID == "" {
		return
	}
	if err := r.gateway.Block(ctx, stored.ID); err != nil {
		log.Printf("llmkey: block refused credential: %v", err)
	}
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
	// An unreadable store is not an account without a credential, and the difference
	// matters more here than anywhere else in this package: everywhere else a failed read
	// costs attribution, while here it would let a deletion report success having revoked
	// nothing, leaving a live credential spending for an account that no longer exists.
	// The error goes back so the caller decides — deleting the row is what makes the id
	// unrecoverable, so this is the last moment anyone can.
	stored, ok := r.read(ctx, userID)
	if !ok {
		return fmt.Errorf("%w: cannot read the credential to revoke", ErrUpstream)
	}
	if stored.ID == "" {
		return nil
	}
	return r.gateway.Block(ctx, stored.ID)
}

// revoke deletes a credential at the gateway, best-effort. Every caller has already
// decided the value is not to be used again, so a failure here leaves a key that spends
// nothing — worth a log line and nothing more.
func (r *Resolver) revoke(ctx context.Context, keyID string) {
	if err := r.gateway.Delete(ctx, keyID); err != nil {
		log.Printf("llmkey: revoke credential: %v", err)
	}
}
