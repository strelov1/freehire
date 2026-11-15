package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	// trackingMaxLimit is the higher ceiling for the caller's own tracked-jobs listing: the
	// Kanban board is unpaginated, so the shared 100 cap would hide older applications.
	trackingMaxLimit = 500
	// maxPageWindow bounds how deep pagination may reach (offset+limit) on every list endpoint,
	// whichever store answers it. Past it the caller must narrow the filter or page by cursor.
	// Why it is REFUSED rather than clamped is argued at pageParamsWindowed.
	maxPageWindow = 10000
	// telegramLinkTTL bounds how long a deep-link token is valid: long enough to open Telegram
	// and tap Start, short enough to limit a leaked link's window.
	telegramLinkTTL = 10 * time.Minute
	// matchAnalysisLLMTimeout is the per-stage LLM timeout for the fit analysis. Its reasoning
	// model is slow (healthy stages take up to ~80s), so a stage needs more than the shared
	// default. It is a budget for ONE attempt of the stages a reader is blocked on;
	// matchanalysis's timeoutForStage and attemptsForStage adjust it per stage.
	matchAnalysisLLMTimeout = 90 * time.Second
	// resumeExtractLLMTimeout bounds the single structured-résumé extraction call. It runs in
	// the background, so it can be generous, but bounded so a stalled gateway cannot leak a goroutine.
	resumeExtractLLMTimeout = 120 * time.Second
	// assistantLLMTimeout bounds ONE model call in an assistant turn. A cut call fails the whole
	// turn (Runner.Run surfaces a model error), so the ceiling must sit well past the slowest
	// successful call (~83s). Still under the gateway's own limits (nginx reads for 300s), so a
	// hung call is bounded here.
	assistantLLMTimeout = 180 * time.Second
)

// API holds the cross-cutting dependencies every route shares. Feature handlers carry their
// own dependencies (see the *Handlers structs) and register their own routes; Register wires
// them onto the app.
type API struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	issuer  *auth.Issuer
	// browserTools relays browser-tool frames between a user's harness and browser extension
	// (the /tools/ws wire). In-memory and per-instance: both ends are live connections here.
	browserTools *browsertools.Hub
}

// middleware bundles the auth gates the feature handlers mount their routes behind. optional
// attaches the caller when signed in but never rejects, so public reads can overlay the
// caller's own state. key accepts the session cookie or an API key (RequireAuthOrKey). cookie
// is the cookie-only gate (RequireAuth) for surfaces where a leaked API key must not act.
// moderator gates on the moderator role and is stacked after key or cookie.
type middleware struct {
	optional fiber.Handler
	key      fiber.Handler
	// cvKey is keyAuth widened to admit the narrow `cv` key. Only /auth/me mounts it; every other
	// key-accepting route stays full-scope-only, so a new endpoint is out of a leaked agent
	// credential's reach unless it opts in.
	cvKey fiber.Handler
	// autoApplyGate is keyAuth widened with a fallback to the shared auto-apply orchestrator
	// secret. Only the two auto-apply tailor/review routes mount it.
	autoApplyGate fiber.Handler
	cookie        fiber.Handler
	// optionalCookie attaches a cookie session when there is one but never rejects. For provider
	// callbacks, which are browser navigations: a 401 there would render JSON in the address bar.
	optionalCookie fiber.Handler
	moderator      fiber.Handler
	// paidAI is the paid-entitlement gate (paid_ai.go) every route that makes a model call for its
	// caller mounts AFTER its authentication gate. Takes the metered feature (zero Feature for an
	// action with no allowance of its own). An unpaid caller is a 402 before the handler runs; an
	// unreadable plan is a 503. The binding (user_llm.go) refuses a model client without the mark
	// this leaves, so a route that forgets it fails its first test.
	paidAI func(plan.Feature) fiber.Handler
}
