package browsertools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Caller is a harness end that lives in this process: hire's own autofill agent
// drives the user's browser through it. It attaches to the hub exactly as a
// remote harness does — same role, same frames, same routing — so there is one
// relay path rather than an in-process shortcut beside it.
//
// Attaching displaces any remote harness on that channel, which is the intended
// "last connection wins" behaviour: one brain at a time drives a browser.
type Caller struct {
	hub  *Hub
	user int64

	mu      sync.Mutex
	pending map[string]chan []byte
	leave   func()
}

// NewCaller attaches an in-process harness to the user's channel. Close detaches
// it; until then every result for this user is routed here.
func (h *Hub) NewCaller(user int64) *Caller {
	c := &Caller{hub: h, user: user, pending: make(map[string]chan []byte)}
	c.leave = h.Join(user, RoleHarness, c)
	return c
}

func (c *Caller) Close() {
	c.leave()
	c.mu.Lock()
	defer c.mu.Unlock()
	// Unblock anything still waiting; a closed channel is a failed call, not a
	// goroutine parked forever.
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// Send receives one frame from the hub — a result the extension produced, or the
// relay's own error answer — and hands it to whichever call is waiting on its id.
func (c *Caller) Send(frame []byte) error {
	var answer struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame, &answer); err != nil {
		return err
	}
	c.mu.Lock()
	ch, ok := c.pending[answer.ID]
	delete(c.pending, answer.ID)
	c.mu.Unlock()
	if !ok {
		return nil // a late answer to an abandoned call
	}
	ch <- frame
	return nil
}

// Call issues one tool call and waits for its result. It returns the tool's raw
// JSON result for the caller to unmarshal, the executor's error, or ctx's error
// if the browser never answers.
func (c *Caller) Call(ctx context.Context, tool string, args any) (json.RawMessage, error) {
	id, ch := c.register()
	defer c.forget(id)

	frame, err := json.Marshal(struct {
		ID   string `json:"id"`
		Tool string `json:"tool"`
		Args any    `json:"args,omitempty"`
	}{ID: id, Tool: tool, Args: args})
	if err != nil {
		return nil, err
	}
	c.hub.Forward(c.user, RoleHarness, frame)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case raw, ok := <-ch:
		if !ok {
			return nil, errors.New("browser-tool channel closed before the call was answered")
		}
		var answer struct {
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(raw, &answer); err != nil {
			return nil, fmt.Errorf("browser-tool answer is not JSON: %w", err)
		}
		if answer.Error != "" {
			return nil, errors.New(answer.Error)
		}
		return answer.Result, nil
	}
}

func (c *Caller) register() (string, chan []byte) {
	id := c.hub.NextCallID()
	ch := make(chan []byte, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	return id, ch
}

func (c *Caller) forget(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}
