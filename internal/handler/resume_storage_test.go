package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
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
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeResumeBlobs) Delete(_ context.Context, key string) error {
	delete(f.objs, key)
	return nil
}

// fakeResumeRepo is an in-memory résumé-pointer Repository. Set stamps a timestamp,
// mirroring the SQL now().
type fakeResumeRepo struct {
	key string
	set bool
}

func (r *fakeResumeRepo) Get(_ context.Context, _ int64) (db.GetUserResumeRow, error) {
	if !r.set {
		return db.GetUserResumeRow{}, nil
	}
	return db.GetUserResumeRow{
		ResumeObjectKey:  pgtype.Text{String: r.key, Valid: true},
		ResumeUploadedAt: pgtype.Timestamptz{Time: time.Unix(1_700_000_000, 0).UTC(), Valid: true},
	}, nil
}

func (r *fakeResumeRepo) Set(_ context.Context, _ int64, key string) error {
	r.key, r.set = key, true
	return nil
}

func (r *fakeResumeRepo) Clear(_ context.Context, _ int64) error {
	r.key, r.set = "", false
	return nil
}

func resumeStorageApp(t *testing.T, store *resume.Store) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, resume: store}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss)
	app.Put("/me/resume", g, h.PutResume)
	app.Get("/me/resume", g, h.GetResume)
	app.Delete("/me/resume", g, h.DeleteResume)
	return app, token
}

func resumeReq(t *testing.T, app *fiber.App, method, body, token string) (int, resumeMetaResponse) {
	t.Helper()
	req := httptest.NewRequest(method, "/me/resume", strings.NewReader(body))
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

func TestResume_Unauthenticated(t *testing.T) {
	app, _ := resumeStorageApp(t, resume.New(newFakeResumeBlobs(), &fakeResumeRepo{}))
	if status, _ := resumeReq(t, app, fiber.MethodGet, "", ""); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}
