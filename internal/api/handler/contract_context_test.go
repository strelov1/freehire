package handler

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ai/plan"
)

type ctxProbeKey struct{}

// A contract handler's context answers what middleware put in Fiber's user context (the
// paid-AI mark, the trace) and keeps the request's own values and cancellation.
func TestWithUserContextAnswersBoth(t *testing.T) {
	req, cancel := context.WithCancel(context.WithValue(context.Background(), ctxProbeKey{}, "request"))
	user := plan.WithPaidAI(context.Background())
	ctx := withUserContext{Context: req, user: user}

	if !plan.PaidAI(ctx) {
		t.Error("the paid-AI mark from the user context is lost")
	}
	if got := ctx.Value(ctxProbeKey{}); got != "request" {
		t.Errorf("request value = %v, want it still answered", got)
	}
	cancel()
	if ctx.Err() == nil {
		t.Error("cancelling the request must cancel the handler's context")
	}
}
