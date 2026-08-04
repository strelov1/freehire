package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTurnRegistryAdmitsOneTurnPerSession(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()

	first, waiter, err := reg.claim(session, func() {})
	if err != nil || first == nil || waiter != nil {
		t.Fatalf("the first turn of an idle session did not get the slot: %v, %v, %v", first, waiter, err)
	}

	// Another session is another slot: the bound is per conversation, not global.
	if other, waiter, err := reg.claim(uuid.New(), func() {}); err != nil || other == nil || waiter != nil {
		t.Fatalf("a different session was made to wait while this one ran: %v, %v, %v", other, waiter, err)
	}

	reg.release(session, first)
	if again, waiter, err := reg.claim(session, func() {}); err != nil || again == nil || waiter != nil {
		t.Fatalf("the session stayed busy after its turn ended: %v, %v, %v", again, waiter, err)
	}
}

// A registry that keeps entries for turns that have ended is a leak that grows for as long as
// the process lives, and it would also refuse every later turn of the session.
func TestTurnRegistryForgetsAFinishedTurn(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()

	slot, _, _ := reg.claim(session, func() {})
	reg.release(session, slot)

	if n := reg.len(); n != 0 {
		t.Fatalf("registry holds %d entries after the turn ended, want 0", n)
	}
}

func TestTurnRegistryCancelsTheTurnItHolds(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())

	slot, _, _ := reg.claim(session, cancel)
	if !reg.cancel(session) {
		t.Fatal("cancelling a running turn reported nothing to cancel")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("the turn's context was not cancelled")
	}
	reg.release(session, slot)
}

// Cancelling a session with nothing running is how a client stops a turn it cannot see the end
// of. It reports that there was nothing to do rather than failing.
func TestTurnRegistryCancelIsHarmlessWhenIdle(t *testing.T) {
	var reg turnRegistry

	if reg.cancel(uuid.New()) {
		t.Fatal("cancelling an idle session claimed to have cancelled a turn")
	}
}

// A second message does not race the turn in flight — it waits for it. Racing would put two
// turns of one tailoring session on one CV, each blind to the other's edits.
func TestTurnRegistrySecondClaimWaitsForTheFirst(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()

	running, waiter, err := reg.claim(session, func() {})
	if err != nil || running == nil || waiter != nil {
		t.Fatalf("first claim = (%v, %v, %v), want the slot", running, waiter, err)
	}

	_, waiter, err = reg.claim(session, func() {})
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if waiter == nil {
		t.Fatal("the second claim took a slot beside the first instead of waiting for it")
	}

	entered := make(chan *turnSlot, 1)
	go func() {
		slot, err := waiter.enter(context.Background(), func() {})
		if err != nil {
			t.Errorf("enter: %v", err)
		}
		entered <- slot
	}()

	select {
	case <-entered:
		t.Fatal("the waiting turn started while the first was still running")
	case <-time.After(50 * time.Millisecond):
	}

	reg.release(session, running)
	select {
	case slot := <-entered:
		if slot == nil {
			t.Fatal("the waiting turn was let in without a slot")
		}
		reg.release(session, slot)
	case <-time.After(2 * time.Second):
		t.Fatal("the waiting turn never started after the first ended")
	}
}

// One waiter, not a queue. A queue a client can grow without limit is a way to hold the process
// open, and the waiting message is a live request whose natural depth is one.
func TestTurnRegistryRefusesASecondWaiter(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()

	running, _, _ := reg.claim(session, func() {})
	if _, waiter, err := reg.claim(session, func() {}); err != nil || waiter == nil {
		t.Fatalf("the first waiter was refused: %v", err)
	}

	if _, _, err := reg.claim(session, func() {}); !errors.Is(err, errTurnQueueFull) {
		t.Fatalf("third claim err = %v, want errTurnQueueFull", err)
	}
	reg.release(session, running)
}

// A wait that outlives its patience gives its place back, or the session would look permanently
// occupied to every later message.
func TestTurnRegistryWaitGivesUpWithItsContext(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()

	running, _, _ := reg.claim(session, func() {})
	_, waiter, _ := reg.claim(session, func() {})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := waiter.enter(ctx, func() {}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("enter err = %v, want the deadline", err)
	}

	// The place is free again: another message may now wait.
	if _, waiter, err := reg.claim(session, func() {}); err != nil || waiter == nil {
		t.Fatalf("the waiting place was not given back: %v, %v", waiter, err)
	}
	reg.release(session, running)
}

// The registry is reached from the stream writer's goroutine and from cancel requests at the
// same time, so its map must never be touched unguarded.
func TestTurnRegistryIsSafeUnderConcurrentUse(t *testing.T) {
	var reg turnRegistry
	sessions := make([]uuid.UUID, 8)
	for i := range sessions {
		sessions[i] = uuid.New()
	}

	var wg sync.WaitGroup
	for _, session := range sessions {
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if slot, _, err := reg.claim(session, func() {}); err == nil && slot != nil {
					reg.cancel(session)
					reg.release(session, slot)
				}
			}()
		}
	}
	wg.Wait()

	if n := reg.len(); n != 0 {
		t.Fatalf("registry holds %d entries after every turn ended, want 0", n)
	}
}

// Stop must reach a turn that has not started yet. Otherwise pressing Stop with a message
// queued behind the running one CANCELS the running turn and thereby STARTS the queued one —
// the opposite of what was asked.
func TestTurnRegistryCancelReachesAQueuedTurn(t *testing.T) {
	var reg turnRegistry
	session := uuid.New()

	running, _, _ := reg.claim(session, func() {})
	waitCtx, waitCancel := context.WithCancel(context.Background())
	_, waiter, err := reg.claim(session, waitCancel)
	if err != nil || waiter == nil {
		t.Fatalf("second claim did not queue: %v, %v", waiter, err)
	}

	entered := make(chan error, 1)
	go func() {
		_, err := waiter.enter(waitCtx, waitCancel)
		entered <- err
	}()

	reg.cancel(session)
	reg.release(session, running)

	select {
	case err := <-entered:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("the queued turn started anyway (err = %v)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the queued turn neither started nor gave up")
	}
	if n := reg.len(); n != 0 {
		t.Fatalf("registry holds %d entries, want 0", n)
	}
}
