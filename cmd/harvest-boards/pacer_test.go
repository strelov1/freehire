package main

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/net/html"
)

// countingWaiter stands in for the rate limiter so the gate can be asserted without timing
// flake: it records every admission and can refuse, the way a cancelled context does.
type countingWaiter struct {
	admitted int
	err      error
}

func (w *countingWaiter) Wait(context.Context) error {
	w.admitted++
	return w.err
}

// recordingClient counts the calls that actually reached the transport.
type recordingClient struct{ calls int }

func (c *recordingClient) GetJSON(context.Context, string, any) error { c.calls++; return nil }
func (c *recordingClient) PostJSON(context.Context, string, any, any) error {
	c.calls++
	return nil
}
func (c *recordingClient) GetXML(context.Context, string, any) error { c.calls++; return nil }
func (c *recordingClient) GetHTML(context.Context, string) (*html.Node, error) {
	c.calls++
	return nil, nil
}
func (c *recordingClient) GetText(context.Context, string) (string, error) {
	c.calls++
	return "", nil
}
func (c *recordingClient) GetJSONWithHeaders(context.Context, string, map[string]string, any) error {
	c.calls++
	return nil
}
func (c *recordingClient) PostJSONWithHeaders(context.Context, string, map[string]string, any, any) error {
	c.calls++
	return nil
}

// Every verb a prober can reach the platform through has to pass the same bucket. A verb that
// skipped it would let a provider whose prober uses it run unpaced — the whole point is an
// aggregate rate, not a per-method one.
func TestPacedClientGatesEveryVerb(t *testing.T) {
	w := &countingWaiter{}
	inner := &recordingClient{}
	c := pacedClient{inner: inner, limiter: w}
	ctx := context.Background()

	_ = c.GetJSON(ctx, "u", nil)
	_ = c.PostJSON(ctx, "u", nil, nil)
	_ = c.GetXML(ctx, "u", nil)
	_, _ = c.GetHTML(ctx, "u")
	_, _ = c.GetText(ctx, "u")
	_ = c.GetJSONWithHeaders(ctx, "u", nil, nil)
	_ = c.PostJSONWithHeaders(ctx, "u", nil, nil, nil)

	if w.admitted != 7 {
		t.Errorf("admitted %d requests, want 7 — a verb is bypassing the pacer", w.admitted)
	}
	if inner.calls != 7 {
		t.Errorf("inner calls = %d, want 7", inner.calls)
	}
}

// A refused admission (a cancelled run) must not reach the platform: the pacer exists to keep
// requests off the wire, so delegating anyway would defeat it at exactly the wrong moment.
func TestPacedClientDoesNotFetchWhenRefused(t *testing.T) {
	w := &countingWaiter{err: errors.New("cancelled")}
	inner := &recordingClient{}
	c := pacedClient{inner: inner, limiter: w}

	if err := c.GetJSON(context.Background(), "u", nil); err == nil {
		t.Error("a refused admission must surface as an error")
	}
	if inner.calls != 0 {
		t.Errorf("inner calls = %d, want 0 — the fetch went out despite the refusal", inner.calls)
	}
}
