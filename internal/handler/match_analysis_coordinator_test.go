package handler

import (
	"sync"
	"testing"
	"time"
)

func TestMatchAnalysisCoordinatorSecondCallerFollows(t *testing.T) {
	var co matchAnalysisCoordinator

	done, _, isLeader := co.lead(1, 100)
	if !isLeader {
		t.Fatal("first caller for a fresh key must lead")
	}

	followerReturned := make(chan struct{})
	go func() {
		_, run, isLeader := co.lead(1, 100)
		if isLeader {
			t.Error("second caller for the same key must follow, not lead")
		}
		<-run.done
		close(followerReturned)
	}()

	select {
	case <-followerReturned:
		t.Fatal("follower returned before the leader called done")
	case <-time.After(50 * time.Millisecond):
	}

	done(true)

	select {
	case <-followerReturned:
	case <-time.After(time.Second):
		t.Fatal("follower never unblocked after done()")
	}
}

// TestMatchAnalysisCoordinatorSucceededVisibleToFollower is the coordinator-level building
// block for followMatchAnalysis's "don't trust the cache after a failed run" fix: whatever
// succeeded value the leader passes to done() must be exactly what a follower observes on
// run.succeeded once <-run.done unblocks it, for both outcomes.
func TestMatchAnalysisCoordinatorSucceededVisibleToFollower(t *testing.T) {
	for _, succeeded := range []bool{true, false} {
		var co matchAnalysisCoordinator
		done, _, isLeader := co.lead(1, 100)
		if !isLeader {
			t.Fatal("first caller for a fresh key must lead")
		}
		_, run, isLeader := co.lead(1, 100)
		if isLeader {
			t.Fatal("second caller for the same key must follow, not lead")
		}

		done(succeeded)

		select {
		case <-run.done:
		case <-time.After(time.Second):
			t.Fatal("follower's run.done never closed")
		}
		if run.succeeded != succeeded {
			t.Errorf("run.succeeded = %v, want %v", run.succeeded, succeeded)
		}
	}
}

// TestMatchAnalysisCoordinatorDoneIsIdempotent guards the fix for a real hazard: done() used
// to be called ad hoc at several return points in StreamMatchAnalysis rather than deferred
// once, so a future edit adding one more early return before an existing done() call would
// double-close run.done and panic. done() must tolerate being called more than once.
func TestMatchAnalysisCoordinatorDoneIsIdempotent(t *testing.T) {
	var co matchAnalysisCoordinator
	done, run, isLeader := co.lead(1, 100)
	if !isLeader {
		t.Fatal("first caller for a fresh key must lead")
	}

	done(true)
	if !run.succeeded {
		t.Fatal("run.succeeded = false after done(true)")
	}

	// A second call — with a different argument, to make a silent no-op obvious — must not
	// panic (double close) and must not overwrite the first call's result.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("second done() call panicked: %v", r)
			}
		}()
		done(false)
	}()
	if !run.succeeded {
		t.Error("a second done() call overwrote the first call's succeeded value")
	}
}

func TestMatchAnalysisCoordinatorDifferentKeysDoNotBlock(t *testing.T) {
	var co matchAnalysisCoordinator
	_, _, leaderA := co.lead(1, 100)
	_, _, leaderB := co.lead(1, 200)
	if !leaderA || !leaderB {
		t.Fatal("distinct (user, job) pairs must each lead independently")
	}
}

func TestMatchAnalysisCoordinatorRunsAgainAfterDone(t *testing.T) {
	var co matchAnalysisCoordinator
	done, _, isLeader := co.lead(1, 100)
	if !isLeader {
		t.Fatal("first caller for a fresh key must lead")
	}
	done(true)

	_, _, isLeader = co.lead(1, 100)
	if !isLeader {
		t.Fatal("a later call for the same key, after the prior run finished, must lead again")
	}
}

func TestMatchAnalysisCoordinatorZeroValueUsable(t *testing.T) {
	var co matchAnalysisCoordinator
	done, _, isLeader := co.lead(1, 100)
	if !isLeader {
		t.Fatal("zero-value coordinator must work with no constructor")
	}
	done(true)
}

// TestMatchAnalysisCoordinatorRace exercises lead() under -race with many goroutines racing
// on one key: exactly one must lead, none may deadlock, and the leader's done() must
// eventually release every follower.
func TestMatchAnalysisCoordinatorRace(t *testing.T) {
	var co matchAnalysisCoordinator
	const n = 50

	var wg sync.WaitGroup
	var mu sync.Mutex
	var leaders int
	var doneFn func(bool)
	followers := make([]*analysisRun, 0, n)

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			done, run, isLeader := co.lead(1, 100)
			mu.Lock()
			defer mu.Unlock()
			if isLeader {
				leaders++
				doneFn = done
			} else {
				followers = append(followers, run)
			}
		}()
	}
	wg.Wait()

	if leaders != 1 {
		t.Fatalf("expected exactly one leader, got %d", leaders)
	}
	if doneFn == nil {
		t.Fatal("leader's done() was never captured")
	}
	doneFn(true)

	for _, run := range followers {
		select {
		case <-run.done:
		case <-time.After(time.Second):
			t.Fatal("a follower never unblocked after the leader's done()")
		}
	}
}
