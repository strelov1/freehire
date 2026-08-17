package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/minio/minio-go/v7"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeResumeBlobs is an in-memory blobstore.Store for handler tests.
type fakeResumeBlobs struct{ objs map[string][]byte }

func newFakeResumeBlobs() *fakeResumeBlobs { return &fakeResumeBlobs{objs: map[string][]byte{}} }

func (f *fakeResumeBlobs) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objs[key] = b
	return nil
}

func (f *fakeResumeBlobs) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f.objs[key]
	if !ok {
		// Mirrors the shape blobstore.IsNotFound actually recognizes, so this fake
		// exercises the real translation to ErrNotStored rather than a stand-in error.
		return nil, minio.ErrorResponse{Code: minio.NoSuchKey}
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeResumeBlobs) Delete(_ context.Context, key string) error {
	delete(f.objs, key)
	return nil
}

// resumeUploadedAt is the fixed upload time the fake repo stamps a stored résumé with,
// so a test can seed a structured stamp that matches (fresh) or differs (stale).
var resumeUploadedAt = time.Unix(1_700_000_000, 0).UTC()

// fakeResumeRepo is an in-memory résumé-pointer Repository. Set stamps a timestamp,
// mirroring the SQL now(). An upload spawns a background goroutine
// (extractStructuredResume) that writes the extract-status fields independently of the
// request goroutine, so every accessor takes mu — a test reading these fields right after
// an HTTP call would otherwise race that goroutine's write.
type fakeResumeRepo struct {
	mu          sync.Mutex
	key         string
	set         bool
	structured  []byte
	structModel string
	structAt    pgtype.Timestamptz
	// The candidate geography derived from the structure's location line, persisted in
	// the same write. nil means "not known"; an empty slice means the CV stated a place
	// the dictionary could not resolve.
	structCountries []string
	structRegions   []string
	contacts        []byte
	extractStatus   string
	extractDetail   string
	extractFor      pgtype.Timestamptz
}

func (r *fakeResumeRepo) Get(_ context.Context, _ int64) (db.GetUserResumeRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.set {
		return db.GetUserResumeRow{}, nil
	}
	return db.GetUserResumeRow{
		ResumeObjectKey:  pgtype.Text{String: r.key, Valid: true},
		ResumeUploadedAt: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true},
	}, nil
}

func (r *fakeResumeRepo) Set(_ context.Context, _ int64, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.key, r.set = key, true
	r.extractStatus, r.extractDetail = resume.ExtractStatusPending, ""
	r.extractFor = pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true}
	return nil
}

func (r *fakeResumeRepo) Clear(_ context.Context, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Mirrors ClearUserResume: clears pointer + extract artifacts, keeps candidate contacts.
	r.key, r.set = "", false
	r.structured, r.structModel, r.structAt = nil, "", pgtype.Timestamptz{}
	r.extractStatus, r.extractDetail, r.extractFor = "", "", pgtype.Timestamptz{}
	return nil
}

func (r *fakeResumeRepo) SetStructured(_ context.Context, w resume.StructuredWrite) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.structured, r.structModel = w.Blob, w.Model
	r.structAt = pgtype.Timestamptz{Time: w.UploadedAt, Valid: true}
	r.structCountries, r.structRegions = w.Countries, w.Regions
	return true, nil
}

func (r *fakeResumeRepo) GetStructured(_ context.Context, _ int64) (db.GetUserResumeStructuredRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := db.GetUserResumeStructuredRow{
		ResumeStructured:           r.structured,
		ResumeStructuredModel:      pgtype.Text{String: r.structModel, Valid: r.structModel != ""},
		ResumeStructuredUploadedAt: r.structAt,
		CandidateContacts:          r.contacts,
		ResumeExtractStatus:        pgtype.Text{String: r.extractStatus, Valid: r.extractStatus != ""},
		ResumeExtractDetail:        pgtype.Text{String: r.extractDetail, Valid: r.extractDetail != ""},
		ResumeExtractFor:           r.extractFor,
	}
	if r.set {
		row.ResumeUploadedAt = pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true}
	}
	return row, nil
}

func resumeStorageApp(t *testing.T, store *resume.Store) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &resumeHandlers{resume: store}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Put("/me/resume", g, h.PutResume)
	app.Get("/me/resume", g, h.GetResume)
	app.Delete("/me/resume", g, h.DeleteResume)
	return app, token
}

