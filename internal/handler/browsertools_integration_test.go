package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	ws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/browsertools"
)

// arrivalBudget bounds the waits for a frame that must arrive. It is far longer than the relay
// needs (the package runs in ~1.6s locally); the budget only sets how long a genuinely broken
// relay takes to report, so the cost of generosity is paid only when the test is already failing.
//
// It was 5s, then 30s, on the theory that the waits were measuring a contended CI runner. They
// were not: the frame was never coming (see awaitRelay), so each raise only bought a longer wait
// before the same failure. With the precondition established explicitly, a wait that expires now
// means a relay fault, and 10s is more than enough to say so.
const arrivalBudget = 10 * time.Second

// noKeys stands in for the API-key lookup: these tests authenticate with JWTs,
// so every key is unknown. It exists so the middleware has something to call
// rather than a nil interface.
type noKeys struct{}

func (noKeys) AuthenticateAPIKey(context.Context, string) (auth.APIKeyIdentity, error) {
	return auth.APIKeyIdentity{}, errors.New("no such key")
}

// wireServer starts the /tools/ws route on a real listener — a WebSocket needs an
// actual connection, which app.Test cannot provide — and returns its base URL.
func wireServer(t *testing.T, iss *auth.Issuer) string {
	t.Helper()
	a := &API{issuer: iss, browserTools: browsertools.New()}
	app := fiber.New()
	app.Get("/api/v1/tools/ws", auth.RequireAuthWS(iss, testVersions, noKeys{}), a.BrowserToolsWS())

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	return "ws://" + ln.Addr().String() + "/api/v1/tools/ws"
}

// dial joins one end of a user's channel the way that end really would: the
// extension puts its JWT in the subprotocol, a harness in an Authorization header.
func dial(t *testing.T, base, role, token string) *ws.Conn {
	t.Helper()
	dialer := ws.Dialer{
		Subprotocols:     []string{auth.WSSubprotocolMarker, token},
		HandshakeTimeout: arrivalBudget,
	}
	conn, resp, err := dialer.Dial(base+"?role="+role, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", role, err, status)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// frames starts a reader for one end and delivers whatever arrives on it.
//
// A reader goroutine rather than a read deadline per assertion: in this library a read that
// expires POISONS the connection — every later read returns the same timeout instantly, without
// waiting — so a test that must sometimes wait in vain cannot use a deadline on a connection it
// will read again. The buffer is larger than any test consumes, so the reader never blocks on a
// frame nobody asked for.
func frames(conn *ws.Conn) <-chan string {
	out := make(chan string, 8)
	go func() {
		defer close(out)
		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				return
			}
			out <- string(frame)
		}
	}()
	return out
}

// await takes the next frame from an end, failing if none arrives within the budget.
func await(t *testing.T, in <-chan string) string {
	t.Helper()
	select {
	case frame, ok := <-in:
		if !ok {
			t.Fatal("the connection closed before the frame arrived")
		}
		return frame
	case <-time.After(arrivalBudget):
		t.Fatal("no frame arrived within the budget")
		return ""
	}
}

// awaitRelay blocks until the relay routes between the two ends — the precondition every
// relaying assertion needs, and the one this file used to assume.
//
// The WebSocket handshake completes BEFORE the server registers the end: Join runs in the
// connection goroutine, which the runtime need not have scheduled by the time dial returns. A
// call sent in that window finds the channel half-open, and the hub then does exactly the right
// thing — it answers the HARNESS "the browser extension is not connected". So the extension never
// sees the call, and a wait on it expires at whatever budget it has. Reproduced deterministically
// by sleeping 50ms before Join: the failure is identical to the one CI showed.
//
// Probed through the protocol rather than by reading the hub's state, because the protocol is
// what these tests exercise: the probe either reaches the extension (both ends live) or comes
// back to the harness (not yet). Exactly one of the two happens per attempt, from one Forward
// call, so no stray answer can outlive the loop and be mistaken for a later result.
func awaitRelay(t *testing.T, harness *ws.Conn, fromHarness, toExtension <-chan string) {
	t.Helper()
	const probe = `{"id":"__probe","tool":"__probe"}`

	giveUp := time.After(arrivalBudget)
	for {
		if err := harness.WriteMessage(ws.TextMessage, []byte(probe)); err != nil {
			t.Fatalf("probe write: %v", err)
		}
		select {
		case got := <-toExtension:
			if got != probe {
				t.Fatalf("probe arrived as %q, want it verbatim", got)
			}
			return
		case <-fromHarness:
			// The hub answered the sender: the extension is not registered yet. The next
			// attempt is a round trip away, so pause briefly rather than spin.
			time.Sleep(2 * time.Millisecond)
		case <-giveUp:
			t.Fatal("the relay never routed between the two ends")
		}
	}
}

