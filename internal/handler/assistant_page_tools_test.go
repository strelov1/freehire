package handler

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/browsertools"
)

// extensionStub is the browser end of one user's channel. It answers every call
// with a fixed result and records what it was asked, so a test can assert on the
// primitive the tool reached for without standing up a WebSocket.
type extensionStub struct {
	hub    *browsertools.Hub
	user   int64
	result string

	mu    sync.Mutex
	asked []string
}

func (e *extensionStub) Send(frame []byte) error {
	var call struct {
		ID   string `json:"id"`
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(frame, &call); err != nil {
		return err
	}
	e.mu.Lock()
	e.asked = append(e.asked, call.Tool)
	e.mu.Unlock()

	answer, err := json.Marshal(map[string]any{"id": call.ID, "result": json.RawMessage(e.result)})
	if err != nil {
		return err
	}
	e.hub.Forward(e.user, browsertools.RoleExtension, answer)
	return nil
}

func (e *extensionStub) tools() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.asked...)
}

// attachExtension joins the stub as the given user's browser end.
func attachExtension(t *testing.T, hub *browsertools.Hub, user int64, result string) *extensionStub {
	t.Helper()
	stub := &extensionStub{hub: hub, user: user, result: result}
	leave := hub.Join(user, browsertools.RoleExtension, stub)
	t.Cleanup(leave)
	return stub
}

func TestReadCurrentPageReturnsWhatTheBrowserIsShowing(t *testing.T) {
	hub := browsertools.New()
	stub := attachExtension(t, hub, 7, `{
		"url": "https://jobs.example.test/senior-go",
		"title": "Senior Go Engineer — Example",
		"headline": "Senior Go Engineer",
		"text": "We run a large Go fleet and are hiring."
	}`)
	h := &assistantHandlers{browserTools: hub}

	out, err := h.readCurrentPageTool().Run(context.Background(), 7, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	blob, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("the tool's result must encode as JSON: %v", err)
	}
	var got struct {
		URL      string `json:"url"`
		Title    string `json:"title"`
		Headline string `json:"headline"`
		Text     string `json:"text"`
	}
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.URL != "https://jobs.example.test/senior-go" || got.Headline != "Senior Go Engineer" {
		t.Errorf("result = %+v, want the page's own url and headline", got)
	}
	if !strings.Contains(got.Text, "large Go fleet") {
		t.Errorf("result text = %q, want the page's text", got.Text)
	}
	if asked := stub.tools(); len(asked) != 1 || asked[0] != "read_page" {
		t.Errorf("the browser was asked for %v, want one read_page call", asked)
	}
}

// The common case is no panel open — a web session, or one the user closed. The
// model has to be able to say "open the side panel", which means the turn must
// survive and the message must name the remedy.
func TestReadCurrentPageWithNoBrowserAttachedIsARecoverableToolError(t *testing.T) {
	hub := browsertools.New()
	h := &assistantHandlers{browserTools: hub}
	reg := assistant.NewRegistry(h.readCurrentPageTool())

	res := reg.Call(context.Background(), 7, "read_current_page", json.RawMessage(`{}`))

	if !res.Failed {
		t.Fatalf("a call with no browser attached rendered as a success: %s", res.Content)
	}
	if !strings.Contains(res.Content, "side panel") {
		t.Errorf("error = %s, want it to name the side panel as the remedy", res.Content)
	}
}

// An assembly with no relay at all — a partially wired handler in a test, or a
// future deployment that drops the hub — must answer, not panic. A panic inside a
// tool takes the whole turn down; an error is something the model reads.
func TestReadCurrentPageWithoutARelayAnswersRatherThanPanicking(t *testing.T) {
	h := &assistantHandlers{}
	reg := assistant.NewRegistry(h.readCurrentPageTool())

	res := reg.Call(context.Background(), 7, "read_current_page", json.RawMessage(`{}`))

	if !res.Failed || !strings.Contains(res.Content, "side panel") {
		t.Errorf("result = %+v, want a failed result naming the side panel", res)
	}
}

// Another user's browser must be unreachable even when one is attached: the
// channel is keyed by the id the middleware resolved, never by anything a caller
// supplies.
func TestReadCurrentPageReachesOnlyTheCallersOwnBrowser(t *testing.T) {
	hub := browsertools.New()
	attachExtension(t, hub, 7, `{"url":"https://jobs.example.test/seven","title":"Seven","headline":"Seven","text":"seven"}`)
	h := &assistantHandlers{browserTools: hub}
	reg := assistant.NewRegistry(h.readCurrentPageTool())

	res := reg.Call(context.Background(), 8, "read_current_page", json.RawMessage(`{}`))

	if !res.Failed {
		t.Fatalf("user 8 read user 7's browser: %s", res.Content)
	}
}
