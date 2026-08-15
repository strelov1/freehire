package onboarding_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/onboarding"
)

// fakeStore serves fixed candidate lists and records what was written back.
type fakeStore struct {
	welcome, noAlert, openSource []int64
	recorded                     []db.RecordOnboardingEmailParams
	listErr                      error
	recordErr                    error

	// params captures what the runner asked for, so the tests can assert on the
	// window and caps rather than trusting them.
	welcomeParams db.ListWelcomeCandidatesParams
	noAlertParams db.ListNoAlertCandidatesParams
}

func rows(ids []int64) []db.ListWelcomeCandidatesRow {
	out := make([]db.ListWelcomeCandidatesRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, db.ListWelcomeCandidatesRow{ID: id, Email: "user@example.test"})
	}
	return out
}

func (s *fakeStore) ListWelcomeCandidates(_ context.Context, arg db.ListWelcomeCandidatesParams) ([]db.ListWelcomeCandidatesRow, error) {
	s.welcomeParams = arg
	return rows(s.welcome), s.listErr
}

func (s *fakeStore) ListNoAlertCandidates(_ context.Context, arg db.ListNoAlertCandidatesParams) ([]db.ListNoAlertCandidatesRow, error) {
	s.noAlertParams = arg
	out := make([]db.ListNoAlertCandidatesRow, 0, len(s.noAlert))
	for _, id := range s.noAlert {
		out = append(out, db.ListNoAlertCandidatesRow{ID: id, Email: "user@example.test"})
	}
	return out, nil
}

func (s *fakeStore) ListOpenSourceCandidates(_ context.Context, _ db.ListOpenSourceCandidatesParams) ([]db.ListOpenSourceCandidatesRow, error) {
	out := make([]db.ListOpenSourceCandidatesRow, 0, len(s.openSource))
	for _, id := range s.openSource {
		out = append(out, db.ListOpenSourceCandidatesRow{ID: id, Email: "user@example.test"})
	}
	return out, nil
}

func (s *fakeStore) RecordOnboardingEmail(_ context.Context, arg db.RecordOnboardingEmailParams) error {
	s.recorded = append(s.recorded, arg)
	return s.recordErr
}

// fakeSender captures every message instead of delivering it.
type fakeSender struct {
	sent []sentMail
	err  error
}

type sentMail struct{ from, replyTo, to, subject, html, text string }

func (f *fakeSender) SendWithReplyTo(_ context.Context, from, replyTo, to, subject, htmlBody, textBody string) error {
	f.sent = append(f.sent, sentMail{from, replyTo, to, subject, htmlBody, textBody})
	return f.err
}

func newRunner(store *fakeStore, sender *fakeSender) *onboarding.Runner {
	mailer := onboarding.NewMailer(sender, "notifications@freehire.me", "ilya@example.test", "https://freehire.me")
	return onboarding.New(store, mailer, onboarding.DefaultConfig())
}

func TestRun_SendsOneMailPerCandidatePerStep(t *testing.T) {
	store := &fakeStore{welcome: []int64{1, 2}, noAlert: []int64{3}, openSource: []int64{4}}
	sender := &fakeSender{}

	stats, err := newRunner(store, sender).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sent) != 4 {
		t.Fatalf("sent %d mails, want 4", len(sender.sent))
	}
	if stats.Sent[onboarding.StepWelcome] != 2 || stats.Sent[onboarding.StepNoAlert] != 1 {
		t.Errorf("stats = %+v, want 2 welcome and 1 no_alert", stats.Sent)
	}
	if len(store.recorded) != 4 {
		t.Errorf("recorded %d ledger rows, want one per send", len(store.recorded))
	}
}

