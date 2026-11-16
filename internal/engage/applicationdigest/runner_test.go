package applicationdigest

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

func (s *memoryStore) Candidates(context.Context, time.Time, int64, int32) ([]Candidate, error) {
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
func (s *memoryStore) MarkSent(ctx context.Context, _ Candidate, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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

func TestWeeklyDigestSkipsEmptyAndAlreadyClaimed(t *testing.T) {
	for _, empty := range []bool{true, false} {
		store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, claimed: !empty}
		if !empty {
			store.snapshot = Snapshot{AppliedCount: 1}
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

func TestWeeklyDigestEscapesSourcesAndRecordsOnlySuccess(t *testing.T) {
	store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, snapshot: Snapshot{AppliedCount: 12, Followups: []Followup{{ID: 11, RoleTitle: "<script>offer</script>", DaysSilent: 30, FollowedUpAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}}}}
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
	if strings.Contains(m.HTML, "<script>offer") || !strings.Contains(m.HTML, "&lt;script&gt;") || !strings.Contains(m.Text, "No reply recorded") || !strings.Contains(m.Text, "Followed up") || !strings.Contains(m.Text, "/my/tracking/a11") || m.UnsubscribeURL == "" {
		t.Fatalf("incorrect rendered mail: %s", m.Text)
	}
}

func TestWeeklyDigestRetainsAmbiguousFailureAndReleasesPausedSend(t *testing.T) {
	for _, paused := range []bool{false, true} {
		store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, snapshot: Snapshot{AppliedCount: 1}}
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

type cancellingSender struct{ cancel context.CancelFunc }

func (s cancellingSender) Send(context.Context, emailnotify.Message) error {
	s.cancel() // shutdown lands right after the provider accepted the mail
	return nil
}

func TestWeeklyDigestRecordsSendDespiteShutdown(t *testing.T) {
	store := &memoryStore{candidates: []Candidate{{UserID: 7, Email: "member@example.test"}}, snapshot: Snapshot{AppliedCount: 1}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := New(store, NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32)), cancellingSender{cancel})
	stats, err := r.Run(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if store.completed != 1 || stats.Sent != 1 {
		t.Fatalf("confirmed send left unrecorded: %+v", stats)
	}
}

func TestStageOnlySnapshotIsEmpty(t *testing.T) {
	if !(Snapshot{StageCounts: map[string]int64{"applied": 3}}).Empty() {
		t.Fatal("unchanged stages trigger mail")
	}
}

type pagedStore struct {
	memoryStore
	after []int64
}

func (s *pagedStore) Candidates(_ context.Context, _ time.Time, after int64, _ int32) ([]Candidate, error) {
	s.after = append(s.after, after)
	if after == 0 {
		out := make([]Candidate, BatchSize)
		for i := range out {
			out[i] = Candidate{UserID: int64(i + 1)}
		}
		return out, nil
	}
	return []Candidate{{UserID: 201, Email: "member@example.test"}}, nil
}
func (s *pagedStore) Snapshot(_ context.Context, c Candidate, _ time.Time) (Snapshot, error) {
	if c.UserID == 201 {
		return Snapshot{AppliedCount: 1}, nil
	}
	return Snapshot{}, nil
}
func TestEmptyFirstPageDoesNotStarveLaterRecipients(t *testing.T) {
	store := &pagedStore{}
	sender := &capturingSender{}
	stats, err := New(store, NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32)), sender).Run(context.Background(), time.Now())
	if err != nil || stats.Sent != 1 || len(store.after) != 2 || store.after[1] != 200 {
		t.Fatalf("stats=%+v after=%v err=%v", stats, store.after, err)
	}
}
func TestMailerUsesBoardReferencesAndStageLabels(t *testing.T) {
	mailer := NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32))
	m, err := mailer.Render(Candidate{UserID: 7}, Snapshot{AppliedCount: 1, StageCounts: map[string]int64{"applied": 2}, Followups: []Followup{{ID: 11, JobSlug: "engineer-acme", Stage: "applied"}, {ID: 12, Stage: "interview"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Applied: 2", "/my/tracking/engineer-acme", "/my/tracking/a12"} {
		if !strings.Contains(m.Text, want) {
			t.Fatalf("missing %q in %s", want, m.Text)
		}
	}
	m, err = mailer.Render(Candidate{UserID: 7}, Snapshot{AppliedCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Text, "Current stages") || strings.Contains(m.HTML, "Current stages") {
		t.Fatal("empty stages heading")
	}
}

func TestStageChangeIsActivityEvenWithoutNewApplications(t *testing.T) {
	if (Snapshot{ActivityCount: 1, StageCounts: map[string]int64{"rejected": 1}}).Empty() {
		t.Fatal("a recorded stage change must allow a weekly status summary")
	}
}
