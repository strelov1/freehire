// Package accountdelete erases a member's account. Deletion spans four systems that
// cannot share a transaction (Postgres, object storage, Google, the LLM gateway), so the
// ordering between them is the design, kept here so it can be tested with fakes.
//
// Every user-owned table cascades on delete, so one DELETE covers the rows. This package
// adds what the cascade cannot reach: bucket objects, the Google OAuth grant, and the
// gateway credential.
//
// The gateway credential is blocked, not erased: the gateway's spend history hangs off it.
package accountdelete

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/blobstore"
)

// ErrStorageUnavailable reports that the member's objects could not be erased, so nothing
// was deleted and the request can be retried.
var ErrStorageUnavailable = errors.New("accountdelete: object storage unavailable")

// Repository is the database side of erasure.
type Repository interface {
	// BlobKeys returns every object-storage key the account owns. Only valid before
	// DeleteUser: mail and referral-proof keys live in the rows themselves.
	BlobKeys(ctx context.Context, userID int64) ([]string, error)
	// DeleteUser erases the account row, and with it every cascading user-owned row.
	DeleteUser(ctx context.Context, userID int64) error
}

// RevokeFunc surrenders an external grant held for the member (today the Gmail OAuth
// token). Nil when unconfigured.
type RevokeFunc func(ctx context.Context, userID int64) error

// AppleGrantsFunc releases the account's Apple Sign In grants. Not best-effort: apple_grants
// references users ON DELETE RESTRICT, so a failure must stop deletion before DeleteUser
// hits a raw FK violation.
type AppleGrantsFunc func(ctx context.Context, userID int64) error

// Service erases accounts. blobs is nil when object storage is unconfigured and revoke is
// nil when Gmail is; there is nothing to erase there and it must not block leaving.
type Service struct {
	repo   Repository
	blobs  blobstore.Store
	revoke RevokeFunc
	// blockKey stops the account's LLM gateway credential without erasing it, since the spend
	// record hangs off it. Nil when spend is not attributed.
	blockKey RevokeFunc
	// appleGrants releases Apple Sign In grants. Nil only in tests; production always wires
	// it, since apple_grants exists regardless of AUTH_V2_ENABLED.
	appleGrants AppleGrantsFunc
}

// New builds the service; nil blobs or revoke mean those features are off.
func New(repo Repository, blobs blobstore.Store, revoke RevokeFunc) *Service {
	return &Service{repo: repo, blobs: blobs, revoke: revoke}
}

// Delete erases the account and everything it owns, permanently.
//
// Every step that can hard-fail runs before the irrecoverable one, object deletion. Apple
// grants go first (a failure aborts with nothing lost). Object keys are collected next,
// since mail and referral-proof keys are only knowable through rows about to disappear.
// Objects go before the rows: if that fails the member can retry, whereas the reverse would
// strand their objects with no key left to find them. Object deletes are idempotent.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	// Runs first: unlike the best-effort steps below, a failure stops deletion, and it must
	// do so before the objects, which cannot be un-deleted, are gone.
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
		// The row names only the current CV object; others under the member's prefix (failed
		// cleanups, interrupted uploads) are found by listing.
		cvKeys, err := blobstore.ResumeObjectKeys(ctx, s.blobs, userID)
		if err != nil {
			return fmt.Errorf("%w: list CV objects: %v", ErrStorageUnavailable, err)
		}
		if err := s.deleteObjects(ctx, unique(append(keys, cvKeys...))); err != nil {
			return err
		}
	}
	// Best-effort: the token is discarded anyway, and Google's availability must not block
	// the member leaving.
	if s.revoke != nil {
		if err := s.revoke(ctx, userID); err != nil {
			log.Printf("accountdelete: revoke grant for user %d: %v", userID, err)
		}
	}
	// The credential's value lives in the row about to disappear, so block it first or a
	// live key would keep spending for a deleted account. Blocked, not erased, to keep the
	// cost history. Best-effort like the grant above.
	if s.blockKey != nil {
		if err := s.blockKey(ctx, userID); err != nil {
			log.Printf("accountdelete: block gateway credential for user %d: %v", userID, err)
		}
	}
	if err := s.repo.DeleteUser(ctx, userID); err != nil {
		return err
	}
	s.sweepCVObjects(ctx, userID)
	return nil
}

// sweepCVObjects re-lists the member's CV upload prefix after the row is gone: an upload
// that committed between the earlier listing and DeleteUser left an object nothing else can
// name. Best-effort; the account is already erased, so failures are only logged.
func (s *Service) sweepCVObjects(ctx context.Context, userID int64) {
	if s.blobs == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	keys, err := s.blobs.List(ctx, blobstore.ResumeUploadPrefix(userID))
	if err != nil {
		log.Printf("accountdelete: re-list CV objects for user %d: %v", userID, err)
		return
	}
	for _, key := range keys {
		if err := s.blobs.Delete(ctx, key); err != nil {
			log.Printf("accountdelete: delete CV object %s: %v", key, err)
		}
	}
}

// unique drops repeated keys, keeping the first occurrence's order.
func unique(keys []string) []string {
	seen := make(map[string]bool, len(keys))
	out := keys[:0]
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// WithGatewayKeys attaches the blocker for the LLM gateway credential. Optional, since a
// deployment without spend attribution is normal.
func (s *Service) WithGatewayKeys(block RevokeFunc) *Service {
	s.blockKey = block
	return s
}

// WithAppleGrants attaches the releaser for the account's Apple Sign In grants.
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
