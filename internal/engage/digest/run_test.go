package digest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

// snap is the smallest Snapshot: what the store returns decides whether it is empty.
type snap struct{ empty bool }

func (s snap) Empty() bool { return s.empty }

// step drives one candidate's pass through the loop.
type step struct {
	snapshot    snap
	snapshotErr error
	claim       bool
	claimErr    error
	markErr     error
}

// store serves one candidate per page and records the calls the runner makes.
type store struct {
	steps        []step
	selection    error
	markCtxErr   error // what MarkSent saw of its context, before the runner cancels it
	releaseErr   error
	releaseCtxEr error // same for ReleasePaused
	calls        []string
	pages        int
}

// Candidates reports another page while steps remain.
func (s *store) Candidates(_ context.Context, _ time.Time, _ int64) ([]Candidate, bool, error) {
	s.pages++
	s.calls = append(s.calls, "candidates")
	if s.selection != nil {
		return nil, false, s.selection
	}
	if s.pages > len(s.steps) {
		return nil, false, nil
	}
	return []Candidate{{UserID: int64(s.pages)}}, s.pages < len(s.steps), nil
}

func (s *store) Snapshot(context.Context, Candidate, time.Time) (snap, error) {
	s.calls = append(s.calls, "snapshot")
	return s.steps[s.pages-1].snapshot, s.steps[s.pages-1].snapshotErr
}

func (s *store) Claim(context.Context, Candidate, snap, time.Time) (bool, error) {
	s.calls = append(s.calls, "claim")
	st := s.steps[s.pages-1]
	return st.claim, st.claimErr
}

func (s *store) MarkSent(ctx context.Context, _ Candidate, _ time.Time) error {
	s.calls = append(s.calls, "mark")
	s.markCtxErr = ctx.Err()
	return s.steps[s.pages-1].markErr
}

func (s *store) ReleasePaused(ctx context.Context, _ Candidate, _ time.Time) error {
	s.calls = append(s.calls, "release")
	s.releaseCtxEr = ctx.Err()
	return s.releaseErr
}

// sender fails the n-th send with errs[n-1] when one is set.
type sender struct {
	sent int
	errs []error
}

func (s *sender) Send(context.Context, emailnotify.Message) error {
	s.sent++
	if s.sent <= len(s.errs) {
		return s.errs[s.sent-1]
	}
	return nil
}

func render(Candidate, snap) (emailnotify.Message, error) { return emailnotify.Message{}, nil }

func run(t *testing.T, st *store, s *sender, ctx context.Context) Stats {
	t.Helper()
	stats, err := Run(ctx, time.Now(), "test", st, s, render)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return stats
}

func TestRunDeliversAClaimedSnapshot(t *testing.T) {
	st := &store{steps: []step{{claim: true}}}
	s := &sender{}
	if stats := run(t, st, s, context.Background()); stats != (Stats{Sent: 1}) || s.sent != 1 {
		t.Fatalf("stats=%+v sent=%d", stats, s.sent)
	}
	if got := strings.Join(st.calls, ","); got != "candidates,snapshot,claim,mark" {
		t.Fatalf("calls=%s", got)
	}
}

// An empty snapshot and a lost claim race must not render or send.
func TestRunSkipsEmptyAndUnclaimedWithoutSending(t *testing.T) {
	for _, tc := range []struct {
		name string
		step step
		want string
	}{
		{"empty snapshot", step{snapshot: snap{empty: true}}, "candidates,snapshot"},
		{"lost claim race", step{}, "candidates,snapshot,claim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &store{steps: []step{tc.step}}
			s := &sender{}
			if stats := run(t, st, s, context.Background()); stats != (Stats{}) || s.sent != 0 {
				t.Fatalf("stats=%+v sent=%d", stats, s.sent)
			}
			if got := strings.Join(st.calls, ","); got != tc.want {
				t.Fatalf("calls=%s", got)
			}
		})
	}
}

// A failed step counts once and the pass moves on to the next account.
func TestRunCountsEachFailureAndKeepsGoing(t *testing.T) {
	st := &store{steps: []step{
		{snapshotErr: errors.New("snapshot down")},
		{claimErr: errors.New("claim down")},
		{claim: true},
		{claim: true},
	}}
	s := &sender{errs: []error{errors.New("connection lost after dispatch")}}
	if stats := run(t, st, s, context.Background()); stats != (Stats{Sent: 1, Failed: 3}) {
		t.Fatalf("stats=%+v", stats)
	}
}

// MarkSent runs on a context the run's own cancellation cannot kill, so a shutdown
// right after the provider accepted the mail still records the send.
func TestRunRecordsTheSendDespiteCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the run starts already cancelled, as a shutdown mid-send would leave it
	st := &store{steps: []step{{claim: true}}}
	s := &sender{}
	stats, err := Run(ctx, time.Now(), "test", st, s, render)
	if err != nil || stats != (Stats{Sent: 1}) || s.sent != 1 {
		t.Fatalf("stats=%+v sent=%d err=%v", stats, s.sent, err)
	}
	// The write must see a live context even though the run's own is cancelled.
	if st.markCtxErr != nil {
		t.Fatalf("mark sent saw a cancelled context: %v", st.markCtxErr)
	}
}

// A pause is account-wide: release the one claim and end the pass without paging on.
// The release runs on a detached context, so a shutdown during the pause cannot
// strand the claim and lose the account its digest for the day.
func TestRunStopsAtAPauseAndReleasesTheClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st := &store{steps: []step{{claim: true}, {claim: true}}}
	s := &sender{errs: []error{emailnotify.ErrSendingPaused}}
	stats, err := Run(ctx, time.Now(), "test", st, s, render)
	if err != nil || stats != (Stats{Skipped: 1}) {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	if st.pages != 1 || strings.Join(st.calls, ",") != "candidates,snapshot,claim,release" {
		t.Fatalf("pages=%d calls=%v", st.pages, st.calls)
	}
	if st.releaseCtxEr != nil {
		t.Fatalf("release saw a cancelled context: %v", st.releaseCtxEr)
	}
}

// A failed release is a failure, not a skip.
func TestRunCountsAFailedRelease(t *testing.T) {
	st := &store{steps: []step{{claim: true}}}
	s := &sender{errs: []error{emailnotify.ErrSendingPaused}}
	st.releaseErr = errors.New("ledger gone")
	if stats := run(t, st, s, context.Background()); stats != (Stats{Failed: 1}) {
		t.Fatalf("stats=%+v", stats)
	}
}

// The store decides whether another page follows; the runner follows it.
func TestRunPagesWhileTheStoreSaysMore(t *testing.T) {
	st := &store{steps: []step{{claim: true}, {claim: true}, {claim: true}}}
	if stats := run(t, st, &sender{}, context.Background()); stats != (Stats{Sent: 3}) || st.pages != 3 {
		t.Fatalf("stats=%+v pages=%d", stats, st.pages)
	}
}

// One page and the store says no more: the runner must not ask again.
func TestRunStopsWhenTheStoreHasNoMorePages(t *testing.T) {
	st := &store{steps: []step{{claim: true}}}
	if stats := run(t, st, &sender{}, context.Background()); stats != (Stats{Sent: 1}) || st.pages != 1 {
		t.Fatalf("stats=%+v pages=%d", stats, st.pages)
	}
}

func TestRunReturnsTheSelectionError(t *testing.T) {
	st := &store{selection: errors.New("db unreachable")}
	if _, err := Run(context.Background(), time.Now(), "test", st, &sender{}, render); !errors.Is(err, st.selection) {
		t.Fatalf("err=%v", err)
	}
}
