package maillink

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/mailclassify"
)

type fakeStore struct {
	apps        []Application
	threadLinks map[string]int64
	stage       string
	claimed     []Claimed
	claimedOnce bool
	saved       []Result
	savedOutbox []int64
}

func (s *fakeStore) EnqueuePending(context.Context) (int64, error) { return 0, nil }
func (s *fakeStore) ClaimBatch(context.Context, int, int) ([]Claimed, error) {
	if s.claimedOnce {
		return nil, nil
	}
	s.claimedOnce = true
	return s.claimed, nil
}
func (s *fakeStore) Applications(context.Context, int64) ([]Application, error) { return s.apps, nil }
func (s *fakeStore) ThreadLinks(context.Context, int64) (map[string]int64, error) {
	return s.threadLinks, nil
}
func (s *fakeStore) CurrentStage(context.Context, int64, int64) (string, error) { return s.stage, nil }
func (s *fakeStore) Save(_ context.Context, outboxID, _ int64, r Result, _ string) error {
	s.saved = append(s.saved, r)
	s.savedOutbox = append(s.savedOutbox, outboxID)
	return nil
}
func (s *fakeStore) Fail(context.Context, int64, string, int) error { return nil }

type fakeClassifier struct {
	out        mailclassify.Classification
	gotCandCnt int
	gotBody    string
}

func (c *fakeClassifier) Classify(_ context.Context, in mailclassify.Input) (mailclassify.Classification, error) {
	c.gotCandCnt = len(in.Candidates)
	c.gotBody = in.Body
	return c.out, nil
}

func TestRunnerAutoLinksDeterministicMatchAndAdvancesStage(t *testing.T) {
	store := &fakeStore{
		apps:  []Application{{JobID: 5, Company: "Acme"}},
		stage: "applied",
		claimed: []Claimed{{
			OutboxID: 100, EmailID: 200, UserID: 1,
			FromName: "Acme Hiring Team", Subject: "Acme Application Update", Body: "Great news",
		}},
	}
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalInterviewInvitation, Confidence: 0.95}}
	r := New(store, cls, "test-model")

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved %d results, want 1", len(store.saved))
	}
	got := store.saved[0]
	if got.JobID != 5 || got.LinkSource != "auto" {
		t.Errorf("link = (job=%d src=%q), want (5, auto)", got.JobID, got.LinkSource)
	}
	if got.Signal != mailclassify.SignalInterviewInvitation {
		t.Errorf("signal = %q, want interview_invitation", got.Signal)
	}
	if got.AdvanceStageTo != "interview" {
		t.Errorf("advance = %q, want interview", got.AdvanceStageTo)
	}
	if cls.gotCandCnt != 0 {
		t.Errorf("classifier got %d candidates, want 0 (already auto-linked)", cls.gotCandCnt)
	}
	if store.savedOutbox[0] != 100 {
		t.Errorf("saved outbox id = %d, want 100", store.savedOutbox[0])
	}
}

func TestRunnerFeedsHTMLOnlyBodyToClassifier(t *testing.T) {
	store := &fakeStore{
		apps: []Application{{JobID: 7, Company: "Fingerprint"}},
		claimed: []Claimed{{
			OutboxID: 102, EmailID: 202, UserID: 1,
			FromName: "Fingerprint Recruiting", Subject: "Regarding your Application to Fingerprint",
			Body: "", // HTML-only ATS mail: no plain-text part
			BodyHTML: `<html><body><p>We regret to inform you that we have ` +
				`decided not to proceed with your application.</p></body></html>`,
		}},
	}
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalRejection, Confidence: 0.9}}
	r := New(store, cls, "test-model")

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cls.gotBody == "" {
		t.Fatal("classifier received an empty body for an HTML-only email")
	}
	if !strings.Contains(cls.gotBody, "decided not to proceed") {
		t.Errorf("classifier body missing message text, got %q", cls.gotBody)
	}
	if strings.Contains(cls.gotBody, "<p>") {
		t.Errorf("classifier body should be tag-free, got %q", cls.gotBody)
	}
	if store.saved[0].Signal != mailclassify.SignalRejection {
		t.Errorf("persisted signal = %q, want rejection", store.saved[0].Signal)
	}
}

func TestRunnerAmbiguousMatchOffersLLMSuggestion(t *testing.T) {
	store := &fakeStore{
		apps: []Application{{JobID: 5, Company: "Acme"}, {JobID: 6, Company: "Acme"}},
		claimed: []Claimed{{
			OutboxID: 101, EmailID: 201, UserID: 1,
			Subject: "Thank you for applying to Acme", Body: "…",
		}},
	}
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalRejection, Confidence: 0.9, MatchedJobID: 6}}
	r := New(store, cls, "test-model")

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := store.saved[0]
	if got.JobID != 0 || got.SuggestedJobID != 6 {
		t.Errorf("link = (job=%d sug=%d), want (0, 6)", got.JobID, got.SuggestedJobID)
	}
	if got.AdvanceStageTo != "" {
		t.Errorf("advance = %q, want empty (unlinked suggestion)", got.AdvanceStageTo)
	}
	if cls.gotCandCnt != 2 {
		t.Errorf("classifier got %d candidates, want 2 (disambiguation)", cls.gotCandCnt)
	}
}