func resumeReq(t *testing.T, app *fiber.App, method, body, token string) (int, resumeMetaResponse) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, "/me/resume", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data resumeMetaResponse `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Data
}

func TestResume_PutGetDeleteRoundTrip(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	app, token := resumeStorageApp(t, store)

	status, meta := resumeReq(t, app, fiber.MethodPut, `{"text":"Go and PostgreSQL"}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("PUT status = %d, want 200", status)
	}
	if !meta.Enabled || !meta.Present || meta.UploadedAt == nil {
		t.Fatalf("PUT meta = %+v, want enabled+present with a timestamp", meta)
	}

	status, meta = resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK || !meta.Present {
		t.Fatalf("GET status/meta = %d/%+v, want 200 present", status, meta)
	}

	status, _ = resumeReq(t, app, fiber.MethodDelete, "", token)
	if status != fiber.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", status)
	}

	status, meta = resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK || meta.Present {
		t.Fatalf("after delete GET = %d/%+v, want 200 not-present", status, meta)
	}
}

func TestResume_DisabledStorage(t *testing.T) {
	// nil blob store → storage disabled.
	store := resume.New(nil, &fakeResumeRepo{})
	app, token := resumeStorageApp(t, store)

	if status, _ := resumeReq(t, app, fiber.MethodPut, `{"text":"x"}`, token); status != fiber.StatusNotImplemented {
		t.Errorf("PUT status = %d, want 501", status)
	}
	if status, _ := resumeReq(t, app, fiber.MethodDelete, "", token); status != fiber.StatusNotImplemented {
		t.Errorf("DELETE status = %d, want 501", status)
	}
	status, meta := resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK || meta.Enabled || meta.Present {
		t.Errorf("GET = %d/%+v, want 200 disabled not-present", status, meta)
	}
}

func TestResume_GetStructuredShape(t *testing.T) {
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Jane Doe", TotalYears: 5})

	// Fresh: the structured stamp equals the résumé upload time → structured is served.
	fresh := &fakeResumeRepo{key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true}}
	app, token := resumeStorageApp(t, resume.New(newFakeResumeBlobs(), fresh))
	status, meta := resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK || meta.Structured == nil {
		t.Fatalf("fresh GET = %d/%+v, want 200 with structured present", status, meta)
	}
	if meta.Structured.FullName != "Jane Doe" {
		t.Errorf("structured = %+v, want the stored value", meta.Structured)
	}
	if meta.StructurePending {
		t.Error("fresh GET set structure_pending, want false")
	}

	// Stale: stamp predates the current résumé → provisional contacts + pending, not null.
	stale := &fakeResumeRepo{key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt.Add(-time.Hour), Valid: true}}
	app, token = resumeStorageApp(t, resume.New(newFakeResumeBlobs(), stale))
	status, meta = resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK || meta.Structured == nil {
		t.Fatalf("stale GET = %d/%+v, want 200 with provisional structured", status, meta)
	}
	if !meta.StructurePending {
		t.Error("stale GET structure_pending = false, want true")
	}
	if meta.Structured.FullName != "Jane Doe" || meta.Structured.TotalYears != 0 {
		t.Errorf("stale structured = %+v, want contacts only (no TotalYears)", meta.Structured)
	}
}

func TestResume_Unauthenticated(t *testing.T) {
	app, _ := resumeStorageApp(t, resume.New(newFakeResumeBlobs(), &fakeResumeRepo{}))
	if status, _ := resumeReq(t, app, fiber.MethodGet, "", ""); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}

func resumeContactsApp(t *testing.T, store *resume.Store) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &resumeHandlers{resume: store}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Get("/me/resume", g, h.GetResume)
	app.Put("/me/resume/contacts", g, h.PutResumeContacts)
	app.Post("/me/resume/contacts/replace-from-cv", g, h.ReplaceResumeContactsFromCV)
	return app, token
}

