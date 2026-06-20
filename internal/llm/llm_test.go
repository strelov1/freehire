package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// fakeModel is a stub llms.Model returning a canned response, capturing the
// messages it was sent.
type fakeModel struct {
	resp    string
	err     error
	gotMsgs []llms.MessageContent
}

func (f *fakeModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	f.gotMsgs = msgs
	if f.err != nil {
		return nil, f.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: f.resp}}}, nil
}

func (f *fakeModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func TestGenerateJSONStripsFenceAndSendsMessages(t *testing.T) {
	f := &fakeModel{resp: "```json\n{\"a\":1}\n```"}
	got, err := NewWithModel(f).GenerateJSON(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if got != `{"a":1}` {
		t.Errorf("content = %q, want fence-stripped JSON", got)
	}
	if len(f.gotMsgs) != 2 || f.gotMsgs[0].Role != llms.ChatMessageTypeSystem {
		t.Errorf("must send system+user messages, got %d", len(f.gotMsgs))
	}
}

func TestGenerateJSONPropagatesModelError(t *testing.T) {
	c := NewWithModel(&fakeModel{err: errors.New("boom")})
	if _, err := c.GenerateJSON(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected error from model, got nil")
	}
}

// blockingModel hangs until its context is cancelled, modelling a stalled gateway.
type blockingModel struct{}

func (blockingModel) GenerateContent(ctx context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// A stalled gateway must not hang the caller: the per-call timeout cancels the
// request so GenerateJSON returns an error instead of blocking forever.
func TestGenerateJSONTimesOutOnStalledModel(t *testing.T) {
	c := &Client{model: blockingModel{}, timeout: 20 * time.Millisecond}
	done := make(chan error, 1)
	go func() {
		_, err := c.GenerateJSON(context.Background(), "s", "u")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("GenerateJSON returned nil error, want a timeout error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GenerateJSON did not return; the per-call timeout did not fire")
	}
}

func TestTruncateRunes(t *testing.T) {
	// Within the limit: returned unchanged.
	if got := TruncateRunes("héllo", 10); got != "héllo" {
		t.Errorf("under limit: got %q", got)
	}
	// Over the limit: clamped to rune count, never splitting a multi-byte rune.
	if got := TruncateRunes("héllo", 3); got != "hél" {
		t.Errorf("over limit: got %q want %q", got, "hél")
	}
}

func TestStripJSONFence(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":1}\n```":     `{"a":1}`,
		`{"a":1}`:                 `{"a":1}`,
		"  {\"a\":1}  ":           `{"a":1}`,
	}
	for in, want := range cases {
		if got := StripJSONFence(in); got != want {
			t.Errorf("StripJSONFence(%q) = %q, want %q", in, got, want)
		}
	}
}
