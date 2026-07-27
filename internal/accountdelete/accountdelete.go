// Package accountdelete erases a member's account for good. Deletion spans three
// systems that cannot share a transaction — Postgres, object storage, and Google —
// so the ordering between them IS the design, and it lives here rather than in a
// handler where it could not be tested with fakes.
//
// Postgres does most of the work by itself: every user-owned table declares
// ON DELETE CASCADE, so one DELETE reaches the CVs, mail, credits, tracking, keys,
// and community persona. What the cascade cannot reach is what this package adds:
// the objects in the bucket and the OAuth grant held at Google.
package accountdelete

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/blobstore"
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

// Service erases accounts. blobs is nil when object storage is unconfigured and
// revoke is nil when Gmail is; either way there is simply nothing to erase there,
// which must not stop a member from leaving.
type Service struct {
	repo   Repository
	blobs  blobstore.Store
	revoke RevokeFunc
}

// New builds the service. Pass a nil store and/or a nil revoker for a deployment
// where those features are off.
func New(repo Repository, blobs blobstore.Store, revoke RevokeFunc) *Service {
	return &Service{repo: repo, blobs: blobs, revoke: revoke}
}

// Delete erases the account and everything it owns, permanently.
//
// The order is deliberate. Object keys are collected first, because the mail and
// referral-proof keys are only knowable through rows that are about to disappear.
// Objects are then erased BEFORE the rows: if that fails, nothing has been deleted
// and the member can retry, whereas the reverse order would strand their CV and raw
// mail in the bucket with no key left to find them by. Object deletes are idempotent,
// so a retry after a partial run is safe.
func (s *Service) Delete(ctx context.Context, userID int64) error {
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
	return s.repo.DeleteUser(ctx, userID)
}

func (s *Service) deleteObjects(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := s.blobs.Delete(ctx, key); err != nil {
			return fmt.Errorf("%w: delete %q: %v", ErrStorageUnavailable, key, err)
		}
	}
	return nil
}
