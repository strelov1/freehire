package resume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
)

// fakeBlobs is an in-memory blobstore.Store for tests.
type fakeBlobs struct{ objs map[string][]byte }

func newFakeBlobs() *fakeBlobs { return &fakeBlobs{objs: map[string][]byte{}} }

func (f *fakeBlobs) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objs[key] = data
	return nil
}

func (f *fakeBlobs) Get(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := f.objs[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeBlobs) Delete(_ context.Context, key string) error {
	delete(f.objs, key)
	return nil
}

// fakeRepo is an in-memory Repository (one pointer per user).
type fakeRepo struct {
	ptr      map[int64]string
	embVec   map[int64][]float64
	embModel map[int64]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{ptr: map[int64]string{}, embVec: map[int64][]float64{}, embModel: map[int64]string{}}
}

func (r *fakeRepo) SetEmbedding(_ context.Context, userID int64, vec []float64, model string) error {
	r.embVec[userID], r.embModel[userID] = vec, model
	return nil
}

func (r *fakeRepo) GetEmbedding(_ context.Context, userID int64) (db.GetUserResumeEmbeddingRow, error) {
	return db.GetUserResumeEmbeddingRow{
		ResumeEmbedding:      r.embVec[userID],
		ResumeEmbeddingModel: pgtype.Text{String: r.embModel[userID], Valid: r.embModel[userID] != ""},
	}, nil
}

func (r *fakeRepo) Get(_ context.Context, userID int64) (db.GetUserResumeRow, error) {
	key, ok := r.ptr[userID]
	if !ok {
		return db.GetUserResumeRow{}, nil
	}
	return db.GetUserResumeRow{
		ResumeObjectKey:  pgtype.Text{String: key, Valid: true},
		ResumeUploadedAt: pgtype.Timestamptz{Valid: true},
	}, nil
}

func (r *fakeRepo) Set(_ context.Context, userID int64, key string) error {
	r.ptr[userID] = key
	return nil
}

func (r *fakeRepo) Clear(_ context.Context, userID int64) error {
	delete(r.ptr, userID)
	return nil
}

func TestStore_DisabledWhenNoBlobStore(t *testing.T) {
	s := New(nil, newFakeRepo())
	if s.Enabled() {
		t.Fatal("Enabled should be false without a blob store")
	}
	if _, err := s.Put(context.Background(), 1, "text/plain", []byte("x")); !errors.Is(err, ErrStorageDisabled) {
		t.Errorf("Put err = %v, want ErrStorageDisabled", err)
	}
	if _, err := s.Status(context.Background(), 1); !errors.Is(err, ErrStorageDisabled) {
		t.Errorf("Status err = %v, want ErrStorageDisabled", err)
	}
	if _, err := s.Text(context.Background(), 1); !errors.Is(err, ErrStorageDisabled) {
		t.Errorf("Text err = %v, want ErrStorageDisabled", err)
	}
	if err := s.Delete(context.Background(), 1); !errors.Is(err, ErrStorageDisabled) {
		t.Errorf("Delete err = %v, want ErrStorageDisabled", err)
	}
}

func TestStore_PutStatusTextRoundTrip(t *testing.T) {
	s := New(newFakeBlobs(), newFakeRepo())
	ctx := context.Background()

	meta, err := s.Put(ctx, 7, "text/plain; charset=utf-8", []byte("Go and PostgreSQL"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !meta.Present || meta.UploadedAt == nil {
		t.Fatalf("Put meta = %+v, want present with a timestamp", meta)
	}

	got, err := s.Status(ctx, 7)
	if err != nil || !got.Present {
		t.Fatalf("Status = %+v, %v; want present", got, err)
	}

	text, err := s.Text(ctx, 7)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if text != "Go and PostgreSQL" {
		t.Errorf("Text = %q, want the stored text", text)
	}
}

func TestStore_TextNotStored(t *testing.T) {
	s := New(newFakeBlobs(), newFakeRepo())
	if _, err := s.Text(context.Background(), 42); !errors.Is(err, ErrNotStored) {
		t.Errorf("Text err = %v, want ErrNotStored", err)
	}
	meta, err := s.Status(context.Background(), 42)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if meta.Present {
		t.Error("Status.Present should be false when nothing is stored")
	}
}

func TestStore_DeleteClearsObjectAndPointer(t *testing.T) {
	blobs := newFakeBlobs()
	repo := newFakeRepo()
	s := New(blobs, repo)
	ctx := context.Background()

	if _, err := s.Put(ctx, 3, "text/plain", []byte("resume")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(ctx, 3); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(blobs.objs) != 0 {
		t.Errorf("object not deleted: %v", blobs.objs)
	}
	if _, err := s.Text(ctx, 3); !errors.Is(err, ErrNotStored) {
		t.Errorf("after delete, Text err = %v, want ErrNotStored", err)
	}
}

func TestExtractText_RoutesByMagicNumber(t *testing.T) {
	// Plain text passes through untouched.
	if got, err := extractText([]byte("just text")); err != nil || got != "just text" {
		t.Errorf("extractText(text) = %q, %v; want the text", got, err)
	}
	// A "%PDF" prefix routes to the PDF parser; garbage after it is a parse error, not a
	// pass-through as text.
	if _, err := extractText([]byte("%PDF-1.4 not really a pdf")); err == nil {
		t.Error("extractText on a bogus PDF should error, not return raw bytes")
	}
}

var _ blobstore.Store = (*fakeBlobs)(nil)
