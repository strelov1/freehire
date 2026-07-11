package resume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
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
	ptr        map[int64]string
	uploadedAt map[int64]pgtype.Timestamptz // simulates users.resume_uploaded_at
	embVec     map[int64][]float64
	embModel   map[int64]string
	structured map[int64][]byte
	structMod  map[int64]string
	structAt   map[int64]pgtype.Timestamptz // simulates users.resume_structured_uploaded_at
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		ptr:        map[int64]string{},
		uploadedAt: map[int64]pgtype.Timestamptz{},
		embVec:     map[int64][]float64{},
		embModel:   map[int64]string{},
		structured: map[int64][]byte{},
		structMod:  map[int64]string{},
		structAt:   map[int64]pgtype.Timestamptz{},
	}
}

func (r *fakeRepo) SetStructured(_ context.Context, userID int64, blob []byte, model string, uploadedAt time.Time) error {
	r.structured[userID] = blob
	r.structMod[userID] = model
	r.structAt[userID] = pgtype.Timestamptz{Time: uploadedAt, Valid: true}
	return nil
}

func (r *fakeRepo) GetStructured(_ context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return db.GetUserResumeStructuredRow{
		ResumeStructured:           r.structured[userID],
		ResumeStructuredModel:      pgtype.Text{String: r.structMod[userID], Valid: r.structMod[userID] != ""},
		ResumeStructuredUploadedAt: r.structAt[userID],
		ResumeUploadedAt:           r.uploadedAt[userID],
	}, nil
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
	// Mirrors ClearUserResume, which nulls the pointer AND the derived structured columns.
	delete(r.ptr, userID)
	delete(r.structured, userID)
	delete(r.structMod, userID)
	delete(r.structAt, userID)
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

func TestStore_StructuredServedWhenStampMatches(t *testing.T) {
	repo := newFakeRepo()
	t1 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{}}, repo)

	want := resumeextract.Structured{FullName: "Jane Doe", TotalYears: 5}
	if err := s.SetStructured(context.Background(), 7, want, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	got, ok, err := s.Structured(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("Structured = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if got.FullName != "Jane Doe" || got.TotalYears != 5 {
		t.Errorf("Structured = %+v, want the stored value", got)
	}
}

func TestStore_StructuredAbsentWhenStale(t *testing.T) {
	repo := newFakeRepo()
	t1 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{}}, repo)
	if err := s.SetStructured(context.Background(), 7, resumeextract.Structured{FullName: "Old"}, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	// Simulate a re-upload: the résumé upload time moves ahead of the structured stamp.
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1.Add(time.Hour), Valid: true}

	if _, ok, err := s.Structured(context.Background(), 7); ok || err != nil {
		t.Fatalf("Structured after re-upload = (_, %v, %v), want (_, false, nil) — stale is absent", ok, err)
	}
}

func TestStore_StructuredAbsentWhenNone(t *testing.T) {
	s := New(&fakeBlobs{objs: map[string][]byte{}}, newFakeRepo())
	if _, ok, err := s.Structured(context.Background(), 99); ok || err != nil {
		t.Fatalf("Structured for user with none = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}

func TestStore_DeleteClearsStructured(t *testing.T) {
	repo := newFakeRepo()
	t1 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{"resumes/7": []byte("cv")}}, repo)
	if err := s.SetStructured(context.Background(), 7, resumeextract.Structured{FullName: "Jane"}, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := s.Structured(context.Background(), 7); ok || err != nil {
		t.Fatalf("Structured after Delete = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}

var _ blobstore.Store = (*fakeBlobs)(nil)
