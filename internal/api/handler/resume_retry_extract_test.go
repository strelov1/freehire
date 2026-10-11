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

// A client polling GET /me/resume right after the retry responds must see 'pending', not
// the PREVIOUS attempt's 'failed' — otherwise it reads a stale terminal status and never
// learns the retry's real outcome. Reading the status at one instant would race the
// background derivation goroutine (which, with no structuredExtractor, can settle into its
// OWN 'failed' write within microseconds) — so this asserts on the ORDER of writes instead:
// 'pending' must appear before the derivation's terminal write, regardless of how fast that
// write lands.
func TestRetryResumeExtract_MarksPendingBeforeBackgroundDerivationStarts(t *testing.T) {
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
		done := repo.extractDetail == "extractor unavailable"
		history := append([]string(nil), repo.statusHistory...)
		repo.mu.Unlock()
		if done {
			if len(history) == 0 || history[0] != resume.ExtractStatusPending {
				t.Fatalf("status history = %v, want it to start with %q (the retry's own clear) "+
					"before any terminal write the background derivation makes",
					history, resume.ExtractStatusPending)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("background derivation never settled; status history = %v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// With no structuredExtractor configured, a triggered derivation settles into
// ExtractStatusFailed/"extractor unavailable" — the same outcome PutResume's own background
// derivation reaches when disabled. Reaching that same outcome from the retry endpoint is
// proof that it invoked the shared background path with a non-nil uploadedAt (a nil one
// would panic inside extractStructuredResume, not settle quietly) — not proof of what text
// was passed; the nil-extractor branch returns before ever looking at the text argument.
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
