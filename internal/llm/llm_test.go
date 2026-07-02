package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// fakeModel is a stub llms.Model returning a canned response, capturing the
// messages it was sent. genInfo, when set, is attached to the choice as
// langchaingo's usage-token map.
type fakeModel struct {
	resp    string
	err     error
	genInfo map[string]any
	gotMsgs []llms.MessageContent
}

func (f *fakeModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	f.gotMsgs = msgs
	if f.err != nil {
		return nil, f.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: f.resp, GenerationInfo: f.genInfo}}}, nil
}

// captureTracer records the generations it observes.
type captureTracer struct{ got []Generation }

func (c *captureTracer) Observe(g Generation)           { c.got = append(c.got, g) }
func (c *captureTracer) Shutdown(context.Context) error { return nil }

func TestGenerateJSON_observesSuccessWithUsage(t *testing.T) {
	f := &fakeModel{
		resp:    `{"a":1}`,
		genInfo: map[string]any{"PromptTokens": 1200, "CompletionTokens": 40, "TotalTokens": 1240},
	}
	ct := &captureTracer{}
	c := &Client{model: f, timeout: time.Second, modelID: "qwen2.5-72b", tracer: ct, source: "enrich"}

	if _, err := c.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if len(ct.got) != 1 {
		t.Fatalf("observed %d generations, want 1", len(ct.got))
	}
	g := ct.got[0]
	if g.Model != "qwen2.5-72b" || g.System != "sys" || g.User != "usr" || g.Output != `{"a":1}` {
		t.Errorf("generation fields wrong: %+v", g)
	}
	if g.Source != "enrich" {
		t.Errorf("source = %q, want enrich", g.Source)
	}
	if g.Usage == nil || g.Usage.Input != 1200 || g.Usage.Output != 40 || g.Usage.Total != 1240 {
		t.Errorf("usage = %+v, want 1200/40/1240", g.Usage)
	}
	if !g.End.After(g.Start) && g.End != g.Start {
		t.Errorf("timestamps not set: start=%v end=%v", g.Start, g.End)
	}
	if g.Err != nil {
		t.Errorf("Err = %v, want nil on success", g.Err)
	}
}

func TestGenerateJSON_observesSuccessOmitsUsageWhenAbsent(t *testing.T) {
	f := &fakeModel{resp: `{"a":1}`} // no genInfo
	ct := &captureTracer{}
	c := &Client{model: f, timeout: time.Second, modelID: "m", tracer: ct, source: "enrich"}

	if _, err := c.GenerateJSON(context.Background(), "s", "u"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if len(ct.got) != 1 || ct.got[0].Usage != nil {
		t.Errorf("usage should be nil when the model reported none, got %+v", ct.got[0].Usage)
	}
}

func TestGenerateJSON_observesErrorAndReturnsUnchangedError(t *testing.T) {
	sentinel := errors.New("gateway boom")
	f := &fakeModel{err: sentinel}
	ct := &captureTracer{}
	c := &Client{model: f, timeout: time.Second, modelID: "m", tracer: ct, source: "enrich"}

	_, err := c.GenerateJSON(context.Background(), "s", "u")
	if !errors.Is(err, sentinel) {
		t.Errorf("returned error %v does not wrap the model error", err)
	}
	if len(ct.got) != 1 || ct.got[0].Err == nil {
		t.Fatalf("expected one error generation, got %+v", ct.got)
	}
	if ct.got[0].Output != "" {
		t.Errorf("error generation should have no output, got %q", ct.got[0].Output)
	}
}

func TestGenerateJSON_nilTracerIsUnchanged(t *testing.T) {
	// A client with no tracer must behave exactly as before: no panic, same result.
	f := &fakeModel{resp: `{"a":1}`}
	c := &Client{model: f, timeout: time.Second}
	got, err := c.GenerateJSON(context.Background(), "s", "u")
	if err != nil || got != `{"a":1}` {
		t.Errorf("nil-tracer client: got %q err %v, want clean result", got, err)
	}
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
