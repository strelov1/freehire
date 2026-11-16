package fitanalysis_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TestRunReleasesTheReservationWhenNothingIsProduced is the other half of making the debit the
// gate: taking the credit up front is only honest if a run that produces nothing gives it back.
// Otherwise closing the race would have cost candidates money on every failed analysis.
func TestRunReleasesTheReservationWhenNothingIsProduced(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	meter := newMeter(10)
	svc := newService(store, meter)

	reserved, err := svc.Reserve(ctx, userID, jobID)
	if err != nil || !reserved {
		t.Fatalf("Reserve = %v, %v; want a reservation", reserved, err)
	}
	if meter.remaining != 9 {
		t.Fatalf("remaining = %d, want the credit held", meter.remaining)
	}

	// A nil client makes Analyze answer (nil, nil): the LLM is unconfigured, so nothing is
	// computed and nothing cached.
	analysis, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: reserved,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil)
	if err != nil || analysis != nil {
		t.Fatalf("Run = %v, %v; want nil/nil for an unconfigured LLM", analysis, err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — a failed analysis costs nothing", meter.remaining)
	}
	if len(meter.releases) != 1 {
		t.Errorf("releases = %v, want the reservation given back", meter.releases)
	}
	if len(store.upserted) != 0 {
		t.Errorf("upserts = %d, want 0 — nothing was produced to cache", len(store.upserted))
	}

	// And the ref is chargeable again, so the candidate's retry is charged once, not never.
	again, err := svc.Reserve(ctx, userID, jobID)
	if err != nil || !again {
		t.Fatalf("retry Reserve = %v, %v; a released ref must be chargeable again", again, err)
	}
}

// TestRunKeepsTheReservationWhenItProducesAnAnalysis is the companion: a run that delivers is
// paid for, and the release path must not fire on success.
func TestRunKeepsTheReservationWhenItProducesAnAnalysis(t *testing.T) {
	// Run's success branch needs a real analyzer, which needs a model; the streamed and
	// on-demand success paths are covered end to end by the tagged handler tests. What is
	// checked here is the accounting either side of it: Reserve holds, and nothing releases
	// unless the run says it produced nothing.
	ctx := context.Background()
	meter := newMeter(10)
	svc := newService(&fakeStore{}, meter)

	if _, err := svc.Reserve(ctx, userID, jobID); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if meter.remaining != 9 || len(meter.releases) != 0 {
		t.Fatalf("remaining=%d releases=%v after reserving", meter.remaining, meter.releases)
	}
}

// TestReleaseNeverVoidsACreditAnEarlierRunEarned pins the other half of the rule: a LATER run
// that fails — the autopilot's refresh, a recompute — must not give back the credit an earlier
// run already earned. A credit buys HAVING the analysis, not the attempt that produced it.
func TestReleaseNeverVoidsACreditAnEarlierRunEarned(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	// An analysis exists for the pair, and it was paid for.
	svc := newService(&fakeStore{row: cachedRow(t, 80)}, meter)
	reserve(t, meter, jobID)

	// A recompute runs later and produces nothing. It reaches the release (it ran the chain),
	// and the guard is what stops it giving back the earlier run's credit.
	if _, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID},
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(meter.releases) != 0 {
		t.Errorf("releases = %v, want none — the analysis it was paid for still exists", meter.releases)
	}
	if meter.remaining != 9 {
		t.Errorf("remaining = %d, want 9", meter.remaining)
	}
}

// TestAFreeLeaderReleasesThePayingFollowersCredit is why the release belongs to whoever ran the
// chain rather than to whoever paid. The autopilot's cold-start pre-run leads for free; a
// streaming caller reserved and is following it. When the pre-run produces nothing, the
// follower is told the analysis is unavailable — and the charge standing against that pair is
// theirs, so the leader gives it back on their behalf.
func TestAFreeLeaderReleasesThePayingFollowersCredit(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	svc := newService(&fakeStore{}, meter)

	// The streaming caller reserved before it started following.
	reserve(t, meter, jobID)
	if meter.remaining != 9 {
		t.Fatalf("remaining = %d, want the credit held", meter.remaining)
	}

	leader := svc.Claim(userID, jobID)
	follower := svc.Claim(userID, jobID)

	// The pre-run: nothing cached, a nil client, so it computes nothing. Reserved is false —
	// this half never charges.
	_ = svc.Ensure(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Claim: leader,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	})

	if _, err := svc.Follow(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true, Claim: follower,
	}); !errors.Is(err, fitanalysis.ErrUnavailable) {
		t.Fatalf("follower err = %v, want ErrUnavailable", err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — nobody got an analysis, so nobody pays", meter.remaining)
	}
	if len(meter.releases) != 1 {
		t.Errorf("releases = %v, want exactly one, from the leader", meter.releases)
	}
}

// TestAFollowerNeverReleasesOnItsOwn is the other side: the debit is per (candidate, job) and
// shared, so a follower giving it back could void a charge the leader's result earned. The
// route that matters is a leader whose CACHE WRITE fails — it still returned its analysis to
// whoever paid for it, and leaves no row for the follower to read.
func TestAFollowerNeverReleasesOnItsOwn(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	// No row: the leader "succeeded" but its cache write did not land.
	svc := newService(&fakeStore{}, meter)
	reserve(t, meter, jobID)

	if _, err := svc.Follow(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
		Claim: followerOf(svc, true), // leader reports success
	}); !errors.Is(err, fitanalysis.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}

	if len(meter.releases) != 0 {
		t.Errorf("releases = %v, want none — the leader was served and earned the charge", meter.releases)
	}
	if meter.remaining != 9 {
		t.Errorf("remaining = %d, want 9 — the served analysis stays paid for", meter.remaining)
	}
}

// TestACancelledRunStillGivesTheCreditBack is the disconnect case. The on-demand endpoint runs
// on the request's own context, so a client that walks away mid-analysis cancels it — the chain
// fails with the cancellation, and a release on that same context could not even open its
// transaction. The candidate would stay charged for an analysis they never received, in exactly
// the situation the release exists for.
func TestACancelledRunStillGivesTheCreditBack(t *testing.T) {
	meter := newMeter(10)
	svc := newService(&fakeStore{}, meter)

	if _, err := svc.Reserve(context.Background(), userID, jobID); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if meter.remaining != 9 {
		t.Fatalf("remaining = %d, want the credit held", meter.remaining)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the reader is gone before the run even starts

	if _, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — a run nobody waited for still costs nothing", meter.remaining)
	}
	if len(meter.releases) != 1 {
		t.Errorf("releases = %v, want the reservation given back despite the cancellation", meter.releases)
	}
}

// TestAFailedEarnedCheckStillGivesTheCreditBack pins the direction the cleanup errs in. The
// check that stops a release — "does an analysis already exist for this pair?" — is a database
// read, and it can fail or time out. An unanswered question is not a yes: leaving a candidate
// charged for a run that produced nothing is the worse of the two errors.
func TestAFailedEarnedCheckStillGivesTheCreditBack(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	store := &fakeStore{}
	svc := newService(store, meter)

	if _, err := svc.Reserve(ctx, userID, jobID); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	// From here the cache is unreadable, so the release cannot learn whether anything exists.
	store.readErr = errors.New("db down")

	if _, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — an unanswerable check must not keep the charge", meter.remaining)
	}
}
