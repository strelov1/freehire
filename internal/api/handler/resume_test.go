package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// resumeApp mounts the extraction endpoint through the real register, behind a real
// optional session gate. The handler needs no service dependency (extraction is
// stateless: PDF/text -> skilltag), so a bare API with an issuer is enough.
func resumeApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	return resumeAppWith(t, &resumeHandlers{}, nil)
}

// resumeAppWith is resumeApp over a given handler and throttler. Requests may carry an
// X-Forwarded-For, which the app believes, so a test can speak as distinct addresses.
func resumeAppWith(t *testing.T, h *resumeHandlers, throttler ratelimit.Throttler) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError, ProxyHeader: fiber.HeaderXForwardedFor})
	cookie := auth.RequireAuth(iss, testVersions)
	h.register(app, middleware{
		optional:  auth.OptionalAuth(iss, testVersions, nil),
		cookie:    cookie,
		key:       cookie,
		paidAI:    markPaidAI,
		throttler: throttler,
	})
	return app, token
}

func postResumeJSON(t *testing.T, app *fiber.App, body, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/me/resume/extract", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func postResumeFile(t *testing.T, app *fiber.App, filename string, content []byte, token string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = w.Close()

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/me/resume/extract", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func decodeSkills(t *testing.T, resp *http.Response) []string {
	t.Helper()
	var got struct {
		Data struct {
			Skills []string `json:"skills"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got.Data.Skills
}

// A signed-out visitor is served — the AI search lets a guest attach a CV — and nothing
// of theirs is kept: storage is configured here, and the guest path must not touch it.
func TestExtractResumeProfile_GuestIsServedAndNothingStored(t *testing.T) {
	blobs := newFakeResumeBlobs()
	repo := &fakeResumeRepo{}
	app, _ := resumeAppWith(t, &resumeHandlers{resume: resume.New(blobs, repo)}, nil)

	resp := postResumeJSON(t, app, `{"text":"Experienced with PostgreSQL, Kubernetes and Docker."}`, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := decodeSkills(t, resp); strings.Join(got, ",") != "docker,kubernetes,postgresql" {
		t.Errorf("skills = %v, want [docker kubernetes postgresql]", got)
	}
	if len(blobs.objs) != 0 {
		t.Errorf("stored %d objects for a guest, want none", len(blobs.objs))
	}
	if repo.set {
		t.Error("recorded a résumé pointer for a guest")
	}
}

// A signed-in caller is unchanged: with storage configured, the upload is stored.
func TestExtractResumeProfile_SignedInStillStores(t *testing.T) {
	blobs := newFakeResumeBlobs()
	app, token := resumeAppWith(t, &resumeHandlers{resume: resume.New(blobs, &fakeResumeRepo{})}, nil)

	resp := postResumeJSON(t, app, `{"text":"Experienced with PostgreSQL."}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(blobs.objs) != 1 {
		t.Errorf("stored %d objects for a signed-in caller, want 1", len(blobs.objs))
	}
}

// A guest spends a per-IP budget of 20 an hour; a signed-in caller behind the same
// address does not draw on it.
func TestExtractResumeProfile_GuestsAreLimitedPerIP(t *testing.T) {
	app, token := resumeAppWith(t, &resumeHandlers{}, newTestThrottler(t))
	post := func(ip, token string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost,
			"/me/resume/extract", strings.NewReader(`{"text":"Go and PostgreSQL"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(fiber.HeaderXForwardedFor, ip)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for i := 1; i <= guestResumeExtractsPerHour; i++ {
		if got := post("203.0.113.7", ""); got != fiber.StatusOK {
			t.Fatalf("guest call %d = %d, want 200", i, got)
		}
	}
	if got := post("203.0.113.7", ""); got != fiber.StatusTooManyRequests {
		t.Fatalf("guest call %d = %d, want 429", guestResumeExtractsPerHour+1, got)
	}
	if got := post("203.0.113.8", ""); got != fiber.StatusOK {
		t.Fatalf("a guest on another address = %d, want 200 — the budget is per IP", got)
	}
	if got := post("203.0.113.7", token); got != fiber.StatusOK {
		t.Fatalf("a signed-in caller behind the exhausted address = %d, want 200", got)
	}
}

// A guest may not post an encoded body, for the reason RoastCV refuses one: Fiber
// decompresses it with no ceiling, and no account stands behind the request.
func TestExtractResumeProfile_RefusesAGuestsEncodedBody(t *testing.T) {
	app, _ := resumeApp(t)
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost,
		"/me/resume/extract", strings.NewReader(`{"text":"Go"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderContentEncoding, "gzip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestExtractResumeProfile_Text(t *testing.T) {
	app, token := resumeApp(t)
	resp := postResumeJSON(t, app, `{"text":"Experienced with PostgreSQL, Kubernetes and Docker."}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeSkills(t, resp)
	if strings.Join(got, ",") != "docker,kubernetes,postgresql" {
		t.Errorf("skills = %v, want [docker kubernetes postgresql]", got)
	}
}

func TestExtractResumeProfile_TextNoSkills(t *testing.T) {
	app, token := resumeApp(t)
	resp := postResumeJSON(t, app, `{"text":"I like long walks on the beach."}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// Must serialize as an empty array, not null, so the client always gets a list.
	if !strings.Contains(string(body), `"skills":[]`) {
		t.Errorf("body = %s, want skills:[]", body)
	}
}

func TestExtractResumeProfile_EmptyText(t *testing.T) {
	app, token := resumeApp(t)
	resp := postResumeJSON(t, app, `{"text":"   "}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestExtractResumeProfile_PDF(t *testing.T) {
	pdf, err := os.ReadFile("testdata/resume_sample.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	app, token := resumeApp(t)
	resp := postResumeFile(t, app, "resume.pdf", pdf, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeSkills(t, resp)
	if strings.Join(got, ",") != "docker,kubernetes,postgresql" {
		t.Errorf("skills = %v, want [docker kubernetes postgresql]", got)
	}
}

// TestExtractResumeProfile_CanvaCIDPDF exercises the real upload endpoint (the single path
// both the profile form and the onboarding wizard POST to) with a Canva export whose fonts
// are subset CID TrueType / Identity-H. The old pure-Go parser returned empty text for
// these, so the endpoint answered 400 "scan or image"; via pdftotext it must now yield the
// résumé's skills. Skipped when poppler is absent (mirrors the extractor's own test).
func TestExtractResumeProfile_CanvaCIDPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not on PATH; skipping Canva PDF upload test")
	}
	pdf, err := os.ReadFile("../../candidate/resume/testdata/canva_cid_identityh.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	app, token := resumeApp(t)
	resp := postResumeFile(t, app, "resume.pdf", pdf, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (Canva CID PDF must extract, not be rejected)", resp.StatusCode)
	}
	if got := decodeSkills(t, resp); len(got) == 0 {
		t.Error("no skills extracted from the Canva résumé; text layer was not decoded")
	}
}

func TestExtractResumeProfile_MalformedPDF(t *testing.T) {
	app, token := resumeApp(t)
	resp := postResumeFile(t, app, "resume.pdf", []byte("this is not a pdf"), token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
