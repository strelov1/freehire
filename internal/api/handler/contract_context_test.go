package handler

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

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

// A NUL byte reaching a query is a Postgres error, so a 500: the wrapper refuses it first.
func TestContractRefusesNULBytes(t *testing.T) {
	app := talentCatalogApp(&fakeTalentCatalogStore{}, nil)
	for target, want := range map[string]int{
		"/api/v1/talent?skills=go%00x": fiber.StatusBadRequest,
		"/api/v1/talent?sk%00ills=go":  fiber.StatusBadRequest,
		"/api/v1/talent/a%00b":         fiber.StatusBadRequest,
		"/api/v1/talent?skills=go":     fiber.StatusOK,
	} {
		var fctx fasthttp.RequestCtx
		fctx.Request.SetRequestURI(target)
		app.Handler()(&fctx)
		if got := fctx.Response.StatusCode(); got != want {
			t.Errorf("%s = %d %s, want %d", target, got, fctx.Response.Body(), want)
		}
	}
}