func TestResume_PutContactsAndGetParseStatus(t *testing.T) {
	repo := &fakeResumeRepo{
		key: "resumes/1", set: true,
		extractStatus: resume.ExtractStatusPending,
		extractFor:    pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true},
	}
	store := resume.New(newFakeResumeBlobs(), repo)
	app, token := resumeContactsApp(t, store)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/me/resume/contacts", strings.NewReader(
		`{"full_name":"Ada","email":"ada@example.com","links":["https://ada.example"]}`,
	))
	req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT contacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT contacts status = %d, want 200", resp.StatusCode)
	}

	status, meta := resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if meta.ParseStatus != resume.ExtractStatusPending {
		t.Fatalf("parse_status = %q, want pending", meta.ParseStatus)
	}
	if meta.Contacts == nil || meta.Contacts.FullName != "Ada" || meta.Contacts.Email != "ada@example.com" {
		t.Fatalf("contacts = %+v, want Ada / ada@example.com", meta.Contacts)
	}
}

// Languages ride the same owned-contacts write PutContacts already handles — one PUT,
// one blob, no separate endpoint. See Contacts.Languages.
func TestResume_PutContactsLanguagesAndGetReflectsIt(t *testing.T) {
	repo := &fakeResumeRepo{key: "resumes/1", set: true}
	store := resume.New(newFakeResumeBlobs(), repo)
	app, token := resumeContactsApp(t, store)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/me/resume/contacts", strings.NewReader(
		`{"languages":["English","English","  Russian  "]}`,
	))
	req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT contacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT contacts status = %d, want 200", resp.StatusCode)
	}

	status, meta := resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if meta.Structured == nil {
		t.Fatal("structured is nil, want the composed view to carry owned languages")
	}
	if len(meta.Structured.Languages) != 2 || meta.Structured.Languages[0] != "English" || meta.Structured.Languages[1] != "Russian" {
		t.Fatalf("structured.languages = %v, want deduped [English Russian]", meta.Structured.Languages)
	}
}

// A candidate who PUTs only a body field (no identity) must not see their real name/
// email — pulled from the current extract — blanked out on the composed view. Regression
// for Owned.Empty() (now true once ANY field, including a body field, is set) getting
// confused with "identity is set" at the compose site.
func TestResume_PutContactsSummaryOnlyDoesNotBlankIdentity(t *testing.T) {
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Jane Doe", Email: "jane@example.com"})
	repo := &fakeResumeRepo{key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true}}
	store := resume.New(newFakeResumeBlobs(), repo)
	app, token := resumeContactsApp(t, store)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/me/resume/contacts", strings.NewReader(
		`{"summary":"Staff engineer"}`,
	))
	req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT contacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT contacts status = %d, want 200", resp.StatusCode)
	}

	status, meta := resumeReq(t, app, fiber.MethodGet, "", token)
	if status != fiber.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if meta.Structured == nil {
		t.Fatal("structured is nil")
	}
	if meta.Structured.FullName != "Jane Doe" || meta.Structured.Email != "jane@example.com" {
		t.Fatalf("structured identity = %+v, want the extract's Jane Doe / jane@example.com untouched", meta.Structured)
	}
	if meta.Structured.Summary != "Staff engineer" {
		t.Fatalf("structured.summary = %q, want the owned summary applied", meta.Structured.Summary)
	}
}

func (r *fakeResumeRepo) GetGeography(_ context.Context, _ int64) (db.GetUserResumeGeographyRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := db.GetUserResumeGeographyRow{
		ResumeCountries:            r.structCountries,
		ResumeRegions:              r.structRegions,
		ResumeStructuredUploadedAt: r.structAt,
	}
	if r.set {
		row.ResumeUploadedAt = pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true}
	}
	return row, nil
}

func (r *fakeResumeRepo) GetCandidateContacts(_ context.Context, _ int64) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contacts, nil
}

func (r *fakeResumeRepo) SetCandidateContacts(_ context.Context, _ int64, blob []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contacts = blob
	return nil
}

func (r *fakeResumeRepo) SetExtractFailed(_ context.Context, _ int64, detail string, uploadedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extractStatus, r.extractDetail = resume.ExtractStatusFailed, detail
	r.extractFor = pgtype.Timestamptz{Time: uploadedAt, Valid: true}
	return nil
}

func (r *fakeResumeRepo) SetExtractPending(_ context.Context, _ int64, uploadedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extractStatus, r.extractDetail = resume.ExtractStatusPending, ""
	r.extractFor = pgtype.Timestamptz{Time: uploadedAt, Valid: true}
	return nil
}
