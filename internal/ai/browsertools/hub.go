// Package browsertools relays browser-tool frames between the two ends of one
// user's channel: a harness (the "brain") and that user's browser extension, which
// executes the tools.
//
// The relay forwards the raw `{id,tool,args}` / `{id,result}` JSON verbatim, so a new
// primitive needs no change here. A channel is keyed by user id, and a frame never
// crosses from one user's harness to another user's browser.
package browsertools

import (
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
)

// Role names one end of a user's channel.
type Role string

const (
	// RoleExtension is the browser that executes tool calls.
	RoleExtension Role = "extension"
	// RoleHarness is the agent that issues them.
	RoleHarness Role = "harness"
)

// ParseRole validates the role a connection claims, rejecting anything else so a
// typo cannot open a third, unrouted end.
func ParseRole(s string) (Role, bool) {
	switch Role(s) {
	case RoleExtension, RoleHarness:
		return Role(s), true
	default:
		return "", false
	}
}

// Socket is the one thing the hub needs of a connection: somewhere to put a
// frame. Satisfied by the websocket handler's writer, and by a fake in tests.
type Socket interface {
	Send(frame []byte) error
}

// Hub holds the live channels, one per user.
type Hub struct {
	mu       sync.Mutex
	channels map[int64]map[Role]Socket
	callSeq  atomic.Int64
}

func New() *Hub {
	return &Hub{channels: make(map[int64]map[Role]Socket)}
}

// NextCallID returns a call id unique across every Caller this Hub has hosted.
// Callers must not use a local counter: two Callers for one user (a retry after
// Close) would mint the same id, and a late answer could reach the wrong call.
func (h *Hub) NextCallID() string {
	return strconv.FormatInt(h.callSeq.Add(1), 10)
}

// Join attaches a socket as one end of a user's channel and returns the function that
// detaches it. A second connection in the same role replaces the first; the displaced
// connection's leave is then a no-op, so it cannot evict its successor.
func (h *Hub) Join(user int64, role Role, s Socket) (leave func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ends, ok := h.channels[user]
	if !ok {
		ends = make(map[Role]Socket, 2)
		h.channels[user] = ends
	}
	ends[role] = s

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.channels[user][role] != s {
			return
		}
		delete(h.channels[user], role)
		if len(h.channels[user]) == 0 {
			delete(h.channels, user)
		}
	}
}

// Forward hands a frame to the other end of the sender's own channel, never another
// user's. When that end is absent, a harness's call is answered with an error result
// carrying its id (silence would hang the harness); a result with no harness is dropped.
func (h *Hub) Forward(user int64, from Role, frame []byte) {
	h.mu.Lock()
	ends := h.channels[user]
	to, sender := ends[counterpart(from)], ends[from]
	h.mu.Unlock()

	if to != nil {
		_ = to.Send(frame)
		return
	}
	if from == RoleHarness && sender != nil {
		_ = sender.Send(notConnected(frame))
	}
}

func counterpart(r Role) Role {
	if r == RoleHarness {
		return RoleExtension
	}
	return RoleHarness
}

// ErrNotConnected is the relay's answer to a call issued while the user's browser
// extension is not attached: a state the user fixes by opening the panel, not a fault.
//
// The answer crosses the relay as a JSON frame: the hub writes this text
// (notConnected) and Caller.Call reads it back into this value for errors.Is.
var ErrNotConnected = errors.New("the browser extension is not connected")

// ErrExecutorReported wraps whatever the extension said went wrong on its side (no
// active tab, a discarded content script, an unreadable page). The text cannot be
// classified, but callers must tell "the browser could not" (the user's to fix) from
// "we could not" (ours), or every stale content script becomes an internal fault.
var ErrExecutorReported = errors.New("the browser reported")

// notConnected builds the error result for a call that has no executor, echoing the
// call's id; an unparseable frame gets an empty id, still better than no answer.
func notConnected(frame []byte) []byte {
	var call struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(frame, &call)
	answer, _ := json.Marshal(struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}{ID: call.ID, Error: ErrNotConnected.Error()})
	return answer
}
