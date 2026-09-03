package llmkey

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// Bind binds a model client to the caller's own gateway credential and labels the call
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
//
// Originally internal/api/handler's userLLM, unexported and callable only from Fiber
// handlers. Extracted here once cmd/auto-apply became a second real caller with the same
// ownership shape (a specific candidate's own work, not owner-less background work) but no
// reason to import internal/api/handler — see
// openspec/changes/auto-apply-llm-drafting/design.md.
func Bind(ctx context.Context, keys *Resolver, client *llm.Client, userID int64, dims ...llm.Dimension) *llm.Client {
	if client == nil {
		return nil
	}
	secret := keys.For(ctx, userID)
	if secret == "" {
		return client.As("", nil, dims...)
	}
	// Forgetting runs on a context detached from the caller's. A refusal is most likely
	// to arrive exactly when someone has closed the tab (or, for a queue-triggered caller,
	// when the run's own deadline has passed), and a credential the gateway has rejected
	// must be cleared then too — otherwise every later call by that account pays for one
	// abandoned request.
	forget := func() { keys.Forget(context.WithoutCancel(ctx), userID, secret) }

	return client.As(secret, forget, dims...)
}
