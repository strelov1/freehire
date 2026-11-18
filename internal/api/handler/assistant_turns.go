package handler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// errTurnQueueFull refuses a message that would be the second one waiting. One waiter is a
// courtesy; an unbounded queue is a way for a client to hold the process open.
var errTurnQueueFull = errors.New("this session already has a message waiting")

// turnQueueWait bounds how long a queued message waits for the turn ahead: a wait with no end
// holds the connection and its goroutine for as long as an unattended run goes. A minute
// outlasts an ordinary turn. A var so a test can shorten it; on timeout the queued message is
// destroyed, not deferred.
var turnQueueWait = time.Minute

// turnSlot is one session's running turn. It carries the turn's cancellation, so a later
// request can reach a turn streaming on another goroutine, and a channel that closes when the
// turn ends, which a waiting message waits on.
type turnSlot struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// finish closes the slot however many times it is called: release runs from a deferred call
// that a retry or a second path could reach twice, and closing a closed channel panics.
func (s *turnSlot) finish() {
	s.once.Do(func() { close(s.done) })
}

// turnRegistry is the set of turns running now, keyed by session, plus which sessions have a
// message waiting. A turn outlives the request that started it, so cancel needs this to find
// it; it also keeps a session to one turn at a time (two turns of one tailoring session would
// edit one CV from two conversations that cannot see each other).
// Per-process and not persisted: a CancelFunc cannot be stored, so a turn that outlives a
// blue/green flip is uncancellable and ends at its step cap. The zero value is ready to use,
// because handlers are assembled as struct literals in several places.
type turnRegistry struct {
	mu    sync.Mutex
	turns map[uuid.UUID]*turnSlot
	// waiting holds the cancellation of the message queued behind each session's running turn. It is
	// a cancellation, not a flag, because Stop must reach a turn that has not started: cancelling
	// only the running one would let the queued one in.
	waiting map[uuid.UUID]context.CancelFunc
}

// claim asks for the session's slot for a turn about to start. A free session hands back the
// slot, a busy one a waiter to enter through, and one that already has a waiter refuses.
func (r *turnRegistry) claim(session uuid.UUID, cancel context.CancelFunc) (*turnSlot, *turnWaiter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, running := r.turns[session]; !running {
		return r.take(session, cancel), nil, nil
	}
	if _, queued := r.waiting[session]; queued {
		return nil, nil, errTurnQueueFull
	}
	if r.waiting == nil {
		r.waiting = make(map[uuid.UUID]context.CancelFunc)
	}
	r.waiting[session] = cancel
	return nil, &turnWaiter{registry: r, session: session}, nil
}

// full reports whether the session has a turn running and another waiting, so claim would refuse.
// It is a read, not a reservation (claim stays the guard); it lets a message be refused BEFORE
// it is metered.
func (r *turnRegistry) full(session uuid.UUID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, running := r.turns[session]
	_, queued := r.waiting[session]
	return running && queued
}

// tryClaim is claim for a turn that will NOT wait: it hands back the slot of an idle session
// and reports a busy one as busy. claim's waiter is a reservation the caller must give back
// by entering or leaving it; a synchronous caller that never waits would leak it, and the
// session would answer errTurnQueueFull forever. Here nothing is reserved, so nothing can leak.
func (r *turnRegistry) tryClaim(session uuid.UUID, cancel context.CancelFunc) (*turnSlot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, running := r.turns[session]; running {
		return nil, false
	}
	return r.take(session, cancel), true
}

// take records a turn as running. The caller holds the lock.
func (r *turnRegistry) take(session uuid.UUID, cancel context.CancelFunc) *turnSlot {
	if r.turns == nil {
		r.turns = make(map[uuid.UUID]*turnSlot)
	}
	slot := &turnSlot{cancel: cancel, done: make(chan struct{})}
	r.turns[session] = slot
	return slot
}

// release gives the slot back when the turn ends and wakes whoever waited. It clears the entry
// only if the session still holds this very slot, so a replaced turn cannot delete its
// successor's entry.
func (r *turnRegistry) release(session uuid.UUID, slot *turnSlot) {
	if slot == nil {
		return
	}
	r.mu.Lock()
	if current, ok := r.turns[session]; ok && current == slot {
		delete(r.turns, session)
	}
	r.mu.Unlock()

	slot.finish()
}

// cancel stops the session's turns, the running one and the queued one behind it. It reports
// whether there was anything to stop; nothing running is not an error.
// The cancellations run under the lock: a CancelFunc reaches nothing in the registry, so it
// cannot deadlock, and releasing first would let the queued turn take the slot and the cancel
// land on a dead context while its successor runs.
func (r *turnRegistry) cancel(session uuid.UUID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	slot, running := r.turns[session]
	if running {
		slot.cancel()
	}
	if queued, waiting := r.waiting[session]; waiting {
		queued()
	}
	return running
}

// len is the number of turns running, for the tests that guard against the registry keeping
// entries for turns that have ended.
func (r *turnRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.turns)
}

// turnWaiter is a message's place in line for a session that is busy.
type turnWaiter struct {
	registry *turnRegistry
	session  uuid.UUID
}

// enter waits for the running turn to end and then takes the slot. It loops because another
// message may have taken the slot in between. The waiting place is always given back; otherwise
// the session would refuse every later message.
func (w *turnWaiter) enter(ctx context.Context, cancel context.CancelFunc) (*turnSlot, error) {
	defer w.leave()

	for {
		// Checked before taking the slot as well as while waiting: the turn ending and the wait being
		// cancelled can happen in either order.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		w.registry.mu.Lock()
		current, running := w.registry.turns[w.session]
		if !running {
			slot := w.registry.take(w.session, cancel)
			w.registry.mu.Unlock()
			return slot, nil
		}
		ended := current.done
		w.registry.mu.Unlock()

		select {
		case <-ended:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// leave gives the waiting place back.
func (w *turnWaiter) leave() {
	w.registry.mu.Lock()
	delete(w.registry.waiting, w.session)
	w.registry.mu.Unlock()
}
