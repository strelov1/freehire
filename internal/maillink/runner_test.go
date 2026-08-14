package maillink

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/mailclassify"
)

type fakeStore struct {
	appsReads       int
	threadReads     int
	apps            []Application
	threadLinks     map[string]int64
	stage           string
	claimed         []Claimed
	claimedOnce     bool
	gotLeaseSeconds int
	gotBatchSize    int
	saved           []Result
	savedOutbox     []int64
	failCalls       int
	deadLetter      bool
	failErr         error
}

func (s *fakeStore) EnqueuePending(context.Context) (int64, error) { return 0, nil }
func (s *fakeStore) ClaimBatch(_ context.Context, leaseSeconds, batchSize int) ([]Claimed, error) {
	s.gotLeaseSeconds = leaseSeconds
	s.gotBatchSize = batchSize
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
	// Mirrors the real Store: linking an email writes job_id onto it, and
	// ThreadLinks derives the thread->job map from already-written emails — so a
	// link this save just made must be visible to ThreadLinks for the rest of the
	// wave. Looked up by outboxID since Result carries no ThreadID of its own.
	if r.JobID != 0 {
		for _, c := range s.claimed {
			if c.OutboxID == outboxID {
				if s.threadLinks == nil {
					s.threadLinks = map[string]int64{}
				}
				s.threadLinks[c.ThreadID] = r.JobID
				break
			}
		}
	}
	return nil
}

