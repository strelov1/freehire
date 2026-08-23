package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// Feature tags. One per thing a person can ask for, because the question this exists to
// answer is which of them costs what. They are the gateway's grouping key and outlive any
// row we hold, so they are constants rather than strings written at each call site.
// CV tailoring is deliberately absent. It makes no model call of its own: the endpoint
// mints a CV and debits credits, and the work is an assistant turn under the `tailor`
// preset — already tagged as such. A second tag would double-count the same spend.
const (
	tagAssistant     = "assistant"
	tagMatchAnalysis = "match-analysis"
	tagCVExtract     = "cv-extract"
	tagATSReview     = "ats-review"
	tagAutofill      = "autofill"
	tagMailRecall    = "mail-recall"
	tagSearchIntent  = "search-intent"
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

// bind returns the client one call should travel on. See userLLM: it cannot fail.
func (b llmBinding) bind(ctx context.Context, userID int64, dims ...llm.Dimension) *llm.Client {
	return userLLM(ctx, b.keys, b.client, userID, dims...)
}

// userLLM binds a model client to the caller's own gateway credential and labels the call
// with the feature it serves.
//
// EVERY model call made on behalf of a signed-in user goes through here. That is the whole
// reason it exists as one function: a call site that forgets to attribute keeps working
// perfectly and simply spends anonymously, which is the failure nobody notices, so the
// thing to grep for is a single identifier.
//
// It cannot fail. An unresolvable credential, an unconfigured gateway and an unreachable
// admin API all return a client that spends on the service credential — still tagged,
// because knowing which feature spent is useful even when the person could not be worked
// out. A nil model stays nil, so a deployment with no LLM keeps reporting the feature off.
func userLLM(ctx context.Context, keys *llmkey.Resolver, client *llm.Client, userID int64, dims ...llm.Dimension) *llm.Client {
	if client == nil {
		return nil
	}
	secret := keys.For(ctx, userID)
	if secret == "" {
		return client.As("", nil, dims...)
	}
	// Forgetting runs on a context detached from the caller's. A refusal is most likely
	// to arrive exactly when someone has closed the tab, and a credential the gateway has
	// rejected must be cleared then too — otherwise every later call by that account
	// pays for one abandoned request.
	forget := func() { keys.Forget(context.WithoutCancel(ctx), userID, secret) }

	return client.As(secret, forget, dims...)
}
