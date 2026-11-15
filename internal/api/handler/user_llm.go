package handler

import (
	"context"
	"errors"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// Feature tags, one per thing a person can ask for, so spend can be grouped by feature. They
// are the gateway's grouping key and outlive any row, hence constants.
// CV tailoring is deliberately absent: it makes no model call of its own (it is an assistant
// turn under the `tailor` preset, already tagged), and a second tag would double-count.
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

// llmBinding is what a per-user surface needs to spend as its caller: the client the feature
// runs on and the resolver that names the account on the gateway. One field, so the two cannot
// be wired half-way; the zero value is the unconfigured deployment (spend on the service credential).
type llmBinding struct {
	client *llm.Client
	keys   *llmkey.Resolver
}

// errUnpaidModelCall is what the binding answers for a context the paid-AI gate did not mark.
// It is OUR fault (a model-calling route mounted without mw.paidAI, or a goroutine that dropped
// the request context), so it renders as a reported 500, not an upgrade prompt.
var errUnpaidModelCall = errors.New("handler: model call without a verified paid entitlement")

// requirePaidCall is the binding's check, also made directly by model calls that bypass the
// binding (dictation, the voice token). Calls marked plan.WithFreeAI (CV parse, onboarding CV
// read, search intent, support agent) pass too.
func requirePaidCall(ctx context.Context) error {
	if !plan.PaidAI(ctx) && !plan.FreeAI(ctx) {
		return errUnpaidModelCall
	}
	return nil
}

// bind returns the client one call should travel on. See llmkey.Bind.
//
// It FAILS CLOSED: a context the paid-AI gate (paid_ai.go) did not mark gets no client. This is
// the structural half of the paid gate: every user-attributed call comes through here or
// `caller`, so a route wired without the middleware fails its first test. Past that check it
// cannot fail: an unresolvable credential falls back to the service credential.
//
// Only for building something out of this client. A component that ALREADY holds a client
// takes `caller` instead, so its own configuration survives. See llm.Caller.
func (b llmBinding) bind(ctx context.Context, userID int64, dims ...llm.Dimension) (*llm.Client, error) {
	if err := requirePaidCall(ctx); err != nil {
		return nil, err
	}
	return llmkey.Bind(ctx, b.keys, b.client, userID, dims...), nil
}

// caller returns who one call spends as, for a component to apply to its own client. See
// llmkey.Caller. It fails closed like bind.
func (b llmBinding) caller(ctx context.Context, userID int64, dims ...llm.Dimension) (llm.Caller, error) {
	if err := requirePaidCall(ctx); err != nil {
		return llm.Caller{}, err
	}
	return llmkey.Caller(ctx, b.keys, userID, dims...), nil
}
