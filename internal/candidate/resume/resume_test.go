package resume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/minio/minio-go/v7"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/db"
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
		// Mirrors the shape blobstore.IsNotFound actually recognizes, so this fake
		// exercises the real translation to ErrNotStored rather than a stand-in error.
		return nil, minio.ErrorResponse{Code: minio.NoSuchKey}
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeBlobs) List(_ context.Context, prefix string) ([]string, error) {
	var out []string
	for k := range f.objs {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeBlobs) Delete(_ context.Context, key string) error {
	delete(f.objs, key)
	return nil
}

// fakeRepo is an in-memory Repository (one pointer per user).
type fakeRepo struct {
	ptr        map[int64]string
	uploadedAt map[int64]pgtype.Timestamptz // simulates users.resume_uploaded_at
	structured map[int64][]byte
	structMod  map[int64]string
	structAt   map[int64]pgtype.Timestamptz // simulates users.resume_structured_uploaded_at
	geo        map[int64]storedGeo          // simulates users.resume_countries/regions/cities
	contacts   map[int64][]byte
	extractSt  map[int64]string
	extractDet map[int64]string
	extractFor map[int64]pgtype.Timestamptz
}

// storedGeo mirrors the three derived geography columns. A nil slice stands for SQL NULL
// ("not known"); a non-nil empty slice stands for '{}' ("stated, but unresolvable").
type storedGeo struct {
	Countries []string
	Regions   []string
	Cities    []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		ptr:        map[int64]string{},
		uploadedAt: map[int64]pgtype.Timestamptz{},
		structured: map[int64][]byte{},
		structMod:  map[int64]string{},
		structAt:   map[int64]pgtype.Timestamptz{},
		geo:        map[int64]storedGeo{},
		contacts:   map[int64][]byte{},
		extractSt:  map[int64]string{},
		extractDet: map[int64]string{},
		extractFor: map[int64]pgtype.Timestamptz{},
	}
}

// SetStructured models the real statement, guard included: the SQL carries
// `WHERE id = $1 AND resume_uploaded_at = $stamp`, so a write stamped with a
// since-superseded upload time matches no row and is dropped whole — structure AND
// geography. The fake used to write unconditionally, which meant no Store-level test
// could ever observe that the two travel together.
func (r *fakeRepo) SetStructured(_ context.Context, w StructuredWrite) (bool, error) {
	if cur, ok := r.uploadedAt[w.UserID]; !ok || !cur.Valid || !cur.Time.Equal(w.UploadedAt) {
		return false, nil
	}
	r.structured[w.UserID] = w.Blob
	r.structMod[w.UserID] = w.Model
	r.structAt[w.UserID] = pgtype.Timestamptz{Time: w.UploadedAt, Valid: true}
	r.geo[w.UserID] = storedGeo{Countries: w.Countries, Regions: w.Regions, Cities: w.Cities}
	r.extractSt[w.UserID] = ExtractStatusOK
	r.extractDet[w.UserID] = ""
	r.extractFor[w.UserID] = pgtype.Timestamptz{Time: w.UploadedAt, Valid: true}
	return true, nil
}

func (r *fakeRepo) GetStructured(_ context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return db.GetUserResumeStructuredRow{
		ResumeStructured:           r.structured[userID],
		ResumeStructuredModel:      pgtype.Text{String: r.structMod[userID], Valid: r.structMod[userID] != ""},
		ResumeStructuredUploadedAt: r.structAt[userID],
		ResumeUploadedAt:           r.uploadedAt[userID],
		CandidateContacts:          r.contacts[userID],
		ResumeExtractStatus:        pgtype.Text{String: r.extractSt[userID], Valid: r.extractSt[userID] != ""},
		ResumeExtractDetail:        pgtype.Text{String: r.extractDet[userID], Valid: r.extractDet[userID] != ""},
		ResumeExtractFor:           r.extractFor[userID],
	}, nil
}

