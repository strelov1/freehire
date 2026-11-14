package inboxdigest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

type memoryStore struct {
	candidates                  []Candidate
	snapshot                    Snapshot
	claimed                     bool
	claims, completed, released int
}

func (s *memoryStore) Candidates(context.Context, time.Time, int32) ([]Candidate, error) {
	return s.candidates, nil
}
func (s *memoryStore) Snapshot(context.Context, Candidate, time.Time) (Snapshot, error) {
	return s.snapshot, nil
}
func (s *memoryStore) Claim(context.Context, Candidate, Snapshot, time.Time) (bool, error) {
	s.claims++
	if s.claimed {
		return false, nil
	}
	s.claimed = true
	return true, nil
}
func (s *memoryStore) MarkSent(context.Context, Candidate, time.Time) error {
	s.completed++
	return nil
}
func (s *memoryStore) ReleasePaused(context.Context, Candidate, time.Time) error {
	s.claimed = false
	s.released++
	return nil
}

type capturingSender struct {
	messages []emailnotify.Message
	err      error
}

func (s *capturingSender) Send(_ context.Context, m emailnotify.Message) error {
	s.messages = append(s.messages, m)
	return s.err
}

func TestDailyDigestSkipsEmptyAndAlreadyClaimed(t *testing.T) {
	for _, empty := range []bool{true, false} {
		store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, claimed: !empty}
		if !empty {
			store.snapshot = Snapshot{IDs: []int64{11}, Total: 1, Items: []Item{{ID: 11, Subject: "Interview", Signal: "interview_invitation"}}}
		}
		sender := &capturingSender{}
		r := New(store, NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32)), sender)
		if _, err := r.Run(context.Background(), time.Now()); err != nil {
			t.Fatal(err)
		}
		if len(sender.messages) != 0 {
			t.Fatal("empty or claimed day sent mail")
		}
		if empty && store.claims != 0 {
			t.Fatal("empty day consumed its claim")
		}
	}
}

func TestDailyDigestEscapesSourcesAndRecordsOnlySuccess(t *testing.T) {
	store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, snapshot: Snapshot{IDs: []int64{11, 12}, Total: 12, Items: []Item{{ID: 11, Subject: "<script>offer</script>", Signal: "screening"}, {ID: 12, Subject: "Availability?", Signal: "info_request", Replied: true}}}}
	sender := &capturingSender{}
	r := New(store, NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32)), sender)
	stats, err := r.Run(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Sent != 1 || store.completed != 1 || len(sender.messages) != 1 {
		t.Fatalf("stats=%+v completed=%d sends=%d", stats, store.completed, len(sender.messages))
	}
	m := sender.messages[0]
	if strings.Contains(m.HTML, "<script>offer") || !strings.Contains(m.HTML, "&lt;script&gt;") || !strings.Contains(m.Text, "later reply") || !strings.Contains(m.Text, "10 more") || !strings.Contains(m.Text, "/my/inbox?message=11") || m.UnsubscribeURL == "" {
		t.Fatalf("incorrect rendered mail: %s", m.Text)
	}
}

func TestDailyDigestRetainsAmbiguousFailureAndReleasesPausedSend(t *testing.T) {
	for _, paused := range []bool{false, true} {
		store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, snapshot: Snapshot{IDs: []int64{11}, Total: 1, Items: []Item{{ID: 11, Subject: "Hello", Signal: "screening"}}}}
		sender := &capturingSender{err: errors.New("connection lost after dispatch")}
		if paused {
			sender.err = emailnotify.ErrSendingPaused
		}
		r := New(store, NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32)), sender)
		stats, err := r.Run(context.Background(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if store.completed != 0 {
			t.Fatal("failed send retired messages")
		}
		if paused {
			if store.claimed || stats.Skipped != 1 {
				t.Fatal("paused send consumed day")
			}
		} else {
			if !store.claimed || stats.Failed != 1 {
				t.Fatal("ambiguous send can repeat same day")
			}
		}
	}
}
