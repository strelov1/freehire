package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/headshot"
)

// fakePhotoBlobs is an in-memory blobstore.Store for the headshot handler tests.
type fakePhotoBlobs struct {
	objs map[string][]byte
	gets int
}

func newFakePhotoBlobs() *fakePhotoBlobs { return &fakePhotoBlobs{objs: map[string][]byte{}} }

func (f *fakePhotoBlobs) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objs[key] = b
	return nil
}

func (f *fakePhotoBlobs) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.gets++
	b, ok := f.objs[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakePhotoBlobs) Delete(_ context.Context, key string) error {
	delete(f.objs, key)
	return nil
}

var photoUploadedAt = time.Unix(1_700_000_500, 0).UTC()

// fakePhotoRepo is an in-memory headshot-pointer Repository.
type fakePhotoRepo struct {
	key string
	set bool
}

func (r *fakePhotoRepo) GetPhoto(context.Context, int64) (db.GetUserPhotoRow, error) {
	if !r.set {
		return db.GetUserPhotoRow{}, nil
	}
	return db.GetUserPhotoRow{
		PhotoObjectKey:  pgtype.Text{String: r.key, Valid: true},
		PhotoUploadedAt: pgtype.Timestamptz{Time: photoUploadedAt, Valid: true},
	}, nil
}

func (r *fakePhotoRepo) SetPhoto(_ context.Context, _ int64, key string) error {
	r.key, r.set = key, true
	return nil
}

func (r *fakePhotoRepo) ClearPhoto(context.Context, int64) error {
	r.key, r.set = "", false
	return nil
}

func photoApp(t *testing.T, store *headshot.Store) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &photoHandlers{photos: store}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Put("/me/photo", g, h.PutPhoto)
	app.Get("/me/photo", g, h.GetPhoto)
	app.Get("/me/photo/image", g, h.GetPhotoImage)
	app.Delete("/me/photo", g, h.DeletePhoto)
	return app, token
}

