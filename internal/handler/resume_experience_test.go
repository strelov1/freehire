package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/resume"

	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeBank records what the upload path handed the experience bank. The mapping itself is
// tested in internal/experience; what matters here is whether the import ran at all, and
// that it never surfaces to the upload when it cannot.
type fakeBank struct {
	calls     int
	entries   []experience.ImportEntry
	sourceRef string
	err       error
	history   []resumeextract.Experience
}

func (f *fakeBank) WorkHistory(context.Context, int64) ([]resumeextract.Experience, error) {
	return f.history, nil
}

func (f *fakeBank) Import(_ context.Context, _ int64, entries []experience.ImportEntry, sourceRef string) (experience.ImportResult, error) {
	f.calls++
	f.entries = entries
	f.sourceRef = sourceRef
	return experience.ImportResult{}, f.err
}

func bankFixture() resumeextract.Structured {
	return resumeextract.Structured{
		Experience: []resumeextract.Experience{
			{Title: "Senior Software Engineer", Company: "RingCentral", End: "Present",
				Highlights: []string{"Cut latency 20s to 1s"}},
		},
	}
}

func TestImportExperienceRunsOnASuccessfulExtract(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, bankFixture(), "2026-07-28T10:00:00Z")

	if bank.calls != 1 {
		t.Fatalf("bank.calls = %d, want 1", bank.calls)
	}
	if bank.sourceRef != "2026-07-28T10:00:00Z" {
		t.Errorf("sourceRef = %q, want the upload stamp", bank.sourceRef)
	}
	if len(bank.entries) != 1 {
		t.Errorf("entries = %d, want 1", len(bank.entries))
	}
}

// The bank is best-effort exactly like the rest of the derive path: an unconfigured bank
// or a failing import must never surface to the upload.
func TestImportExperienceIsBestEffort(t *testing.T) {
	h := &resumeHandlers{bank: nil}
	h.importExperience(context.Background(), 7, bankFixture(), "stamp") // must not panic

	failing := &fakeBank{err: errors.New("database down")}
	h = &resumeHandlers{bank: failing}
	h.importExperience(context.Background(), 7, bankFixture(), "stamp")
	if failing.calls != 1 {
		t.Errorf("bank.calls = %d, want the failure swallowed after one attempt", failing.calls)
	}
}

// An extraction that produced no work history has nothing to bank; the import must not
// run at all rather than call with an empty slice.
func TestImportExperienceSkipsAnEmptyStructure(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, resumeextract.Structured{FullName: "Ilya Strelov"}, "stamp")

	if bank.calls != 0 {
		t.Errorf("bank.calls = %d, want 0 — nothing was extracted to bank", bank.calls)
	}
}

// resumeAppWithBank wires the status surface over a stub bank, so the one behaviour that
// changed can be exercised: where the served work history comes from.
func resumeAppWithBank(t *testing.T, store *resume.Store, bank experienceBank) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &resumeHandlers{resume: store, bank: bank}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/resume", auth.RequireAuth(iss, testVersions), h.GetResume)
	return app, token
}

// The staleness rule used to hide everything: a newer CV whose extraction had not landed
// left the profile with no parsed résumé at all. The bank does not answer to that stamp,
// so the career survives the window while the file-owned sections wait for the extract.
func TestGetResumeServesBankedExperienceThroughAStaleWindow(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{set: true}) // a stored CV, no current structure
	bank := &fakeBank{history: []resumeextract.Experience{
		{Company: "RingCentral", Title: "Senior Software Engineer", Highlights: []string{"Cut latency 20s to 1s"}},
	}}
	app, token := resumeAppWithBank(t, store, bank)

	req := httptest.NewRequest(http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data struct {
			Structured *resumeextract.Structured `json:"structured"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Structured == nil {
		t.Fatal("structured is null though the bank holds a career")
	}
	if len(body.Data.Structured.Experience) != 1 || body.Data.Structured.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked role", body.Data.Structured.Experience)
	}
}
