package handler

import (
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// newMiddleware builds the auth gates every feature surface mounts its routes behind (see
// the middleware type).
func (a *API) newMiddleware(cfg Config, plans *plan.Store) middleware {
	// optionalAuth attaches the caller when signed in (cookie or key) but never
	// rejects, so these public detail reads can overlay the caller's own vote
	// (my_vote) while staying open to anonymous visitors.
	optionalAuth := auth.OptionalAuth(a.issuer, a.queries, apiKeys{a.queries})
	// keyAuth (RequireAuthOrKey) accepts the session cookie or an API key, so a
	// script holding a key can drive the same flow as the browser. The public job
	// reads stay unauthenticated. Jobs are addressed by their public slug; the
	// handlers resolve it to the internal id before writing user_jobs.
	keyAuth := auth.RequireAuthOrKey(a.issuer, a.queries, apiKeys{a.queries})
	cvKeyAuth := auth.RequireAuthOrScopedKey(a.issuer, a.queries, apiKeys{a.queries}, auth.ScopeCV)
	autoApplyGate := autoApplyOrchestratorGate(cfg.AutoApplyOrchestratorSecret, keyAuth)
	// cookieAuth is the single cookie-only gate (RequireAuth) for the
	// browser-convenience surfaces below — key management, saved searches, the CV
	// builder, the inbox, subscriptions — where a leaked API key must not act.
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