func TestBrowserToolsWS_CarriesACallAndItsResultBetweenTheUsersOwnEnds(t *testing.T) {
	iss := auth.NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	base := wireServer(t, iss)

	extension := dial(t, base, "extension", token)
	harness := dial(t, base, "harness", token)
	toExtension, toHarness := frames(extension), frames(harness)
	awaitRelay(t, harness, toHarness, toExtension)

	call := `{"id":"c1","tool":"read_form","args":{}}`
	if err := harness.WriteMessage(ws.TextMessage, []byte(call)); err != nil {
		t.Fatalf("harness write: %v", err)
	}
	if got := await(t, toExtension); got != call {
		t.Fatalf("extension got %q, want the call verbatim", got)
	}

	result := `{"id":"c1","result":{"fields":[]}}`
	if err := extension.WriteMessage(ws.TextMessage, []byte(result)); err != nil {
		t.Fatalf("extension write: %v", err)
	}
	if got := await(t, toHarness); got != result {
		t.Fatalf("harness got %q, want the result verbatim", got)
	}
}

func TestBrowserToolsWS_AnswersACallWithNoExtensionInsteadOfHanging(t *testing.T) {
	iss := auth.NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, testTokenVersion)
	harness := dial(t, wireServer(t, iss), "harness", token)

	if err := harness.WriteMessage(ws.TextMessage, []byte(`{"id":"c9","tool":"read_form"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	var answer struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(await(t, frames(harness))), &answer); err != nil {
		t.Fatalf("answer is not JSON: %v", err)
	}
	if answer.ID != "c9" || answer.Error == "" {
		t.Fatalf("answer = %+v, want the call id and an error", answer)
	}
}

func TestBrowserToolsWS_DoesNotBridgeBetweenUsers(t *testing.T) {
	iss := auth.NewIssuer("secret", time.Hour)
	tokenA, _ := iss.Issue(1, testTokenVersion)
	tokenB, _ := iss.Issue(2, testTokenVersion)
	base := wireServer(t, iss)

	extensionB := dial(t, base, "extension", tokenB)
	harnessA := dial(t, base, "harness", tokenA)
	toExtensionB, toHarnessA := frames(extensionB), frames(harnessA)

	call := `{"id":"c1","tool":"read_form"}`
	if err := harnessA.WriteMessage(ws.TextMessage, []byte(call)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// User A's harness is answered by the relay itself — and here that answer is guaranteed
	// whatever the scheduling, since user A HAS no extension to register.
	if got := await(t, toHarnessA); got == call {
		t.Fatal("the call came back to its sender")
	}
	// ...and user B's extension is never handed anything.
	select {
	case frame := <-toExtensionB:
		t.Fatalf("user B's extension received %q from user A's harness", frame)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestBrowserToolsWS_RefusesAnUnauthenticatedHandshake(t *testing.T) {
	base := wireServer(t, auth.NewIssuer("secret", time.Hour))

	dialer := ws.Dialer{HandshakeTimeout: arrivalBudget}
	conn, resp, err := dialer.Dial(base+"?role=extension", nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil {
		_ = conn.Close()
		t.Fatal("an unauthenticated connection was accepted")
	}
	if resp == nil || resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}

func TestBrowserToolsWS_EchoesOnlyTheSubprotocolMarkerNeverTheToken(t *testing.T) {
	iss := auth.NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, testTokenVersion)

	conn := dial(t, wireServer(t, iss), "extension", token)

	if got := conn.Subprotocol(); got != auth.WSSubprotocolMarker {
		t.Fatalf("negotiated subprotocol = %q, want only the marker", got)
	}
}
