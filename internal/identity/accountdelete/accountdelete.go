// Package accountdelete erases a member's account for good. Deletion spans four
// systems that cannot share a transaction — Postgres, object storage, Google, and the
// LLM gateway — so the ordering between them IS the design, and it lives here rather
// than in a handler where it could not be tested with fakes.
//
// Postgres does most of the work by itself: every user-owned table declares
// ON DELETE CASCADE, so one DELETE reaches the CVs, mail, plan usage, tracking, keys,
// and community persona. What the cascade cannot reach is what this package adds:
// the objects in the bucket, the OAuth grant held at Google, and the gateway
// credential the account's model calls were spent under.
//
// The gateway credential is the one thing here that is retired rather than erased. It
// is what the gateway's spend records hang off, and a cost history that vanishes when
// somebody leaves is not a cost history; blocking it stops the spending and keeps the
// numbers.
package accountdelete

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/platform/blobstore"
)

// ErrStorageUnavailable reports that the member's objects could not be erased, so
// nothing was deleted. The handler renders it as a retryable failure: the account is
// untouched and the same request can simply be made again.
var ErrStorageUnavailable = errors.New("accountdelete: object storage unavailable")

// Repository is the database side of erasure.
type Repository interface {
	// BlobKeys returns every object-storage key the account owns. Only meaningful
	// before DeleteUser: the mail and referral-proof keys live in the rows themselves.
	BlobKeys(ctx context.Context, userID int64) ([]string, error)
	// DeleteUser erases the account row, and with it every cascading user-owned row.
	DeleteUser(ctx context.Context, userID int64) error
}

// RevokeFunc surrenders an external grant held on the member's behalf — today the
// Gmail OAuth token, revoked at Google. It is nil when the feature is unconfigured.
type RevokeFunc func(ctx context.Context, userID int64) error

// AppleGrantsFunc releases the account's Apple Sign In grants. Unlike RevokeFunc
// above, it is not best-effort: the apple_grants row references users with
// ON DELETE RESTRICT, so a failure here must stop deletion rather than let
// DeleteUser fail underneath it with a raw FK violation.
type AppleGrantsFunc func(ctx context.Context, userID int64) error

// Service erases accounts. blobs is nil when object storage is unconfigured and
// revoke is nil when Gmail is; either way there is simply nothing to erase there,
// which must not stop a member from leaving.
type Service struct {
	repo   Repository
	blobs  blobstore.Store
	revoke RevokeFunc
	// blockKey stops the credential the LLM gateway knows this account by, without
	// erasing it — the gateway's spend record hangs off that key. Nil when the
	// deployment does not attribute spend, which is simply nothing to do there.
	blockKey RevokeFunc
	// appleGrants releases the account's Apple Sign In grants. Nil only in tests
	// that don't wire it; production always does, since apple_grants exists
	// regardless of whether AUTH_V2_ENABLED is on.
	appleGrants AppleGrantsFunc
}

// New builds the service. Pass a nil store and/or a nil revoker for a deployment
// where those features are off.
func New(repo Repository, blobs blobstore.Store, revoke RevokeFunc) *Service {
	return &Service{repo: repo, blobs: blobs, revoke: revoke}
}

// Delete erases the account and everything it owns, permanently.
//
// The order is deliberate: every step that can still hard-fail runs BEFORE the one
// irrecoverable step, object deletion. Apple grants go first because a failure there
// must abort deletion (the row is ON DELETE RESTRICT'd by apple_grants) — and it must
// abort before anything unrecoverable has happened, not after. Object keys are
// collected next, because the mail and referral-proof keys are only knowable through
// rows that are about to disappear. Objects are then erased BEFORE the rows: if that
// fails, nothing has been deleted and the member can retry, whereas the reverse order
// would strand their CV and raw mail in the bucket with no key left to find them by.
// Object deletes are idempotent, so a retry after a partial run is safe.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	// Apple grants block the row itself (ON DELETE RESTRICT), so releasing them must
	// happen before DeleteUser regardless — and, unlike the best-effort steps below, a
	// failure here stops deletion instead of surfacing as a raw FK violation out of
	// DeleteUser. It runs first of all so that a hard failure here leaves the account
	// (and its objects) completely untouched, rather than aborting after the objects —
	// which cannot be un-deleted — are already gone.
	if s.appleGrants != nil {
		if err := s.appleGrants(ctx, userID); err != nil {
			return fmt.Errorf("accountdelete: release apple grants: %w", err)
		}
	}
	if s.blobs != nil {
		keys, err := s.repo.BlobKeys(ctx, userID)
		if err != nil {
			return fmt.Errorf("accountdelete: collect object keys: %w", err)
		}
		if err := s.deleteObjects(ctx, keys); err != nil {
			return err
		}
	}
	// Best-effort: the stored token is about to be discarded either way, which already
	// costs this system its access. Blocking the member's exit on Google's
	// availability would be the worse failure.
	if s.revoke != nil {
		if err := s.revoke(ctx, userID); err != nil {
			log.Printf("accountdelete: revoke grant for user %d: %v", userID, err)
		}
	}
	// The gateway credential is the same shape of problem as the objects: its value
	// lives in the row that is about to disappear, so it has to be dealt with first or
	// nothing can name it afterwards — a live credential spending under an account that
	// no longer exists.
	//
	// It is BLOCKED rather than erased. The gateway's record of what that key spent is
	// the cost history, and deleting the key takes the history with it; a departing
	// member must stop being able to spend, but need not take last quarter's numbers
	// along. Best-effort like the grant above, and for the same reason.
	if s.blockKey != nil {
		if err := s.blockKey(ctx, userID); err != nil {
			log.Printf("accountdelete: block gateway credential for user %d: %v", userID, err)
		}
	}
	return s.repo.DeleteUser(ctx, userID)
}

// WithGatewayKeys attaches the blocker for the LLM gateway credential. Separate from the
// constructor because a deployment that does not attribute spend is ordinary rather than
// degraded, and because this is the third external system erasure spans — a fourth
// positional argument would stop saying which is which.
func (s *Service) WithGatewayKeys(block RevokeFunc) *Service {
	s.blockKey = block
	return s
}

// WithAppleGrants attaches the releaser for the account's Apple Sign In grants.
// Separate from the constructor for the same reason WithGatewayKeys is.
func (s *Service) WithAppleGrants(release AppleGrantsFunc) *Service {
	s.appleGrants = release
	return s
}

func (s *Service) deleteObjects(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := s.blobs.Delete(ctx, key); err != nil {
			return fmt.Errorf("%w: delete %q: %v", ErrStorageUnavailable, key, err)
		}
	}
	return nil
}
