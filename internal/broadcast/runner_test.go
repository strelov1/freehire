package broadcast_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/broadcast"
	"github.com/strelov1/freehire/internal/db"
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

	stats, err := newRunner(store, sender, 0).Run(context.Background(), campaign(t, "ph-heads-up"))
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

	stats, err := newRunner(store, sender, 0).Run(context.Background(), campaign(t, "ph-heads-up"))
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
	if _, err := newRunner(store, &fakeSender{}, 25).Run(context.Background(), campaign(t, "ph-heads-up")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if store.lastQuery.MaxRows != 25 {
		t.Errorf("cap = %d, want 25", store.lastQuery.MaxRows)
	}
	if store.lastQuery.Campaign != "ph-heads-up" {
		t.Errorf("campaign = %q, want the one being sent", store.lastQuery.Campaign)
	}
}

func TestPending_SendsNothing(t *testing.T) {
	store := &fakeStore{ids: []int64{1, 2}, remaining: 641}
	sender := &fakeSender{}

	got, err := newRunner(store, sender, 0).Pending(context.Background(), campaign(t, "ph-heads-up"))
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

// Both Product Hunt letters point at the same page but ask for different things,
// because before launch day the vote does not exist yet. Sending the launch-day
// copy early would promise a button that is not on the page.
func TestCampaigns_AskForTheRightThingAtTheRightTime(t *testing.T) {
	heads := campaign(t, "ph-heads-up")
	live := campaign(t, "ph-live")

	if heads.Name == live.Name {
		t.Fatal("the two campaigns share a ledger key, so the second would never send")
	}

	sender := &fakeSender{}
	m := broadcast.NewMailer(sender, "notifications@freehire.me", "ilya@example.test", "https://freehire.me")
	for _, c := range []broadcast.Campaign{heads, live} {
		if err := m.Send(context.Background(), c, "someone@example.com"); err != nil {
			t.Fatalf("Send %s: %v", c.Name, err)
		}
	}

	before, after := sender.sent[0], sender.sent[1]
	if !strings.Contains(before.html, "Notify me") {
		t.Error("the pre-launch mail should ask for a reminder tap, not a vote")
	}
	if strings.Contains(strings.ToLower(before.html), "upvote") {
		t.Error("the pre-launch mail asks for an upvote that cannot be cast yet")
	}
	if !strings.Contains(strings.ToLower(after.html), "upvote") {
		t.Error("the launch-day mail should ask for the vote")
	}
	for _, m := range []sentMail{before, after} {
		if !strings.Contains(m.from, "Ilya") {
			t.Errorf("from = %q, want a person's name", m.from)
		}
		if strings.TrimSpace(m.text) == "" {
			t.Errorf("%q has no plain-text body", m.subject)
		}
		if !strings.Contains(m.html, "producthunt.com") {
			t.Errorf("%q does not link to the launch page", m.subject)
		}
	}
}
