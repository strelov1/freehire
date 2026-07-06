package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/resume"
)

// resumeEmbedApp wires PutResume with both a résumé store and a searcher, so the
// upload's best-effort CV-embedding hook is exercised.
func resumeEmbedApp(t *testing.T, store *resume.Store, s searcher) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, resume: store, search: s}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Put("/me/resume", auth.RequireAuth(iss), h.PutResume)
	return app, token
}

// A CV upload embeds the CV text through the searcher and persists the resulting
// vector plus the embedder identity.
func TestPutResume_EmbedsAndPersistsVector(t *testing.T) {
	repo := &fakeResumeRepo{}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{embedVec: []float64{0.1, 0.2, 0.3}, embedModel: "test-embedder"}
	app, token := resumeEmbedApp(t, store, fs)

	if status, _ := resumeReq(t, app, fiber.MethodPut, `{"text":"Go and PostgreSQL and Kubernetes"}`, token); status != fiber.StatusOK {
		t.Fatalf("upload status = %d, want 200", status)
	}
	if fs.gotEmbedText == "" {
		t.Error("CV text was not sent to the embedder")
	}
	if repo.embModel != "test-embedder" || len(repo.embVec) != 3 {
		t.Errorf("persisted embedding = (%v, %q), want the fake vector + model", repo.embVec, repo.embModel)
	}
}

// An embedder failure must not fail the upload, and it clears any prior vector so the
// new CV is never matched by a stale one.
func TestPutResume_EmbedFailureClearsVector(t *testing.T) {
	repo := &fakeResumeRepo{embVec: []float64{9, 9}, embModel: "stale-model"}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{embedErr: errors.New("embedder down")}
	app, token := resumeEmbedApp(t, store, fs)

	if status, _ := resumeReq(t, app, fiber.MethodPut, `{"text":"resume text"}`, token); status != fiber.StatusOK {
		t.Fatalf("upload status = %d, want 200 despite embed failure", status)
	}
	if repo.embVec != nil || repo.embModel != "" {
		t.Errorf("stale vector not cleared: (%v, %q)", repo.embVec, repo.embModel)
	}
}
