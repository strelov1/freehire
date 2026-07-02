// Package resume is the per-user stored-résumé use case: a signed-in user keeps at
// most one résumé, the original file living in S3 (internal/blobstore) under a per-user
// key, with a pointer (object key + upload time) recorded on the users row. It stores,
// reports, deletes, and derives the plain text of that résumé, so the one upload feeds
// both skill extraction and the verdict's coherence check without a second upload. When
// object storage is unconfigured the service is disabled (Enabled reports false) and
// callers degrade to the previous in-request extraction.
package resume

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ledongthuc/pdf"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
)

var (
	// ErrStorageDisabled is returned when object storage is unconfigured, so no résumé
	// can be stored, fetched, or deleted. The handler maps it to 501 (or degrades to the
	// per-request upload path).
	ErrStorageDisabled = errors.New("resume: storage is not configured")
	// ErrNotStored is returned when the user has no stored résumé (the handler maps it to
	// a conflict — the SPA prompts a single upload).
	ErrNotStored = errors.New("resume: no résumé stored")
)

// Meta reports whether a résumé is stored and, if so, when it was uploaded.
type Meta struct {
	Present    bool
	UploadedAt *time.Time
}

// Repository persists the per-user résumé pointer on the users row. Owner-scoped by id;
// the object key is derived from the id, never client input.
type Repository interface {
	Get(ctx context.Context, userID int64) (db.GetUserResumeRow, error)
	Set(ctx context.Context, userID int64, key string) error
	Clear(ctx context.Context, userID int64) error
}

// Store owns the résumé's object storage plus its pointer. blobs is nil when storage is
// unconfigured; the service then reports Enabled()==false and every operation returns
// ErrStorageDisabled (matching the searcher/facetCounter nil-guard pattern).
type Store struct {
	blobs blobstore.Store
	repo  Repository
}

// New builds the service over a blob store (nil when unconfigured) and a pointer repo.
func New(blobs blobstore.Store, repo Repository) *Store {
	return &Store{blobs: blobs, repo: repo}
}

// Enabled reports whether object storage is configured.
func (s *Store) Enabled() bool {
	return s != nil && s.blobs != nil
}

// Put stores (or replaces) the user's résumé: the original bytes go to S3 under the
// user's key and the pointer is stamped with the upload time. contentType is recorded on
// the object for a correct future download. Returns the resulting metadata.
func (s *Store) Put(ctx context.Context, userID int64, contentType string, data []byte) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	key := blobstore.ResumeKey(userID)
	if err := s.blobs.Put(ctx, key, contentType, bytes.NewReader(data), int64(len(data))); err != nil {
		return Meta{}, err
	}
	if err := s.repo.Set(ctx, userID, key); err != nil {
		return Meta{}, err
	}
	return s.Status(ctx, userID)
}

// Status reports whether the user has a stored résumé and when it was uploaded.
func (s *Store) Status(ctx context.Context, userID int64) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	ptr, err := s.repo.Get(ctx, userID)
	if err != nil {
		return Meta{}, err
	}
	return metaFromPointer(ptr), nil
}

// Text fetches the stored résumé and derives its plain text — parsing a PDF, or reading
// text as-is (the pasted-text path). ErrNotStored when the user has no résumé; the text
// is never persisted separately (derived on read, no drift).
func (s *Store) Text(ctx context.Context, userID int64) (string, error) {
	if !s.Enabled() {
		return "", ErrStorageDisabled
	}
	ptr, err := s.repo.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	if !ptr.ResumeObjectKey.Valid {
		return "", ErrNotStored
	}
	rc, err := s.blobs.Get(ctx, ptr.ResumeObjectKey.String)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("resume: read object: %w", err)
	}
	return extractText(data)
}

// Delete removes the stored object and clears the pointer.
func (s *Store) Delete(ctx context.Context, userID int64) error {
	if !s.Enabled() {
		return ErrStorageDisabled
	}
	if err := s.blobs.Delete(ctx, blobstore.ResumeKey(userID)); err != nil {
		return err
	}
	return s.repo.Clear(ctx, userID)
}

func metaFromPointer(ptr db.GetUserResumeRow) Meta {
	if !ptr.ResumeObjectKey.Valid {
		return Meta{}
	}
	m := Meta{Present: true}
	if ptr.ResumeUploadedAt.Valid {
		t := ptr.ResumeUploadedAt.Time
		m.UploadedAt = &t
	}
	return m
}

// extractText derives plain text from a stored résumé: a PDF (magic number "%PDF") is
// parsed, anything else is read as UTF-8 text (the pasted-text path). Sniffing the bytes
// keeps the blobstore interface a pure key→bytes abstraction, free of content-type
// plumbing.
func extractText(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return pdfText(data)
	}
	return string(data), nil
}

// pdfText extracts plain text from PDF bytes. ledongthuc/pdf can panic (not just error)
// on a malformed content stream, so a deferred recover maps that to an error rather than
// crashing the request.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if p := recover(); p != nil {
			text, err = "", errors.New("resume: invalid PDF")
		}
	}()
	rd, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("resume: invalid PDF: %w", err)
	}
	tr, err := rd.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("resume: invalid PDF: %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tr); err != nil {
		return "", fmt.Errorf("resume: invalid PDF: %w", err)
	}
	return buf.String(), nil
}

// QueriesRepository adapts *db.Queries to Repository, mapping the pointer to the nullable
// users columns.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository wraps *db.Queries as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func (r *QueriesRepository) Get(ctx context.Context, userID int64) (db.GetUserResumeRow, error) {
	return r.q.GetUserResume(ctx, userID)
}

func (r *QueriesRepository) Set(ctx context.Context, userID int64, key string) error {
	return r.q.SetUserResume(ctx, db.SetUserResumeParams{
		ID:              userID,
		ResumeObjectKey: pgtype.Text{String: key, Valid: true},
	})
}

func (r *QueriesRepository) Clear(ctx context.Context, userID int64) error {
	return r.q.ClearUserResume(ctx, userID)
}
