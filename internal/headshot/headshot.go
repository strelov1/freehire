// Package headshot is the per-user profile photo: one image per member, normalized on
// upload (square, fixed size, JPEG), the bytes living in object storage under a key
// derived from the user id and a pointer (object key + upload time) recorded on the
// users row. The CV renderer composes it into the templates that print a portrait.
//
// It deliberately does NOT live on the CV document. A document is client-writable and
// travels into tailoring prompts; a headshot is neither. Keeping it a profile asset means
// one upload serves every CV the member owns, and no image can enter a prompt.
//
// When object storage is unconfigured the service reports Enabled()==false and every
// operation returns ErrStorageDisabled — the same nil-degradation as internal/resume.
package headshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
)

// contentType is what every stored headshot is, because Normalize re-encodes whatever
// was uploaded. Nothing derived from the request's declared type is trusted or kept.
const contentType = "image/jpeg"

var (
	// ErrStorageDisabled is returned when object storage is unconfigured. The handler
	// maps it to 501: the feature is absent, not broken.
	ErrStorageDisabled = errors.New("headshot: storage is not configured")
	// ErrNotStored is returned when the user has no headshot. The read maps it to 404;
	// the renderer treats it as "draw the placeholder".
	ErrNotStored = errors.New("headshot: no headshot stored")
)

// Meta reports whether a headshot is stored and, if so, when it was last uploaded. The
// SPA uses the timestamp as a cache buster on the image URL.
type Meta struct {
	Present    bool
	UploadedAt *time.Time
}

// Repository persists the per-user headshot pointer on the users row. Owner-scoped by
// id; the object key is derived from the id, never client input.
type Repository interface {
	GetPhoto(ctx context.Context, userID int64) (db.GetUserPhotoRow, error)
	SetPhoto(ctx context.Context, userID int64, key string) error
	ClearPhoto(ctx context.Context, userID int64) error
}

// Store owns the headshot's object storage plus its pointer. blobs is nil when storage
// is unconfigured; the service then reports Enabled()==false and every operation returns
// ErrStorageDisabled.
type Store struct {
	blobs blobstore.Store
	repo  Repository
}

// New builds the service over a blob store (nil when unconfigured) and a pointer repo.
func New(blobs blobstore.Store, repo Repository) *Store {
	return &Store{blobs: blobs, repo: repo}
}

// Enabled reports whether object storage is configured. The nil-receiver case is what a
// server assembled without storage holds, so callers can ask unconditionally.
func (s *Store) Enabled() bool {
	return s != nil && s.blobs != nil
}

// Put normalizes the upload and stores it as the user's headshot, replacing any previous
// one and stamping the pointer. An upload that is not a supported image is rejected
// before storage is touched, so a bad request can never displace a good headshot.
func (s *Store) Put(ctx context.Context, userID int64, data []byte) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	normalized, err := Normalize(data)
	if err != nil {
		return Meta{}, err
	}
	key := blobstore.PhotoKey(userID)
	if err := s.blobs.Put(ctx, key, contentType, bytes.NewReader(normalized), int64(len(normalized))); err != nil {
		return Meta{}, err
	}
	if err := s.repo.SetPhoto(ctx, userID, key); err != nil {
		// The object is now in the bucket with nothing pointing at it, and account deletion
		// collects keys FROM the pointers — so an orphan here would outlive the member's
		// erasure. Best-effort removal; the upload is a failure either way.
		if delErr := s.blobs.Delete(ctx, key); delErr != nil {
			log.Printf("headshot: orphaned object %s after pointer write failed: %v", key, delErr)
		}
		return Meta{}, err
	}
	return s.Status(ctx, userID)
}

// Get returns the stored headshot's bytes, or ErrNotStored when the user has none. The
// pointer is consulted first: it is the record of whether a headshot exists, and reading
// it costs nothing next to a bucket round trip that would 404 anyway.
func (s *Store) Get(ctx context.Context, userID int64) ([]byte, error) {
	if !s.Enabled() {
		return nil, ErrStorageDisabled
	}
	ptr, err := s.repo.GetPhoto(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ptr.PhotoObjectKey.Valid {
		return nil, ErrNotStored
	}
	rc, err := s.blobs.Get(ctx, ptr.PhotoObjectKey.String)
	if err != nil {
		if blobstore.IsNotFound(err) {
			return nil, ErrNotStored
		}
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		// The S3 client's Get is lazy: a missing object typically surfaces here, on the
		// first read, rather than from Get itself.
		if blobstore.IsNotFound(err) {
			return nil, ErrNotStored
		}
		return nil, fmt.Errorf("headshot: read object: %w", err)
	}
	return data, nil
}

// Status reports whether the user has a stored headshot and when it was uploaded.
func (s *Store) Status(ctx context.Context, userID int64) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	ptr, err := s.repo.GetPhoto(ctx, userID)
	if err != nil {
		return Meta{}, err
	}
	return metaFromPointer(ptr), nil
}

// Delete removes the stored object and clears the pointer. The key comes from the pointer,
// not from PhotoKey, so the two reads of "which object is this member's" cannot drift — and
// a member with no headshot costs no delete call.
func (s *Store) Delete(ctx context.Context, userID int64) error {
	if !s.Enabled() {
		return ErrStorageDisabled
	}
	ptr, err := s.repo.GetPhoto(ctx, userID)
	if err != nil {
		return err
	}
	if !ptr.PhotoObjectKey.Valid {
		return s.repo.ClearPhoto(ctx, userID)
	}
	if err := s.blobs.Delete(ctx, ptr.PhotoObjectKey.String); err != nil {
		return err
	}
	return s.repo.ClearPhoto(ctx, userID)
}

func metaFromPointer(ptr db.GetUserPhotoRow) Meta {
	if !ptr.PhotoObjectKey.Valid {
		return Meta{}
	}
	m := Meta{Present: true}
	if ptr.PhotoUploadedAt.Valid {
		t := ptr.PhotoUploadedAt.Time
		m.UploadedAt = &t
	}
	return m
}

// QueriesRepository adapts *db.Queries to Repository, mapping the pointer to the
// nullable users columns.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository wraps *db.Queries as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func (r *QueriesRepository) GetPhoto(ctx context.Context, userID int64) (db.GetUserPhotoRow, error) {
	return r.q.GetUserPhoto(ctx, userID)
}

func (r *QueriesRepository) SetPhoto(ctx context.Context, userID int64, key string) error {
	return r.q.SetUserPhoto(ctx, db.SetUserPhotoParams{
		ID:             userID,
		PhotoObjectKey: pgtype.Text{String: key, Valid: true},
	})
}

func (r *QueriesRepository) ClearPhoto(ctx context.Context, userID int64) error {
	return r.q.ClearUserPhoto(ctx, userID)
}