// photoUpload builds a multipart body with the image under the "file" part.
func photoUpload(t *testing.T, data []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "headshot.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func samplePhoto(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 160))
	for y := range 160 {
		for x := range 240 {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func doPhoto(t *testing.T, app *fiber.App, method string, body io.Reader, ctype, token string) *http.Response {
	return doPhotoPath(t, app, method, "/me/photo", body, ctype, token)
}

func doPhotoImage(t *testing.T, app *fiber.App, token string) *http.Response {
	return doPhotoPath(t, app, fiber.MethodGet, "/me/photo/image", nil, "", token)
}

func doPhotoPath(t *testing.T, app *fiber.App, method, path string, body io.Reader, ctype, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, body)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func decodePhotoMeta(t *testing.T, resp *http.Response) photoMetaResponse {
	t.Helper()
	var out struct {
		Data photoMetaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	return out.Data
}

func TestPhoto_PutGetDeleteRoundTrip(t *testing.T) {
	blobs := newFakePhotoBlobs()
	app, token := photoApp(t, headshot.New(blobs, &fakePhotoRepo{}))

	body, ctype := photoUpload(t, samplePhoto(t))
	resp := doPhoto(t, app, fiber.MethodPut, body, ctype, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp.StatusCode)
	}
	meta := decodePhotoMeta(t, resp)
	if !meta.Present || meta.UploadedAt == nil {
		t.Fatalf("PUT meta = %+v, want present with an upload time", meta)
	}

	status := doPhoto(t, app, fiber.MethodGet, nil, "", token)
	defer status.Body.Close()
	if status.StatusCode != fiber.StatusOK {
		t.Fatalf("GET meta status = %d, want 200", status.StatusCode)
	}
	if m := decodePhotoMeta(t, status); !m.Enabled || !m.Present || m.UploadedAt == nil {
		t.Errorf("GET meta = %+v, want enabled+present with an upload time", m)
	}

	get := doPhotoImage(t, app, token)
	defer get.Body.Close()
	if get.StatusCode != fiber.StatusOK {
		t.Fatalf("GET image status = %d, want 200", get.StatusCode)
	}
	if ct := get.Header.Get(fiber.HeaderContentType); ct != "image/jpeg" {
		t.Errorf("GET content type = %q, want image/jpeg", ct)
	}
	// The image is private to the member; a shared cache must never keep a copy.
	if cc := get.Header.Get(fiber.HeaderCacheControl); !bytes.Contains([]byte(cc), []byte("private")) {
		t.Errorf("GET Cache-Control = %q, want it to mark the response private", cc)
	}
	served, err := io.ReadAll(get.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(served, blobs.objs["photos/1"]) {
		t.Error("GET served bytes other than the stored object")
	}

	del := doPhoto(t, app, fiber.MethodDelete, nil, "", token)
	defer del.Body.Close()
	if del.StatusCode != fiber.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", del.StatusCode)
	}
	if len(blobs.objs) != 0 {
		t.Errorf("DELETE left %d objects behind", len(blobs.objs))
	}
}

func TestPhoto_WithoutAHeadshot(t *testing.T) {
	app, token := photoApp(t, headshot.New(newFakePhotoBlobs(), &fakePhotoRepo{}))

	// The meta read is a normal state, not an error: the SPA renders "no photo yet".
	meta := doPhoto(t, app, fiber.MethodGet, nil, "", token)
	defer meta.Body.Close()
	if meta.StatusCode != fiber.StatusOK {
		t.Fatalf("GET meta status = %d, want 200", meta.StatusCode)
	}
	if m := decodePhotoMeta(t, meta); !m.Enabled || m.Present {
		t.Errorf("GET meta = %+v, want enabled and not present", m)
	}

	img := doPhotoImage(t, app, token)
	defer img.Body.Close()
	if img.StatusCode != fiber.StatusNotFound {
		t.Errorf("GET image status = %d, want 404", img.StatusCode)
	}
}

func TestPhoto_RejectsNonImageUpload(t *testing.T) {
	blobs := newFakePhotoBlobs()
	app, token := photoApp(t, headshot.New(blobs, &fakePhotoRepo{}))

	body, ctype := photoUpload(t, []byte("%PDF-1.7 this is a cover letter"))
	resp := doPhoto(t, app, fiber.MethodPut, body, ctype, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("PUT status = %d, want 400", resp.StatusCode)
	}
	if len(blobs.objs) != 0 {
		t.Errorf("a refused upload stored %d objects", len(blobs.objs))
	}
}

func TestPhoto_MissingFilePartIs400(t *testing.T) {
	app, token := photoApp(t, headshot.New(newFakePhotoBlobs(), &fakePhotoRepo{}))
	resp := doPhoto(t, app, fiber.MethodPut, bytes.NewReader(nil), "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("PUT status = %d, want 400", resp.StatusCode)
	}
}

func TestPhoto_StorageDisabled(t *testing.T) {
	app, token := photoApp(t, headshot.New(nil, &fakePhotoRepo{}))

	// The meta read still answers, reporting the feature off — that is what lets the SPA
	// omit the upload control instead of offering one that 501s.
	meta := doPhoto(t, app, fiber.MethodGet, nil, "", token)
	defer meta.Body.Close()
	if meta.StatusCode != fiber.StatusOK {
		t.Fatalf("GET meta status = %d, want 200", meta.StatusCode)
	}
	if m := decodePhotoMeta(t, meta); m.Enabled || m.Present {
		t.Errorf("GET meta = %+v, want disabled and not present", m)
	}

	body, ctype := photoUpload(t, samplePhoto(t))
	for _, c := range []struct {
		name string
		resp *http.Response
	}{
		{"PUT", doPhoto(t, app, fiber.MethodPut, body, ctype, token)},
		{"GET image", doPhotoImage(t, app, token)},
		{"DELETE", doPhoto(t, app, fiber.MethodDelete, nil, "", token)},
	} {
		if c.resp.StatusCode != fiber.StatusNotImplemented {
			t.Errorf("%s status = %d, want 501", c.name, c.resp.StatusCode)
		}
		c.resp.Body.Close()
	}
}

// headshotForTemplate is the gate between the render path and object storage: it must fetch for a
// template that prints a portrait and must not touch the bucket for one that does not.
func TestHeadshotForTemplate(t *testing.T) {
	photoTmpl, err := cv.ResolveTemplate("portrait")
	if err != nil {
		t.Fatalf("resolve portrait: %v", err)
	}
	plainTmpl, err := cv.ResolveTemplate(cv.DefaultTemplateID)
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}

	t.Run("photo template with a stored headshot", func(t *testing.T) {
		blobs := newFakePhotoBlobs()
		store := headshot.New(blobs, &fakePhotoRepo{})
		if _, err := store.Put(context.Background(), 1, samplePhoto(t)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if got := headshotForTemplate(context.Background(), store, 1, photoTmpl); len(got) == 0 {
			t.Error("no headshot handed to a photo-bearing template")
		}
	})

	t.Run("photoless template never reads storage", func(t *testing.T) {
		blobs := newFakePhotoBlobs()
		store := headshot.New(blobs, &fakePhotoRepo{})
		if _, err := store.Put(context.Background(), 1, samplePhoto(t)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		blobs.gets = 0
		if got := headshotForTemplate(context.Background(), store, 1, plainTmpl); got != nil {
			t.Errorf("a photoless template was handed %d bytes", len(got))
		}
		if blobs.gets != 0 {
			t.Errorf("object storage was read %d time(s) for a photoless template", blobs.gets)
		}
	})

	t.Run("photo template without a stored headshot", func(t *testing.T) {
		store := headshot.New(newFakePhotoBlobs(), &fakePhotoRepo{})
		if got := headshotForTemplate(context.Background(), store, 1, photoTmpl); got != nil {
			t.Errorf("expected nil (the placeholder path), got %d bytes", len(got))
		}
	})

	t.Run("storage unconfigured", func(t *testing.T) {
		store := headshot.New(nil, &fakePhotoRepo{})
		if got := headshotForTemplate(context.Background(), store, 1, photoTmpl); got != nil {
			t.Errorf("expected nil with storage disabled, got %d bytes", len(got))
		}
	})
}

// TestPhotoRegister_EveryRouteIsCookieOnly pins the gates against the real register().
// Cookie-only is the enforcement of "the headshot is PII": the CV surface admits an API key
// so the tailoring agent's CLI can drive it, and widening any of these routes the same way
// would put a member's face behind a credential that lives outside the browser. This test is
// the tripwire.
func TestPhotoRegister_EveryRouteIsCookieOnly(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&photoHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	for _, r := range []struct{ method, path string }{
		{http.MethodPut, "/api/v1/me/photo"},
		{http.MethodGet, "/api/v1/me/photo"},
		{http.MethodGet, "/api/v1/me/photo/image"},
		{http.MethodDelete, "/api/v1/me/photo"},
	} {
		resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), r.method, r.path, nil))
		if err != nil {
			t.Fatalf("%s %s: %v", r.method, r.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if got := string(body); got != "cookie" {
			t.Errorf("%s %s is gated by %q, want %q", r.method, r.path, got, "cookie")
		}
	}
}

// Every mutation is behind the auth gate, not just the read the round-trip test uses.
func TestPhoto_EveryRouteRequiresAuthentication(t *testing.T) {
	app, _ := photoApp(t, headshot.New(newFakePhotoBlobs(), &fakePhotoRepo{}))
	body, ctype := photoUpload(t, samplePhoto(t))
	for _, c := range []struct {
		name string
		resp *http.Response
	}{
		{"PUT", doPhoto(t, app, fiber.MethodPut, body, ctype, "")},
		{"GET", doPhoto(t, app, fiber.MethodGet, nil, "", "")},
		{"GET image", doPhotoImage(t, app, "")},
		{"DELETE", doPhoto(t, app, fiber.MethodDelete, nil, "", "")},
	} {
		if c.resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("unauthenticated %s status = %d, want 401", c.name, c.resp.StatusCode)
		}
		c.resp.Body.Close()
	}
}