// failCalls records every Fail the run made, and deadLetter decides what each reports — the
// signal the worker's exit code is built from.
func (s *fakeStore) Fail(context.Context, int64, string, int) (bool, error) {
	s.failCalls++
	return s.deadLetter, s.failErr
}

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

	if _, err := r.Run(context.Background()); err != nil {
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

	if _, err := r.Run(context.Background()); err != nil {
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

	if _, err := r.Run(context.Background()); err != nil {
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

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(learner.learned) != 0 {
		t.Errorf("learned %v, want none (signal was 'other')", learner.learned)
	}
}

func TestRunnerDoesNotLearnLowConfidenceSignal(t *testing.T) {
	store := &fakeStore{claimed: []Claimed{{
		OutboxID: 102, EmailID: 202, UserID: 1,
		FromAddr: "no-reply@uncertain.example", Subject: "re: your message", Body: "…",
	}}}
	// A non-"other" signal the classifier itself is unsure about: below cfg.stage
	// (0.8), the same bar this package already requires before acting automatically
	// on a classification (see stageAdvance). Three such guesses on one sender must
	// not be enough to promote its domain — see internal/gmailsync/learn.go's
	// "confident job-mail sightings" gate.
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalAcknowledgement, Confidence: 0.3}}
	learner := &fakeLearner{}
	r := New(store, cls, "test-model").WithLearner(learner)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(learner.learned) != 0 {
		t.Errorf("learned %v, want none (confidence 0.3 is below the learn threshold)", learner.learned)
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

	if _, err := r.Run(context.Background()); err != nil {
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

	if _, err := r.Run(context.Background()); err != nil {
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

// This is the property the whole migration onto outbox.RunPool's sequential
// (Concurrency: 1, no-goroutine) branch exists to preserve: a second email in the
// same thread, with no name match of its own, must still resolve to the first
// email's job via thread continuity — which only works if the first email's Save
// (writing the link) fully completes before the second email's ThreadLinks read
// begins. A concurrent (or reordered) implementation would make this flaky at best.
func TestRunnerAppliesThreadContinuityWithinTheSameWave(t *testing.T) {
	store := &fakeStore{
		apps: []Application{{JobID: 5, Company: "Acme"}},
		claimed: []Claimed{
			{
				OutboxID: 1, EmailID: 100, UserID: 1, ThreadID: "t1",
				FromName: "Acme Hiring Team", Subject: "Acme Application Update",
			},
			{
				// No extractable/matching company name of its own — can only resolve via
				// the thread-continuity tier, which requires message 1's freshly-written
				// link to already be visible.
				OutboxID: 2, EmailID: 101, UserID: 1, ThreadID: "t1",
				FromName: "Jane Doe", Subject: "Re: your interview",
			},
		},
	}
	cls := &fakeClassifier{out: mailclassify.Classification{Signal: mailclassify.SignalInterviewInvitation, Confidence: 0.95}}
	r := New(store, cls, "test-model")

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved %d results, want 2", len(store.saved))
	}
	if store.saved[0].JobID != 5 {
		t.Fatalf("message 1 = %+v, want auto-linked to job 5 by name match", store.saved[0])
	}
	if store.saved[1].JobID != 5 {
		t.Errorf("message 2 = %+v, want linked to job 5 via same-wave thread continuity", store.saved[1])
	}
}

// claimAdapter bridges Store.ClaimBatch's reversed argument order
// (leaseSeconds, batchSize) to outbox.Claimer.Claim's (batch, leaseSeconds). Both are
// plain ints, so a positional swap here would compile silently and pass any test that
// doesn't check which value landed where.
func TestClaimAdapterPassesBatchAndLeaseSecondsToCorrectPositions(t *testing.T) {
	store := &fakeStore{}
	adapter := claimAdapter{store: store}

	if _, err := adapter.Claim(context.Background(), 7, 42); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if store.gotBatchSize != 7 {
		t.Errorf("Store.ClaimBatch's batchSize = %d, want 7", store.gotBatchSize)
	}
	if store.gotLeaseSeconds != 42 {
		t.Errorf("Store.ClaimBatch's leaseSeconds = %d, want 42", store.gotLeaseSeconds)
	}
}

// A queue that fails everything must say so upward. Nothing else reads
// email_classification_outbox.failed_at, so before this the worker logged "done" and returned 0
// while every entry dead-lettered — the state worker.ExitCode exists to surface.
func TestRunnerTalliesFailuresAndDeadLetters(t *testing.T) {
	store := &fakeStore{
		deadLetter: true,
		claimed: []Claimed{
			{OutboxID: 1, EmailID: 11, UserID: 1, Subject: "a"},
			{OutboxID: 2, EmailID: 12, UserID: 1, Subject: "b"},
		},
	}
	r := New(store, &explodingClassifier{}, "test-model")

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Failed and DeadLettered are mutually exclusive (see Stats' doc comment) — a
	// dead-lettered entry counts only in DeadLettered, not also in Failed.
	if stats.Failed != 0 || stats.DeadLettered != 2 {
		t.Errorf("stats = %+v, want Failed=0 DeadLettered=2", stats)
	}
	if store.failCalls != 2 {
		t.Errorf("Fail called %d times, want 2", store.failCalls)
	}
}

// A failure that has NOT exhausted its attempts is a retry, not a dead letter. Counting it as
// one would fail the run for a queue that is working exactly as designed.
func TestRunnerCountsARetryableFailureSeparately(t *testing.T) {
	store := &fakeStore{
		deadLetter: false,
		claimed:    []Claimed{{OutboxID: 1, EmailID: 11, UserID: 1, Subject: "a"}},
	}
	r := New(store, &explodingClassifier{}, "test-model")

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 1 || stats.DeadLettered != 0 {
		t.Errorf("stats = %+v, want Failed=1 DeadLettered=0", stats)
	}
}

// When the bookkeeping write itself fails the entry is left to its lease expiry, so its
// dead-letter state is unknown — and unknown is not guessed. It still counts as failed.
func TestRunnerDoesNotGuessDeadLetterWhenFailItselfFails(t *testing.T) {
	store := &fakeStore{
		deadLetter: true, // would have said yes, had the write landed
		failErr:    errContrived,
		claimed:    []Claimed{{OutboxID: 1, EmailID: 11, UserID: 1, Subject: "a"}},
	}
	r := New(store, &explodingClassifier{}, "test-model")

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 1 || stats.DeadLettered != 0 {
		t.Errorf("stats = %+v, want Failed=1 DeadLettered=0 — the write never landed", stats)
	}
}

var errContrived = errors.New("contrived bookkeeping failure")

// explodingClassifier fails every message, which is how the tests above drive the failure path.
type explodingClassifier struct{}

func (explodingClassifier) Classify(context.Context, mailclassify.Input) (mailclassify.Classification, error) {
	return mailclassify.Classification{}, errContrived
}