// The ledger row is what stops a mail being sent twice, so it must be written even
// when the send failed. Without this, one bad address is retried on every pass —
// which is how a sending domain collects bounces until SES throttles the account.
func TestRun_RecordsTheLedgerEvenWhenTheSendFails(t *testing.T) {
	store := &fakeStore{welcome: []int64{7}}
	sender := &fakeSender{err: errors.New("ses is down")}

	stats, err := newRunner(store, sender).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed[onboarding.StepWelcome] != 1 {
		t.Errorf("stats.Failed = %+v, want the failure counted", stats.Failed)
	}
	if len(store.recorded) != 1 {
		t.Fatalf("recorded %d rows, want the failed send burned too", len(store.recorded))
	}
	if !strings.Contains(store.recorded[0].Error, "ses is down") {
		t.Errorf("ledger error = %q, want the transport failure kept for diagnosis", store.recorded[0].Error)
	}
}

// One unsendable address must not strand the rest of the batch.
func TestRun_ContinuesPastASingleFailure(t *testing.T) {
	store := &fakeStore{welcome: []int64{1, 2, 3}}
	sender := &fakeSender{err: errors.New("nope")}

	if _, err := newRunner(store, sender).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sender.sent) != 3 {
		t.Errorf("attempted %d sends, want all 3 tried", len(sender.sent))
	}
}

// A broken candidate query is a broken query — every subsequent step would hit the
// same wall, so the pass aborts instead of grinding through it.
func TestRun_AbortsWhenACandidateQueryFails(t *testing.T) {
	store := &fakeStore{listErr: errors.New("relation does not exist")}

	if _, err := newRunner(store, &fakeSender{}).Run(context.Background()); err == nil {
		t.Fatal("a failing candidate query must surface, not be swallowed")
	}
}

// The window is the guard that keeps a first deploy from greeting years of
// historical signups. It is a config value that only this assertion protects.
func TestRun_BoundsCandidatesByTheSignupWindow(t *testing.T) {
	store := &fakeStore{}
	if _, err := newRunner(store, &fakeSender{}).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	cfg := onboarding.DefaultConfig()
	if store.welcomeParams.WindowDays != cfg.WindowDays {
		t.Errorf("welcome window = %d days, want %d", store.welcomeParams.WindowDays, cfg.WindowDays)
	}
	if store.welcomeParams.MaxRows != cfg.MaxPerStep {
		t.Errorf("welcome cap = %d, want %d", store.welcomeParams.MaxRows, cfg.MaxPerStep)
	}
	if store.noAlertParams.AfterDays != cfg.NoAlertAfterDays {
		t.Errorf("no_alert delay = %d days, want %d", store.noAlertParams.AfterDays, cfg.NoAlertAfterDays)
	}
}

// These mails ask the reader a question. A reply that goes back to the no-reply
// sender is read by the application-mail parser, not by a person.
func TestRun_MailsCarryTheReplyToAddress(t *testing.T) {
	store := &fakeStore{welcome: []int64{1}}
	sender := &fakeSender{}

	if _, err := newRunner(store, sender).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := sender.sent[0].replyTo; got != "ilya@example.test" {
		t.Errorf("Reply-To = %q, want the human inbox", got)
	}
}

func TestRun_EachStepHasItsOwnSubjectAndBothBodies(t *testing.T) {
	store := &fakeStore{welcome: []int64{1}, noAlert: []int64{2}, openSource: []int64{3}}
	sender := &fakeSender{}

	if _, err := newRunner(store, sender).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	seen := map[string]bool{}
	for _, m := range sender.sent {
		if strings.TrimSpace(m.subject) == "" {
			t.Error("a mail went out with no subject")
		}
		if strings.TrimSpace(m.text) == "" {
			t.Errorf("%q has no plain-text body", m.subject)
		}
		if !strings.Contains(m.html, "<!DOCTYPE html>") {
			t.Errorf("%q is a fragment, not a document", m.subject)
		}
		if seen[m.subject] {
			t.Errorf("two steps share the subject %q", m.subject)
		}
		seen[m.subject] = true
	}
}
