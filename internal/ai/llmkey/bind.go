package llmkey

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// Bind binds a model client to the caller's own gateway credential and tags the call with
// the feature it serves.
//
// EVERY model call made for a signed-in user goes through here: a call site that forgets to
// attribute keeps working and spends anonymously, so this is the one identifier to grep for.
//
// It cannot fail: an unresolvable credential, unconfigured gateway or unreachable admin API
// all yield a client on the service credential, still tagged. A nil model stays nil.
//
// Use Bind only when there is no client of your own yet; a caller that already holds a
// configured client would lose its configuration, so it takes Caller instead.
func Bind(ctx context.Context, keys *Resolver, client *llm.Client, userID int64, dims ...llm.Dimension) *llm.Client {
	if client == nil {
		return nil
	}
	return client.AsCaller(Caller(ctx, keys, userID, dims...))
}

// Caller resolves who a call spends as and returns only that, no client. A component with
// its own client (a deliberately chosen timeout, say) takes on the candidate's identity
// through it and keeps its settings; see llm.Caller.
//
// Same contract as Bind: it cannot fail, and an unresolvable credential yields a tagged
// Caller on the service credential.
func Caller(ctx context.Context, keys *Resolver, userID int64, dims ...llm.Dimension) llm.Caller {
	secret := keys.For(ctx, userID)
	if secret == "" {
		return llm.NewCaller("", nil, dims...)
	}
	// Forget on a context detached from the caller's: a refusal often arrives after the tab
	// closed or the run's deadline passed, and the rejected credential must still be cleared.
	forget := func() { keys.Forget(context.WithoutCancel(ctx), userID, secret) }

	return llm.NewCaller(secret, forget, dims...)
}
