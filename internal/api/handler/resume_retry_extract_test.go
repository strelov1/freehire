package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// retryResumeExtractApp mirrors resumeStorageApp, plus the retry route.
func retryResumeExtractApp(t *testing.T, store *resume.Store) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &resumeHandlers{resume: store}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Post("/me/resume/retry-extract", g, h.RetryResumeExtract)
	return app, token
}

func retryReq(t *testing.T, app *fiber.App, token string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/me/resume/retry-extract", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestRetryResumeExtract_NoResumeStoredIsRejected(t *testing.T) {
	repo := &fakeResumeRepo{}
	store := resume.New(newFakeResumeBlobs(), repo)
	app, token := retryResumeExtractApp(t, store)

	status := retryReq(t, app, token)
	if status == fiber.StatusOK || status == fiber.StatusAccepted || status == fiber.StatusNoContent {
		t.Fatalf("retry with no résumé stored = %d, want a client error", status)
	}

	repo.mu.Lock()
	touched := repo.extractStatus != "" || repo.extractDetail != ""
	repo.mu.Unlock()
	if touched {
		t.Fatal("retry with no résumé stored must not touch extract status at all")
	}
}

// With no structuredExtractor configured, a triggered derivation settles into
// ExtractStatusFailed/"extractor unavailable" — the same outcome PutResume's own background
// derivation reaches when disabled. Reaching that same outcome from the retry endpoint is the
// proof that it actually re-derived text from the stored upload and invoked the shared
// background path, not a no-op.
func TestRetryResumeExtract_TriggersDerivationFromStoredUpload(t *testing.T) {
	repo := &fakeResumeRepo{
		key: "resumes/1", set: true,
		extractStatus: resume.ExtractStatusFailed, extractDetail: "extract failed",
		extractFor: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true},
	}
	blobs := newFakeResumeBlobs()
	blobs.objs["resumes/1"] = []byte("Jane Doe\nSoftware Engineer")
	store := resume.New(blobs, repo)
	app, token := retryResumeExtractApp(t, store)

	status := retryReq(t, app, token)
	if status != fiber.StatusOK && status != fiber.StatusAccepted && status != fiber.StatusNoContent {
		t.Fatalf("retry with a stored résumé = %d, want success", status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		repo.mu.Lock()
		detail := repo.extractDetail
		repo.mu.Unlock()
		if detail == "extractor unavailable" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("extract detail = %q after waiting, want the background derivation to have run "+
				"(nil structuredExtractor settles into \"extractor unavailable\")", detail)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
