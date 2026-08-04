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

// turnQueueWait bounds how long a queued message waits for the turn ahead of it. A wait with no
// end is not patience but a leaked request: the connection, its goroutine and its place in line
// would be held for as long as the turn ahead runs, and an unattended run can go for minutes.
// A minute is longer than an ordinary turn and short enough that a client learns where it
// stands rather than staring at a silent stream.
//
// A var so a test can shorten it: the path it guards — a wait that runs out — is one a test
// would otherwise have to spend a real minute to reach, and untested is how it stayed unnoticed
// that the queued message is destroyed rather than deferred.
var turnQueueWait = time.Minute

// turnSlot is one session's running turn, held only for as long as it runs. It carries the
// turn's cancellation so a later request can reach a turn that is already streaming on another
// goroutine — the only handle on a turn that no HTTP request owns any more — and a channel that
// closes when the turn ends, which is what a waiting message waits on.
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

// turnRegistry is the set of turns running right now, keyed by session, plus which sessions
// have a message waiting to run next.
//
// It exists because a turn outlives the request that started it: the stream writer runs after
// its handler returned, so by the time a cancel request arrives there is nothing left to
// address it to. It also keeps a session to one turn at a time, which matters more than it
// sounds — two turns of one tailoring session would edit one CV from two conversations that
// cannot see each other.
//
// The registry is per-process and deliberately not persisted: a CancelFunc cannot be written to
// a database, and the goroutine that must act on it lives here. A turn that outlives a
// blue/green flip is therefore uncancellable and ends at its step cap instead.
//
// Its zero value is ready to use: the assistant handlers are assembled as a struct literal in
// several places, and a registry that had to be constructed would be nil in whichever one
// forgot — a nil map read is fine, but the first turn to write one would panic.
type turnRegistry struct {
	mu    sync.Mutex
	turns map[uuid.UUID]*turnSlot
	// waiting holds the cancellation of the message queued behind each session's running turn.
	// It is the cancellation and not a flag because Stop has to reach a turn that has not
	// started yet: cancelling only the running one would END it and thereby LET THE QUEUED ONE
	// IN, which is the opposite of what was asked.
	waiting map[uuid.UUID]context.CancelFunc
}

// claim asks for the session's slot on behalf of a turn about to start.
//
// A free session hands back the slot. A busy one hands back a waiter to enter through, so the
// message runs after the turn in flight rather than beside it. A session that already has
// someone waiting refuses.
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

// take records a turn as running. The caller holds the lock.
func (r *turnRegistry) take(session uuid.UUID, cancel context.CancelFunc) *turnSlot {
	if r.turns == nil {
		r.turns = make(map[uuid.UUID]*turnSlot)
	}
	slot := &turnSlot{cancel: cancel, done: make(chan struct{})}
	r.turns[session] = slot
	return slot
}

// release gives the slot back when the turn ends, however it ended, and wakes whoever waited.
//
// It clears the entry only if the session still holds this very slot: a turn that was already
// replaced must not delete its successor's entry and leave the session looking idle while a
// turn runs.
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

// cancel stops the session's turns — the one running and the one queued behind it, because a
// caller asking for the work to stop does not mean "and then start the next one".
//
// It reports whether there was anything to stop. A session with nothing running is not an
// error: a client cancels what it can no longer see the end of, and asking it to first prove
// the turn is alive would just make it guess.
//
// The cancellations run under the lock. A CancelFunc reaches nothing in the registry, so it
// cannot deadlock, and releasing first would leave a window in which the running turn ends, the
// queued one takes the slot, and the cancel lands on a context that is already dead while its
// successor runs on.
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

// enter waits for the running turn to end and then takes the slot for its own.
//
// It loops rather than assuming the slot is free once it wakes: another message may have taken
// it in between, and waking to find it gone is a reason to wait again, not to run beside it.
// Whatever happens, the waiting place is given back — a wait that ended without releasing it
// would leave the session refusing every later message.
func (w *turnWaiter) enter(ctx context.Context, cancel context.CancelFunc) (*turnSlot, error) {
	defer w.leave()

	for {
		// Checked before taking the slot, not only while waiting for it: the turn ahead ending
		// and this wait being cancelled can happen in either order, and waking to an open slot
		// is no reason to start work someone has already asked us not to do.
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
