package maillink

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/mailclassify"
)

type fakeStore struct {
	appsReads   int
	threadReads int
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
func (s *fakeStore) Applications(context.Context, int64) ([]Application, error) {
	s.appsReads++
	return s.apps, nil
}
func (s *fakeStore) ThreadLinks(context.Context, int64) (map[string]int64, error) {
	s.threadReads++
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

type fakeLearner struct{ learned []string }

func (l *fakeLearner) Learn(_ context.Context, fromAddr string) error {
	l.learned = append(l.learned, fromAddr)
	return nil
}

func TestRunnerLearnsSenderOfApplicationMail(t *testing.T) {
	store := &fakeStore{claimed: []Claimed{{
		OutboxID: 100, EmailID: 200, UserID: 1,
		FromAddr: "no-reply@newats.example", Subject: "Thanks for applying", Body: "…",
	}}}
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalAcknowledgement, Confidence: 0.9}}
	learner := &fakeLearner{}
	r := New(store, cls, "test-model").WithLearner(learner)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(learner.learned) != 1 || learner.learned[0] != "no-reply@newats.example" {
		t.Errorf("learned = %v, want [no-reply@newats.example]", learner.learned)
	}
}

func TestRunnerDoesNotLearnNonApplicationMail(t *testing.T) {
	store := &fakeStore{claimed: []Claimed{{
		OutboxID: 101, EmailID: 201, UserID: 1,
		FromAddr: "news@newsletter.example", Subject: "Weekly digest", Body: "…",
	}}}
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalOther, Confidence: 0.9}}
	learner := &fakeLearner{}
	r := New(store, cls, "test-model").WithLearner(learner)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(learner.learned) != 0 {
		t.Errorf("learned %v, want none (signal was 'other')", learner.learned)
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

// The caller's applications and their thread links are per-user, and a wave is
// mostly one user's mail — but only ONE of the two may be cached.
//
// Applications is stable for the length of a run: membership is "applied, saved or
// staged", and the only write a run makes to it is a forward stage move on a row
// that is already a member. ThreadLinks is NOT: Save writes job_id onto the email
// it just linked, so a later message in the same thread must be able to see that
// link and continue it. Caching both looks like the same optimisation and quietly
// costs thread continuity within a wave.
func TestRunnerReadsApplicationsOncePerUserButThreadLinksEveryTime(t *testing.T) {
	store := &fakeStore{
		claimed: []Claimed{
			{OutboxID: 1, EmailID: 11, UserID: 7, Subject: "Thanks for applying to Acme"},
			{OutboxID: 2, EmailID: 12, UserID: 7, Subject: "Thanks for applying to Acme"},
			{OutboxID: 3, EmailID: 13, UserID: 9, Subject: "Thanks for applying to Acme"},
		},
		apps: []Application{{JobID: 42, Company: "Acme"}},
	}
	r := New(store, &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalAcknowledgement}}, "test-model")

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if store.appsReads != 2 {
		t.Errorf("Applications read %d times for 2 users over 3 emails, want 2", store.appsReads)
	}
	if store.threadReads != 3 {
		t.Errorf("ThreadLinks read %d times for 3 emails, want 3 — a link written mid-wave must be visible to the rest of it",
			store.threadReads)
	}
}