func (r *fakeRepo) Get(_ context.Context, userID int64) (db.GetUserResumeRow, error) {
	key, ok := r.ptr[userID]
	if !ok {
		return db.GetUserResumeRow{}, nil
	}
	return db.GetUserResumeRow{
		ResumeObjectKey:  pgtype.Text{String: key, Valid: true},
		ResumeUploadedAt: r.uploadedAt[userID],
	}, nil
}

func (r *fakeRepo) Set(_ context.Context, userID int64, key string) error {
	r.ptr[userID] = key
	now := time.Now().UTC()
	r.uploadedAt[userID] = pgtype.Timestamptz{Time: now, Valid: true}
	r.extractSt[userID] = ExtractStatusPending
	r.extractDet[userID] = ""
	r.extractFor[userID] = pgtype.Timestamptz{Time: now, Valid: true}
	return nil
}

func (r *fakeRepo) Clear(_ context.Context, userID int64) error {
	// Mirrors ClearUserResume: clears pointer + extract artifacts, keeps candidate contacts.
	delete(r.ptr, userID)
	delete(r.uploadedAt, userID)
	delete(r.structured, userID)
	delete(r.structMod, userID)
	delete(r.structAt, userID)
	delete(r.geo, userID)
	delete(r.extractSt, userID)
	delete(r.extractDet, userID)
	delete(r.extractFor, userID)
	return nil
}

func (r *fakeRepo) GetCandidateContacts(_ context.Context, userID int64) ([]byte, error) {
	return r.contacts[userID], nil
}

func (r *fakeRepo) SetCandidateContacts(_ context.Context, userID int64, blob []byte) error {
	r.contacts[userID] = blob
	return nil
}

func (r *fakeRepo) SetExtractFailed(_ context.Context, userID int64, detail string, uploadedAt time.Time) error {
	if cur, ok := r.uploadedAt[userID]; !ok || !cur.Valid || !cur.Time.Equal(uploadedAt) {
		return nil
	}
	r.extractSt[userID] = ExtractStatusFailed
	r.extractDet[userID] = detail
	r.extractFor[userID] = pgtype.Timestamptz{Time: uploadedAt, Valid: true}
	return nil
}

func (r *fakeRepo) WithUploadLock(_ context.Context, _ int64, fn func(Repository) error) error {
	return fn(r)
}

