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
	tagCoverLetter   = "cover-letter"
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

// bind returns the client one call should travel on. See llmkey.Bind: it cannot fail.
func (b llmBinding) bind(ctx context.Context, userID int64, dims ...llm.Dimension) *llm.Client {
	return llmkey.Bind(ctx, b.keys, b.client, userID, dims...)
}
