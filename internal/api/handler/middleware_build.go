package handler

import (
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// newMiddleware builds the auth gates every feature surface mounts its routes behind (see
// the middleware type).
func (a *API) newMiddleware(cfg Config, plans *plan.Store) middleware {
	// optionalAuth attaches the caller when signed in but never rejects, so public detail
	// reads can overlay the caller's my_vote.
	optionalAuth := auth.OptionalAuth(a.issuer, a.queries, apiKeys{a.queries})
	// keyAuth (RequireAuthOrKey) accepts the session cookie or an API key. Public job reads
	// stay unauthenticated; jobs are addressed by public slug and handlers resolve the id.
	keyAuth := auth.RequireAuthOrKey(a.issuer, a.queries, apiKeys{a.queries})
	cvKeyAuth := auth.RequireAuthOrScopedKey(a.issuer, a.queries, apiKeys{a.queries}, auth.ScopeCV)
	autoApplyGate := autoApplyOrchestratorGate(cfg.AutoApplyOrchestratorSecret, keyAuth)
	// cookieAuth (RequireAuth) is the cookie-only gate for browser-convenience surfaces (key
	// management, saved searches, CV builder, inbox, subscriptions) where a leaked API key
	// must not act.
	cookieAuth := auth.RequireAuth(a.issuer, a.queries)
	requireModerator := auth.RequireRole(a.queries, "moderator")
	return middleware{
		optional:       optionalAuth,
		key:            keyAuth,
		cvKey:          cvKeyAuth,
		autoApplyGate:  autoApplyGate,
		cookie:         cookieAuth,
		optionalCookie: auth.OptionalCookieAuth(a.issuer, a.queries),
		moderator:      requireModerator,
		paidAI:         paidAIGate(entitlementOf(plans)),
	}
}
