package browsertools_test

import (
	"context"
	"encoding/json"
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

// senderFunc adapts a function to browsertools.Socket.
type senderFunc func([]byte) error

func (f senderFunc) Send(frame []byte) error { return f(frame) }
