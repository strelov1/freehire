package handler

import (
	"bytes"
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

	"github.com/strelov1/freehire/internal/auth"
)

// resumeApp mounts the extraction endpoint behind RequireAuth. The handler needs no
// service dependency (extraction is stateless: PDF/text -> skilltag), so a bare API
// with an issuer is enough.
func resumeApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/me/resume/extract", auth.RequireAuth(iss), h.ExtractResumeProfile)
	return app, token
}

func postResumeJSON(t *testing.T, app *fiber.App, body, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, "/me/resume/extract", strings.NewReader(body))
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
	w.Close()

	req := httptest.NewRequest(fiber.MethodPost, "/me/resume/extract", &buf)
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

func TestExtractResumeProfile_Unauthenticated(t *testing.T) {
	app, _ := resumeApp(t)
	resp := postResumeJSON(t, app, `{"text":"Go and PostgreSQL"}`, "")
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestExtractResumeProfile_Text(t *testing.T) {
	app, token := resumeApp(t)
	resp := postResumeJSON(t, app, `{"text":"Experienced with PostgreSQL, Kubernetes and Docker."}`, token)
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
	pdf, err := os.ReadFile("../resume/testdata/canva_cid_identityh.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	app, token := resumeApp(t)
	resp := postResumeFile(t, app, "resume.pdf", pdf, token)
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
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
