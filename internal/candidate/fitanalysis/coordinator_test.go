package fitanalysis

import (
	"sync"
	"testing"
	"time"
)

func TestCoordinatorSecondCallerFollows(t *testing.T) {
	var co coordinator

	leader := co.Claim(1, 100)
	if !leader.IsLeader() {
		t.Fatal("first caller for a fresh key must lead")
	}

	followerReturned := make(chan struct{})
	go func() {
		follower := co.Claim(1, 100)
		if follower.IsLeader() {
			t.Error("second caller for the same key must follow, not lead")
		}
		follower.wait()
		close(followerReturned)
	}()

	select {
	case <-followerReturned:
		t.Fatal("follower returned before the leader released")
	case <-time.After(50 * time.Millisecond):
	}

	leader.Release(true)

	select {
	case <-followerReturned:
	case <-time.After(time.Second):
		t.Fatal("follower never unblocked after Release")
	}
}

// TestCoordinatorSucceededVisibleToFollower is the coordinator-level building block for
// Service.Follow's "don't trust the cache after a failed run" rule: whatever value the leader
// passes to Release must be exactly what a follower's wait reports, for both outcomes.
func TestCoordinatorSucceededVisibleToFollower(t *testing.T) {
	for _, succeeded := range []bool{true, false} {
		var co coordinator
		leader := co.Claim(1, 100)
		if !leader.IsLeader() {
			t.Fatal("first caller for a fresh key must lead")
		}
		follower := co.Claim(1, 100)
		if follower.IsLeader() {
			t.Fatal("second caller for the same key must follow, not lead")
		}

		leader.Release(succeeded)

		got := make(chan bool, 1)
		go func() { got <- follower.wait() }()
		select {
		case ok := <-got:
			if ok != succeeded {
				t.Errorf("follower wait = %v, want %v", ok, succeeded)
			}
		case <-time.After(time.Second):
			t.Fatal("follower's wait never returned")
		}
	}
}

// TestCoordinatorReleaseIsIdempotent guards the fix for a real hazard: the release used to be
// called ad hoc at several return points in the streaming handler rather than deferred once,
// so a future edit adding one more early return before an existing call would double-close and
// panic — on a goroutine fasthttp does not recover panics in, taking the process down.
func TestCoordinatorReleaseIsIdempotent(t *testing.T) {
	var co coordinator
	leader := co.Claim(1, 100)
	if !leader.IsLeader() {
		t.Fatal("first caller for a fresh key must lead")
	}

	leader.Release(true)
	if !leader.wait() {
		t.Fatal("wait reported failure after Release(true)")
	}

	// A second call — with a different argument, to make a silent no-op obvious — must not
	// panic (double close) and must not overwrite the first call's result.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("second Release panicked: %v", r)
			}
		}()
		leader.Release(false)
	}()
	if !leader.wait() {
		t.Error("a second Release overwrote the first call's result")
	}
}

func TestCoordinatorDifferentKeysDoNotBlock(t *testing.T) {
	var co coordinator
	if !co.Claim(1, 100).IsLeader() || !co.Claim(1, 200).IsLeader() {
		t.Fatal("distinct (candidate, job) pairs must each lead independently")
	}
}

func TestCoordinatorRunsAgainAfterRelease(t *testing.T) {
	var co coordinator
	leader := co.Claim(1, 100)
	if !leader.IsLeader() {
		t.Fatal("first caller for a fresh key must lead")
	}
	leader.Release(true)

	if !co.Claim(1, 100).IsLeader() {
		t.Fatal("a later call for the same key, after the prior run finished, must lead again")
	}
}

func TestCoordinatorZeroValueUsable(t *testing.T) {
	var co coordinator
	leader := co.Claim(1, 100)
	if !leader.IsLeader() {
		t.Fatal("zero-value coordinator must work with no constructor")
	}
	leader.Release(true)
}

// TestNilClaimIsAWorkingNoOp pins what a caller with no claim at all sees — the plain
// non-coalescing run passes Request.Claim nil, and every method must tolerate it.
func TestNilClaimIsAWorkingNoOp(t *testing.T) {
	var c *Claim
	if c.IsLeader() {
		t.Error("a nil claim must not report itself leader")
	}
	if c.wait() {
		t.Error("a nil claim must not report a successful leader")
	}
	c.Release(true) // must not panic
}

// TestCoordinatorRace exercises Claim under -race with many goroutines racing on one key:
// exactly one must lead, none may deadlock, and the leader's Release must eventually free
// every follower.
func TestCoordinatorRace(t *testing.T) {
	var co coordinator
	const n = 50

	var wg sync.WaitGroup
	var mu sync.Mutex
	var leaders int
	var leader *Claim
	followers := make([]*Claim, 0, n)

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			claim := co.Claim(1, 100)
			mu.Lock()
			defer mu.Unlock()
			if claim.IsLeader() {
				leaders++
				leader = claim
			} else {
				followers = append(followers, claim)
			}
		}()
	}
	wg.Wait()

	if leaders != 1 {
		t.Fatalf("expected exactly one leader, got %d", leaders)
	}
	if leader == nil {
		t.Fatal("the leader's claim was never captured")
	}
	leader.Release(true)

	for _, f := range followers {
		released := make(chan struct{})
		go func() { f.wait(); close(released) }()
		select {
		case <-released:
		case <-time.After(time.Second):
			t.Fatal("a follower never unblocked after the leader's Release")
		}
	}
}
