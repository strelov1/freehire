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
		embVec:     map[int64][]float64{},
		embModel:   map[int64]string{},
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

func TestIsMissingObject(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("not found"), true},
		{errors.New("NoSuchKey"), true},
		{errors.New("The specified key does not exist."), true},
		{errors.New("timeout waiting for peer"), false},
	}
	for _, tc := range cases {
		if got := isMissingObject(tc.err); got != tc.want {
			t.Errorf("isMissingObject(%v) = %v, want %v", tc.err, got, tc.want)
		}
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
