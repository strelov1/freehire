package handler

import (
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/resume"
)

// embedResume is called (backgrounded) from PutResume; tested directly so the
// assertion is synchronous. A successful embed persists the vector + embedder model.
func TestEmbedResume_PersistsVector(t *testing.T) {
	repo := &fakeResumeRepo{}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{embedVec: []float64{0.1, 0.2, 0.3}, embedModel: "test-embedder"}
	h := &API{resume: store, search: fs}

	h.embedResume(1, "Go and PostgreSQL and Kubernetes")

	if fs.gotEmbedText == "" {
		t.Error("CV text was not sent to the embedder")
	}
	if repo.embModel != "test-embedder" || len(repo.embVec) != 3 {
		t.Errorf("persisted embedding = (%v, %q), want the fake vector + model", repo.embVec, repo.embModel)
	}
}

// An embedder failure clears any prior vector so the new CV is never matched by a stale
// one (and never surfaces to the user — it is best-effort, logged).
func TestEmbedResume_FailureClearsVector(t *testing.T) {
	repo := &fakeResumeRepo{embVec: []float64{9, 9}, embModel: "stale-model"}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{embedErr: errors.New("embedder down")}
	h := &API{resume: store, search: fs}

	h.embedResume(1, "resume text")

	if repo.embVec != nil || repo.embModel != "" {
		t.Errorf("stale vector not cleared: (%v, %q)", repo.embVec, repo.embModel)
	}
}

// No search backend → embedResume is a no-op (never panics, persists nothing).
func TestEmbedResume_NoSearchBackend(t *testing.T) {
	repo := &fakeResumeRepo{}
	h := &API{resume: resume.New(newFakeResumeBlobs(), repo), search: nil}
	h.embedResume(1, "text") // must not panic
	if repo.embModel != "" || repo.embVec != nil {
		t.Error("no-op embedResume must not persist anything")
	}
}
