package browsertools_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/browsertools"
)

// echoExtension answers every call the hub forwards it with a fixed result,
// standing in for a browser on the other end.
type echoExtension struct {
	hub    *browsertools.Hub
	user   int64
	result string
}

func (e *echoExtension) Send(frame []byte) error {
	var call struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame, &call); err != nil {
		return err
	}
	e.hub.Forward(e.user, browsertools.RoleExtension,
		[]byte(`{"id":"`+call.ID+`","result":`+e.result+`}`))
	return nil
}

func TestCallerGetsTheExtensionsResult(t *testing.T) {
	hub := browsertools.New()
	hub.Join(7, browsertools.RoleExtension, &echoExtension{hub: hub, user: 7, result: `{"fields":[]}`})

	caller := hub.NewCaller(7)
	defer caller.Close()

	got, err := caller.Call(context.Background(), "read_form", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(got) != `{"fields":[]}` {
		t.Fatalf("result = %s, want the extension's payload", got)
	}
}

func TestCallerSurfacesAnExecutorErrorAsAnError(t *testing.T) {
	hub := browsertools.New()
	hub.Join(7, browsertools.RoleExtension, senderFunc(func(frame []byte) error {
		var call struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(frame, &call)
		hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"`+call.ID+`","error":"no active tab"}`))
		return nil
	}))

	caller := hub.NewCaller(7)
	defer caller.Close()

	if _, err := caller.Call(context.Background(), "read_form", nil); err == nil || err.Error() != "no active tab" {
		t.Fatalf("err = %v, want the executor's message", err)
	}
}

func TestCallerFailsRatherThanBlockingWhenNoExtensionIsConnected(t *testing.T) {
	hub := browsertools.New()
	caller := hub.NewCaller(7)
	defer caller.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := caller.Call(ctx, "read_form", nil)
	if err == nil {
		t.Fatal("Call succeeded with no extension attached")
	}
	if ctx.Err() != nil {
		t.Fatal("Call blocked until the context expired; the relay should have answered")
	}
}

func TestCallerMatchesEachAnswerToItsOwnCall(t *testing.T) {
	hub := browsertools.New()
	// An extension that answers out of order: it holds the first call until the
	// second has been answered.
	var held []byte
	hub.Join(7, browsertools.RoleExtension, senderFunc(func(frame []byte) error {
		var call struct {
			ID   string `json:"id"`
			Tool string `json:"tool"`
		}
		_ = json.Unmarshal(frame, &call)
		if call.Tool == "read_form" {
			held = frame
			return nil
		}
		hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"`+call.ID+`","result":"second"}`))
		var first struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(held, &first)
		hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"`+first.ID+`","result":"first"}`))
		return nil
	}))

	caller := hub.NewCaller(7)
	defer caller.Close()

	firstDone := make(chan json.RawMessage, 1)
	go func() {
		res, _ := caller.Call(context.Background(), "read_form", nil)
		firstDone <- res
	}()
	// Let the first call reach the extension before the second releases it.
	time.Sleep(50 * time.Millisecond)

	second, err := caller.Call(context.Background(), "fill_simple", map[string]any{"fills": []any{}})
	if err != nil {
		t.Fatalf("second Call: %v", err)
	}
	if string(second) != `"second"` {
		t.Fatalf("second call got %s", second)
	}
	select {
	case res := <-firstDone:
		if string(res) != `"first"` {
			t.Fatalf("first call got %s, want its own answer", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first call never resolved")
	}
}

func TestStaleAnswerForAnEvictedCallerDoesNotResolveASuccessorsCall(t *testing.T) {
	hub := browsertools.New()

	var mu sync.Mutex
	var frames [][]byte
	sent := make(chan struct{}, 2)
	extension := senderFunc(func(frame []byte) error {
		mu.Lock()
		frames = append(frames, frame)
		mu.Unlock()
		sent <- struct{}{}
		return nil // never answers; the caller's call times out
	})
	hub.Join(7, browsertools.RoleExtension, extension)

	// Caller A's first call times out and is closed, exactly as a retry would.
	callerA := hub.NewCaller(7)
	ctxA, cancelA := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelA()
	if _, err := callerA.Call(ctxA, "read_form", nil); err == nil {
		t.Fatal("callerA.Call succeeded; want it to time out since the extension never answers")
	}
	callerA.Close()
	<-sent

	mu.Lock()
	idA := frameID(t, frames[0])
	mu.Unlock()

	// Caller B replaces A as the harness end and issues its own call.
	callerB := hub.NewCaller(7)
	defer callerB.Close()

	type callResult struct {
		res json.RawMessage
		err error
	}
	done := make(chan callResult, 1)
	go func() {
		res, err := callerB.Call(context.Background(), "fill_simple", nil)
		done <- callResult{res, err}
	}()
	// Wait for callerB's request to actually reach the extension (proving it
	// registered its pending call) before delivering any answer — a bounded
	// wait rather than a fixed sleep, so this cannot flake under scheduler delay.
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("callerB request did not reach the extension")
	}

	mu.Lock()
	idB := frameID(t, frames[len(frames)-1])
	mu.Unlock()
	if idA == idB {
		t.Fatalf("callerA and callerB both minted call id %q; ids must be unique per hub", idA)
	}

	// A late answer for A's evicted call arrives from the extension. Routed by
	// role, it lands on whichever Caller currently holds RoleHarness — B — and
	// must not be mistaken for an answer to B's own pending call.
	hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"`+idA+`","result":"stale"}`))
	hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"`+idB+`","result":"fresh"}`))

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("callerB.Call: %v", r.err)
		}
		if string(r.res) != `"fresh"` {
			t.Fatalf("callerB.Call result = %s, want its own answer, not the stale one delivered for A's call", r.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callerB.Call never resolved")
	}
}

func frameID(t *testing.T, frame []byte) string {
	t.Helper()
	var call struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame, &call); err != nil {
		t.Fatalf("frame is not JSON: %v", err)
	}
	return call.ID
}

// senderFunc adapts a function to browsertools.Socket.
type senderFunc func([]byte) error

func (f senderFunc) Send(frame []byte) error { return f(frame) }
