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

	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
		HandshakeTimeout: 5 * time.Second,
	}
	conn, resp, err := dialer.Dial(base+"?role="+role, nil)
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

func readFrame(t *testing.T, conn *ws.Conn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(frame)
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

	call := `{"id":"c1","tool":"read_form","args":{}}`
	if err := harness.WriteMessage(ws.TextMessage, []byte(call)); err != nil {
		t.Fatalf("harness write: %v", err)
	}
	if got := readFrame(t, extension); got != call {
		t.Fatalf("extension got %q, want the call verbatim", got)
	}

	result := `{"id":"c1","result":{"fields":[]}}`
	if err := extension.WriteMessage(ws.TextMessage, []byte(result)); err != nil {
		t.Fatalf("extension write: %v", err)
	}
	if got := readFrame(t, harness); got != result {
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
	if err := json.Unmarshal([]byte(readFrame(t, harness)), &answer); err != nil {
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

	if err := harnessA.WriteMessage(ws.TextMessage, []byte(`{"id":"c1","tool":"read_form"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// User A's harness is answered by the relay itself...
	if got := readFrame(t, harnessA); got == `{"id":"c1","tool":"read_form"}` {
		t.Fatal("the call came back to its sender")
	}
	// ...and user B's extension is never handed anything.
	if err := extensionB.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, frame, err := extensionB.ReadMessage(); err == nil {
		t.Fatalf("user B's extension received %q from user A's harness", frame)
	}
}

func TestBrowserToolsWS_RefusesAnUnauthenticatedHandshake(t *testing.T) {
	base := wireServer(t, auth.NewIssuer("secret", time.Hour))

	dialer := ws.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.Dial(base+"?role=extension", nil)
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
