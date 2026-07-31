package headshot

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeBlobs is an in-memory blobstore.Store recording what was written under which key.
type fakeBlobs struct {
	objects  map[string][]byte
	types    map[string]string
	deleted  []string
	putErr   error
	getErr   error
	deletErr error
}

func newFakeBlobs() *fakeBlobs {
	return &fakeBlobs{objects: map[string][]byte{}, types: map[string]string{}}
}

func (f *fakeBlobs) Put(_ context.Context, key, contentType string, r io.Reader, _ int64) error {
	if f.putErr != nil {
		return f.putErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = data
	f.types[key] = contentType
	return nil
}

func (f *fakeBlobs) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	data, ok := f.objects[key]
	if !ok {
		return nil, errors.New("no such object")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeBlobs) Delete(_ context.Context, key string) error {
	if f.deletErr != nil {
		return f.deletErr
	}
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

// fakeRepo is an in-memory pointer repository.
type fakeRepo struct {
	key        string
	uploadedAt time.Time
	sets       int
	clears     int
}

func (r *fakeRepo) GetPhoto(context.Context, int64) (db.GetUserPhotoRow, error) {
	return db.GetUserPhotoRow{
		PhotoObjectKey:  pgtype.Text{String: r.key, Valid: r.key != ""},
		PhotoUploadedAt: pgtype.Timestamptz{Time: r.uploadedAt, Valid: !r.uploadedAt.IsZero()},
	}, nil
}

func (r *fakeRepo) SetPhoto(_ context.Context, _ int64, key string) error {
	r.key, r.uploadedAt, r.sets = key, time.Now(), r.sets+1
	return nil
}

func (r *fakeRepo) ClearPhoto(context.Context, int64) error {
	r.key, r.uploadedAt, r.clears = "", time.Time{}, r.clears+1
	return nil
}

func testImage(t *testing.T) []byte {
	t.Helper()
	return encode(t, bands(300, 100, false, red, green, blue), "jpeg")
}

func TestStore_PutNormalizesAndStoresUnderTheDerivedKey(t *testing.T) {
	blobs, repo := newFakeBlobs(), &fakeRepo{}
	s := New(blobs, repo)

	meta, err := s.Put(context.Background(), 42, testImage(t))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !meta.Present || meta.UploadedAt == nil {
		t.Errorf("Put returned %+v, want a present headshot with an upload time", meta)
	}
	stored, ok := blobs.objects["photos/42"]
	if !ok {
		t.Fatalf("nothing stored under photos/42, have %v", blobs.objects)
	}
	if got := blobs.types["photos/42"]; got != contentType {
		t.Errorf("content type = %q, want %q", got, contentType)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("stored object does not decode: %v", err)
	}
	if format != "jpeg" || cfg.Width != outputEdge || cfg.Height != outputEdge {
		t.Errorf("stored %s %dx%d, want jpeg %dx%d", format, cfg.Width, cfg.Height, outputEdge, outputEdge)
	}
}

func TestStore_PutRejectsNonImageWithoutTouchingStorage(t *testing.T) {
	blobs, repo := newFakeBlobs(), &fakeRepo{}
	s := New(blobs, repo)

	if _, err := s.Put(context.Background(), 42, []byte("%PDF-1.7")); !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("Put error = %v, want ErrUnsupportedImage", err)
	}
	if len(blobs.objects) != 0 || repo.sets != 0 {
		t.Errorf("a refused upload wrote %d objects and %d pointers, want none", len(blobs.objects), repo.sets)
	}
}

func TestStore_PutReplacesTheExistingHeadshot(t *testing.T) {
	blobs, repo := newFakeBlobs(), &fakeRepo{}
	s := New(blobs, repo)

	for range 2 {
		if _, err := s.Put(context.Background(), 42, testImage(t)); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if len(blobs.objects) != 1 {
		t.Errorf("two uploads left %d objects, want 1 (the key is derived, not accumulated)", len(blobs.objects))
	}
}

func TestStore_GetReturnsStoredBytes(t *testing.T) {
	blobs, repo := newFakeBlobs(), &fakeRepo{}
	s := New(blobs, repo)
	if _, err := s.Put(context.Background(), 42, testImage(t)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, blobs.objects["photos/42"]) {
		t.Error("Get returned bytes other than the stored object")
	}
}

func TestStore_GetWithoutAHeadshot(t *testing.T) {
	s := New(newFakeBlobs(), &fakeRepo{})
	if _, err := s.Get(context.Background(), 42); !errors.Is(err, ErrNotStored) {
		t.Errorf("Get error = %v, want ErrNotStored", err)
	}
}

func TestStore_DeleteClearsObjectAndPointer(t *testing.T) {
	blobs, repo := newFakeBlobs(), &fakeRepo{}
	s := New(blobs, repo)
	if _, err := s.Put(context.Background(), 42, testImage(t)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := s.Delete(context.Background(), 42); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(blobs.objects) != 0 || repo.clears != 1 {
		t.Errorf("after Delete: %d objects, %d pointer clears; want 0 and 1", len(blobs.objects), repo.clears)
	}
	meta, err := s.Status(context.Background(), 42)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if meta.Present {
		t.Error("Status still reports a headshot after Delete")
	}
}

func TestStore_DisabledWithoutObjectStorage(t *testing.T) {
	s := New(nil, &fakeRepo{})
	if s.Enabled() {
		t.Fatal("a store with no blobstore reports itself enabled")
	}
	ctx := context.Background()
	ops := map[string]error{
		"Put":    func() error { _, err := s.Put(ctx, 42, testImage(t)); return err }(),
		"Get":    func() error { _, err := s.Get(ctx, 42); return err }(),
		"Status": func() error { _, err := s.Status(ctx, 42); return err }(),
		"Delete": s.Delete(ctx, 42),
	}
	for name, err := range ops {
		if !errors.Is(err, ErrStorageDisabled) {
			t.Errorf("%s error = %v, want ErrStorageDisabled", name, err)
		}
	}
}

// A nil *Store — what a caller assembled without object storage holds — must answer
// rather than panic, so the render path can ask for a photo unconditionally.
func TestStore_NilReceiverIsDisabled(t *testing.T) {
	var s *Store
	if s.Enabled() {
		t.Fatal("nil store reports itself enabled")
	}
	if _, err := s.Get(context.Background(), 42); !errors.Is(err, ErrStorageDisabled) {
		t.Errorf("Get on nil store = %v, want ErrStorageDisabled", err)
	}
}
