package handler

import (
	"context"
	"errors"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// Feature tags. One per thing a person can ask for, because the question this exists to
// answer is which of them costs what. They are the gateway's grouping key and outlive any
// row we hold, so they are constants rather than strings written at each call site.
// CV tailoring is deliberately absent. It makes no model call of its own: the endpoint
// mints a CV and debits credits, and the work is an assistant turn under the `tailor`
// preset — already tagged as such. A second tag would double-count the same spend.
const (
	tagAssistant       = "assistant"
	tagMatchAnalysis   = "match-analysis"
	tagCVExtract       = "cv-extract"
	tagATSReview       = "ats-review"
	tagAutofill        = "autofill"
	tagMailRecall      = "mail-recall"
	tagSearchIntent    = "search-intent"
	tagCoverLetter     = "cover-letter"
	tagReferralMessage = "referral-message"
	tagSupport         = "support"
	tagCVOnboard       = "cv-onboard"
)

// llmBinding is what a per-user surface needs to spend as its caller: the client the
// feature runs on, and the resolver that names the account on the gateway.
//
// It travels as one field so a handler acquires attribution by gaining a single
// dependency rather than two that can be wired half-way — and its zero value is the
// unconfigured deployment, which every path already treats as "spend on the service
// credential".
type llmBinding struct {
	client *llm.Client
	keys   *llmkey.Resolver
}

// errUnpaidModelCall is what the binding answers for a context the paid-AI gate did not
// mark. It is OUR fault, never the caller's — a route that makes a model call was mounted
// without mw.paidAI, or a goroutine dropped the request's context on the way — so it renders
// as a 500 and is reported rather than reading as an upgrade prompt. A paying caller who
// hits it has found a bug.
var errUnpaidModelCall = errors.New("handler: model call without a verified paid entitlement")

// requirePaidCall is the binding's own check, and the one model calls that never come
// through the binding (dictation, the voice token) make for themselves. A call the product
// deliberately gives every account (plan.WithFreeAI: the CV parse, the onboarding CV read, the
// search intent and the support agent) passes it too.
func requirePaidCall(ctx context.Context) error {
	if !plan.PaidAI(ctx) && !plan.FreeAI(ctx) {
		return errUnpaidModelCall
	}
	return nil
}

// bind returns the client one call should travel on. See llmkey.Bind.
//
// It FAILS CLOSED: a context the paid-AI gate (paid_ai.go) did not mark gets no client. That
// is the structural half of the paid gate — every user-attributed model call comes through
// here or `caller` below, so a new one wired without the middleware fails its first test
// instead of spending on somebody who never paid. Past that check it cannot fail: an
// unresolvable credential still yields a client, on the service credential.
//
// Only for a caller building something out of this client. A component that ALREADY holds a
// client — an extractor, an analyzer, a recall service — takes `caller` below instead, so its
// own configuration survives being attributed to a person. See llm.Caller.
func (b llmBinding) bind(ctx context.Context, userID int64, dims ...llm.Dimension) (*llm.Client, error) {
	if err := requirePaidCall(ctx); err != nil {
		return nil, err
	}
	return llmkey.Bind(ctx, b.keys, b.client, userID, dims...), nil
}

// caller returns who one call spends as, for a component to apply to its own client. See
// llmkey.Caller. It fails closed exactly as bind does, and for the same reason.
func (b llmBinding) caller(ctx context.Context, userID int64, dims ...llm.Dimension) (llm.Caller, error) {
	if err := requirePaidCall(ctx); err != nil {
		return llm.Caller{}, err
	}
	return llmkey.Caller(ctx, b.keys, userID, dims...), nil
}
