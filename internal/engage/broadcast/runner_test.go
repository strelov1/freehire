package broadcast_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/broadcast"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeStore struct {
	ids       []int64
	recorded  []db.RecordBroadcastEmailParams
	listErr   error
	countErr  error
	lastQuery db.ListBroadcastCandidatesParams
	remaining int64
}

func (s *fakeStore) ListBroadcastCandidates(_ context.Context, arg db.ListBroadcastCandidatesParams) ([]db.ListBroadcastCandidatesRow, error) {
	s.lastQuery = arg
	out := make([]db.ListBroadcastCandidatesRow, 0, len(s.ids))
	for _, id := range s.ids {
		out = append(out, db.ListBroadcastCandidatesRow{ID: id, Email: "user@example.test"})
	}
	return out, s.listErr
}

func (s *fakeStore) CountBroadcastCandidates(_ context.Context, _ string) (int64, error) {
	return s.remaining, s.countErr
}

func (s *fakeStore) RecordBroadcastEmail(_ context.Context, arg db.RecordBroadcastEmailParams) error {
	s.recorded = append(s.recorded, arg)
	return nil
}

type fakeSender struct {
	sent []sentMail
	err  error
}

type sentMail struct{ from, replyTo, to, subject, html, text string }

func (f *fakeSender) SendWithReplyTo(_ context.Context, from, replyTo, to, subject, htmlBody, textBody string) error {
	f.sent = append(f.sent, sentMail{from, replyTo, to, subject, htmlBody, textBody})
	return f.err
}

func newRunner(store *fakeStore, sender *fakeSender, max int32) *broadcast.Runner {
	m := broadcast.NewMailer(sender, "notifications@freehire.me", "ilya@example.test", "https://freehire.me")
	return broadcast.New(store, m, max)
}

func campaign(t *testing.T, name string) broadcast.Campaign {
	t.Helper()
	c, ok := broadcast.Lookup(name)
	if !ok {
		t.Fatalf("campaign %q is not registered", name)
	}
	return c
}

func TestRun_SendsToEveryCandidateAndRecordsThem(t *testing.T) {
	store := &fakeStore{ids: []int64{1, 2, 3}}
	sender := &fakeSender{}

	stats, err := newRunner(store, sender, 0).Run(context.Background(), campaign(t, "hiring-season-september"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Sent != 3 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want 3 sent", stats)
	}
	if len(store.recorded) != 3 {
		t.Errorf("recorded %d rows, want one per recipient", len(store.recorded))
	}
}

// The ledger is the only thing preventing a second copy reaching the whole list.
func TestRun_BurnsTheLedgerEvenOnFailure(t *testing.T) {
	store := &fakeStore{ids: []int64{9}}
	sender := &fakeSender{err: errors.New("ses rejected it")}

	stats, err := newRunner(store, sender, 0).Run(context.Background(), campaign(t, "hiring-season-september"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("stats = %+v, want the failure counted", stats)
	}
	if len(store.recorded) != 1 || !strings.Contains(store.recorded[0].Error, "ses rejected it") {
		t.Errorf("ledger = %+v, want the failure burned and its reason kept", store.recorded)
	}
}

// The cap is what bounds how much of an untested letter escapes before a human
// looks at the result.
func TestRun_HonoursTheCap(t *testing.T) {
	store := &fakeStore{}
	if _, err := newRunner(store, &fakeSender{}, 25).Run(context.Background(), campaign(t, "hiring-season-september")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if store.lastQuery.MaxRows != 25 {
		t.Errorf("cap = %d, want 25", store.lastQuery.MaxRows)
	}
	if store.lastQuery.Campaign != "hiring-season-september" {
		t.Errorf("campaign = %q, want the one being sent", store.lastQuery.Campaign)
	}
}

func TestPending_SendsNothing(t *testing.T) {
	store := &fakeStore{ids: []int64{1, 2}, remaining: 641}
	sender := &fakeSender{}

	got, err := newRunner(store, sender, 0).Pending(context.Background(), campaign(t, "hiring-season-september"))
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if got != 641 {
		t.Errorf("pending = %d, want 641", got)
	}
	if len(sender.sent) != 0 {
		t.Error("a dry run must not send anything")
	}
}

// A campaign that links back into freehire must take the origin from the Mailer, not
// spell it out: the previews render against a relative base, and a hard-coded
// https://freehire.me would show the production host there while still looking right.
// Both bodies are checked because the plain-text one is the copy most likely to be
// written by hand and the least likely to be looked at.
func TestSend_LinksBackThroughTheConfiguredOrigin(t *testing.T) {
	sender := &fakeSender{}
	m := broadcast.NewMailer(sender, "notifications@freehire.me", "ilya@example.test", "https://preview.test")
	c := campaign(t, "hiring-season-september")
	if err := m.Send(context.Background(), c, "someone@example.com"); err != nil {
		t.Fatalf("Send %s: %v", c.Name, err)
	}

	sent := sender.sent[0]
	const want = "https://preview.test/my/notifications"
	if !strings.Contains(sent.html, want) {
		t.Errorf("the HTML body does not link through the configured origin (%s)", want)
	}
	if !strings.Contains(sent.text, want) {
		t.Errorf("the text body does not link through the configured origin (%s)", want)
	}
	if strings.Contains(sent.text, "freehire.me/my/notifications") {
		t.Error("the text body spells the production origin out instead of taking it from the Mailer")
	}
	if !strings.Contains(sent.from, "Ilya") {
		t.Errorf("from = %q, want a person's name", sent.from)
	}
	if strings.TrimSpace(sent.text) == "" {
		t.Error("the campaign has no plain-text body")
	}
}