func (r *fakeRepo) SetExtractPending(_ context.Context, userID int64, uploadedAt time.Time) error {
	if cur, ok := r.uploadedAt[userID]; !ok || !cur.Valid || !cur.Time.Equal(uploadedAt) {
		return nil
	}
	r.extractSt[userID] = ExtractStatusPending
	r.extractDet[userID] = ""
	r.extractFor[userID] = pgtype.Timestamptz{Time: uploadedAt, Valid: true}
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

// PutIfAbsent stores a first résumé and refuses a second, leaving the first in place.
func TestStore_PutIfAbsentStoresOnlyAFirstResume(t *testing.T) {
	s := New(newFakeBlobs(), newFakeRepo())
	ctx := context.Background()
	if _, err := s.PutIfAbsent(ctx, 7, "text/plain", []byte("first")); err != nil {
		t.Fatalf("first PutIfAbsent: %v", err)
	}
	if _, err := s.PutIfAbsent(ctx, 7, "text/plain", []byte("second")); !errors.Is(err, ErrAlreadyStored) {
		t.Fatalf("second PutIfAbsent err = %v, want ErrAlreadyStored", err)
	}
	if text, _ := s.Text(ctx, 7); text != "first" {
		t.Errorf("stored text = %q, want the first", text)
	}
}

// racingRepo lands another upload's pointer the moment the lock is taken: what a concurrent
// upload that won the lock first looks like to the one waiting behind it.
type racingRepo struct {
	*fakeRepo
	blobs *fakeBlobs
}

func (r racingRepo) WithUploadLock(ctx context.Context, userID int64, fn func(Repository) error) error {
	r.blobs.objs["resumes/7-winner"] = []byte("winner")
	if err := r.Set(ctx, userID, "resumes/7-winner"); err != nil {
		return err
	}
	return fn(r.fakeRepo)
}

// The refusal that decides is the one under the lock, after this upload's object is written:
// it must leave the winner's object untouched and delete its own, so nothing is orphaned.
func TestStore_PutIfAbsentRefusedUnderTheLockLeavesNoOrphan(t *testing.T) {
	blobs := newFakeBlobs()
	s := New(blobs, racingRepo{fakeRepo: newFakeRepo(), blobs: blobs})
	if _, err := s.PutIfAbsent(context.Background(), 7, "text/plain", []byte("loser")); !errors.Is(err, ErrAlreadyStored) {
		t.Fatalf("PutIfAbsent err = %v, want ErrAlreadyStored", err)
	}
	if len(blobs.objs) != 1 || string(blobs.objs["resumes/7-winner"]) != "winner" {
		t.Errorf("objects = %v, want only the winner's, untouched", blobs.objs)
	}
}

// Each upload has its own object; replacing one deletes the object the pointer left, and a
// delete removes the one it names — including a row still on the old fixed key.
func TestStore_ObjectsFollowThePointer(t *testing.T) {
	blobs, repo := newFakeBlobs(), newFakeRepo()
	blobs.objs["resumes/7"] = []byte("legacy")
	repo.ptr[7] = "resumes/7"
	s := New(blobs, repo)
	ctx := context.Background()

	if _, err := s.Put(ctx, 7, "text/plain", []byte("new")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, legacy := blobs.objs["resumes/7"]; legacy || len(blobs.objs) != 1 || string(blobs.objs[repo.ptr[7]]) != "new" {
		t.Fatalf("after replace objects = %v, pointer %q; want only the new object", blobs.objs, repo.ptr[7])
	}
	if err := s.Delete(ctx, 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(blobs.objs) != 0 {
		t.Errorf("after delete objects = %v, want none", blobs.objs)
	}
}

// A pointer write that fails leaves no object behind: the one just written is deleted.
func TestStore_FailedPointerWriteDeletesTheNewObject(t *testing.T) {
	blobs := newFakeBlobs()
	s := New(blobs, failingUploadRepo{newFakeRepo()})
	if _, err := s.Put(context.Background(), 7, "text/plain", []byte("CV")); err == nil {
		t.Fatal("Put succeeded with a failing pointer write")
	}
	if len(blobs.objs) != 0 {
		t.Errorf("objects = %v, want the orphan deleted", blobs.objs)
	}
}

// ambiguousCommitRepo writes the pointer and then reports a failure, as a COMMIT whose reply
// was lost after the server committed it does.
type ambiguousCommitRepo struct{ *fakeRepo }

func (r ambiguousCommitRepo) WithUploadLock(_ context.Context, _ int64, fn func(Repository) error) error {
	if err := fn(r.fakeRepo); err != nil {
		return err
	}
	return errors.New("commit reply lost")
}

// The upload reports the failure, but the object the pointer now names is kept: deleting it
// would leave the pointer naming nothing.
func TestStore_AmbiguousCommitKeepsTheObjectThePointerNames(t *testing.T) {
	blobs, repo := newFakeBlobs(), newFakeRepo()
	s := New(blobs, ambiguousCommitRepo{repo})
	if _, err := s.Put(context.Background(), 7, "text/plain", []byte("CV")); err == nil {
		t.Fatal("Put hid the commit failure")
	}
	if got := string(blobs.objs[repo.ptr[7]]); got != "CV" {
		t.Errorf("the object the pointer names = %q, want it kept", got)
	}
}

// deletedAccountRepo is an upload racing account deletion: the user row is gone by the time
// the pointer is written, so the write fails and the account no longer exists to read.
type deletedAccountRepo struct{ *fakeRepo }

func (deletedAccountRepo) WithUploadLock(context.Context, int64, func(Repository) error) error {
	return pgx.ErrNoRows
}

func (deletedAccountRepo) Get(context.Context, int64) (db.GetUserResumeRow, error) {
	return db.GetUserResumeRow{}, pgx.ErrNoRows
}

// An upload whose account was deleted meanwhile leaves no object behind: no row is left that
// could ever name it.
func TestStore_UploadAfterAccountDeletionDiscardsTheObject(t *testing.T) {
	blobs := newFakeBlobs()
	if _, err := New(blobs, deletedAccountRepo{newFakeRepo()}).Put(context.Background(), 7, "text/plain", []byte("CV")); err == nil {
		t.Fatal("Put succeeded for a deleted account")
	}
	if len(blobs.objs) != 0 {
		t.Errorf("objects left = %v, want none", blobs.objs)
	}
}

// failingDeleteBlobs refuses every delete.
type failingDeleteBlobs struct{ *fakeBlobs }

func (failingDeleteBlobs) Delete(context.Context, string) error { return errors.New("storage down") }

// Deleting a CV succeeds once its pointer is cleared; the object's removal is best-effort, so
// a storage failure there neither fails the call nor brings the CV back.
func TestStore_DeleteSucceedsWhenTheObjectDeleteFails(t *testing.T) {
	repo := newFakeRepo()
	repo.ptr[7] = "resumes/7-1-x"
	s := New(failingDeleteBlobs{newFakeBlobs()}, repo)
	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if meta, _ := s.Status(context.Background(), 7); meta.Present {
		t.Error("the CV is still present after Delete")
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

func TestStore_TextAndUploadedAtRoundTrip(t *testing.T) {
	s := New(newFakeBlobs(), newFakeRepo())
	ctx := context.Background()

	meta, err := s.Put(ctx, 7, "text/plain; charset=utf-8", []byte("Go and PostgreSQL"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	text, uploadedAt, err := s.TextAndUploadedAt(ctx, 7)
	if err != nil {
		t.Fatalf("TextAndUploadedAt: %v", err)
	}
	if text != "Go and PostgreSQL" {
		t.Errorf("text = %q, want the stored text", text)
	}
	if !uploadedAt.Equal(*meta.UploadedAt) {
		t.Errorf("uploadedAt = %v, want %v (the SAME pointer read Text/Status would each see separately)",
			uploadedAt, *meta.UploadedAt)
	}
}

func TestStore_TextAndUploadedAtNotStored(t *testing.T) {
	s := New(newFakeBlobs(), newFakeRepo())
	if _, _, err := s.TextAndUploadedAt(context.Background(), 42); !errors.Is(err, ErrNotStored) {
		t.Errorf("TextAndUploadedAt err = %v, want ErrNotStored", err)
	}
}

func TestStore_TextMissingObjectIsNotStored(t *testing.T) {
	// Pointer without bytes — e.g. LocalStack volume recreated under a surviving DB.
	repo := newFakeRepo()
	repo.ptr[7] = "resumes/7"
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	s := New(newFakeBlobs(), repo)
	if _, err := s.Text(context.Background(), 7); !errors.Is(err, ErrNotStored) {
		t.Fatalf("Text err = %v, want ErrNotStored", err)
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

func TestStore_ProfileReadProvisionalContactsWhenStale(t *testing.T) {
	repo := newFakeRepo()
	t1 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{}}, repo)
	if err := s.SetStructured(context.Background(), 7, resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+44",
		Links: []string{"https://github.com/ada"}, Summary: "old summary", TotalYears: 11,
	}, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1.Add(time.Hour), Valid: true}

	pr, err := s.ProfileReadForUser(context.Background(), 7)
	if err != nil {
		t.Fatalf("ProfileReadForUser: %v", err)
	}
	if pr.Current || !pr.Pending {
		t.Fatalf("flags = current:%v pending:%v, want pending provisional", pr.Current, pr.Pending)
	}
	if pr.Structure.FullName != "Ada Lovelace" || pr.Structure.Phone != "+44" || len(pr.Structure.Links) != 1 {
		t.Errorf("provisional contacts = %+v", pr.Structure)
	}
	if pr.Structure.Summary != "" || pr.Structure.TotalYears != 0 {
		t.Errorf("semantic fields leaked into provisional: %+v", pr.Structure)
	}

	contacts, ok, err := s.ProvisionalContacts(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("ProvisionalContacts = ok:%v err:%v, want contacts", ok, err)
	}
	if contacts.FullName != "Ada Lovelace" || contacts.Summary != "" {
		t.Errorf("ProvisionalContacts = %+v", contacts)
	}
	if _, sok, err := s.Structured(context.Background(), 7); sok || err != nil {
		t.Fatalf("Structured while stale = ok:%v err:%v, want ok=false", sok, err)
	}
}

func TestStore_ProvisionalContactsAbsentWhenCurrent(t *testing.T) {
	repo := newFakeRepo()
	t1 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{}}, repo)
	if err := s.SetStructured(context.Background(), 7, resumeextract.Structured{
		FullName: "Ada", Email: "ada@example.com",
	}, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}
	if _, ok, err := s.ProvisionalContacts(context.Background(), 7); ok || err != nil {
		t.Fatalf("ProvisionalContacts when current = ok:%v err:%v, want ok=false", ok, err)
	}
}

func TestStore_ProvisionalContactsAbsentWhenNoBlob(t *testing.T) {
	repo := newFakeRepo()
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{}}, repo)
	if _, ok, err := s.ProvisionalContacts(context.Background(), 7); ok || err != nil {
		t.Fatalf("ProvisionalContacts with no blob = ok:%v err:%v, want ok=false", ok, err)
	}
}

func TestStore_ProfileReadCurrentIsFull(t *testing.T) {
	repo := newFakeRepo()
	t1 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{}}, repo)
	if err := s.SetStructured(context.Background(), 7, resumeextract.Structured{
		FullName: "Ada", Summary: "now", TotalYears: 5,
	}, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}
	pr, err := s.ProfileReadForUser(context.Background(), 7)
	if err != nil || !pr.Current || pr.Pending {
		t.Fatalf("ProfileReadForUser = %+v err=%v, want current", pr, err)
	}
	if pr.Structure.Summary != "now" || pr.Structure.TotalYears != 5 {
		t.Errorf("current structure = %+v", pr.Structure)
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

func (r *fakeRepo) GetGeography(_ context.Context, userID int64) (db.GetUserResumeGeographyRow, error) {
	g := r.geo[userID]
	return db.GetUserResumeGeographyRow{
		ResumeCountries:            g.Countries,
		ResumeRegions:              g.Regions,
		ResumeCities:               g.Cities,
		ResumeStructuredUploadedAt: r.structAt[userID],
		ResumeUploadedAt:           r.uploadedAt[userID],
	}, nil
}

// File hands back the stored bytes as uploaded — what an application on freehire attaches
// when the candidate sends their own CV rather than a tailored one.
func TestStore_FileReturnsTheUploadedBytes(t *testing.T) {
	s := New(newFakeBlobs(), newFakeRepo())
	ctx := context.Background()
	if _, err := s.File(ctx, 7); !errors.Is(err, ErrNotStored) {
		t.Fatalf("File before an upload: err = %v, want ErrNotStored", err)
	}
	if _, err := s.Put(ctx, 7, "application/pdf", []byte("%PDF-1.7 cv")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.File(ctx, 7)
	if err != nil || string(got) != "%PDF-1.7 cv" {
		t.Fatalf("File = %q, %v; want the uploaded bytes", got, err)
	}
	if _, err := New(nil, newFakeRepo()).File(ctx, 7); !errors.Is(err, ErrStorageDisabled) {
		t.Errorf("File with no storage: err = %v, want ErrStorageDisabled", err)
	}
}
