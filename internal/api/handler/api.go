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
	// trackingMaxLimit is the higher ceiling for the caller's own tracked-jobs
	// listing: the Kanban board is unpaginated (it fetches everything at once), so
	// the shared 100 cap would silently hide a heavy user's older applications.
	trackingMaxLimit = 500
	// maxPageWindow bounds how deep pagination may reach (offset+limit) on every list
	// endpoint, whichever store answers it. ~500 pages at the default limit, 100 at the
	// maximum — past any real browsing, and the cost of one refused request is that the
	// caller must narrow its filter or page by cursor instead.
	//
	// One number for both stores on purpose. It arrived first for the Meili-backed search,
	// where the index's own deep-paging cost argued for it; the Postgres-backed lists were
	// left unbounded because their OFFSET walk was slow rather than refused — which is the
	// gap the 2026-09-14 outage went through. Why a request past it is REFUSED rather than
	// clamped is argued at pageParamsWindowed.
	maxPageWindow = 10000
	// telegramLinkTTL bounds how long a deep-link token is valid — long enough to
	// open Telegram and tap Start, short enough to limit a leaked link's window.
	telegramLinkTTL = 10 * time.Minute
	// matchAnalysisLLMTimeout is the per-stage LLM timeout for the fit analysis: its reasoning
	// model spends tens of seconds thinking before answering, so a stage needs more than
	// the shared client's default.
	//
	// It is a budget for ONE attempt of the stages a reader is blocked on. What each stage
	// does with it is matchanalysis's own business — timeoutForStage doubles it for the
	// adversarial audit, whose subject is already served, and attemptsForStage decides how
	// many attempts each stage gets.
	//
	// 90s was chosen against a spread of 3–25s, which is no longer what production shows:
	// measured 2026-09-08, healthy stages answer in 45–83s, because a call landing on the
	// gateway's deliberating provider is slow rather than hung. That is why the retry no
	// longer helps the stage it was written for and why the audit needed a budget of its
	// own; the figure itself still sits past the spread for stages 1 and 2.
	matchAnalysisLLMTimeout = 90 * time.Second
	// resumeExtractLLMTimeout bounds the single structured-résumé extraction call. It runs
	// off the upload response path (background) so it can be generous, but still bounded so
	// a stalled gateway cannot leak a goroutine indefinitely.
	resumeExtractLLMTimeout = 120 * time.Second
	// assistantLLMTimeout bounds ONE model call in an assistant turn. The shared default of
	// 90s was measured, on 2026-08-21, to sit inside the working range rather than past it:
	// over three days of tailoring turns the gateway's own spend log put p95 at 58s and the
	// slowest SUCCESSFUL call at 83.1s, with the reasoning model taking up to 51.8s before
	// its first token on a single call. A ceiling that close to the spread cuts work that
	// was going to finish.
	//
	// And a cut call does not cost a step, it costs the run: Runner.Run surfaces a model
	// error as a failed turn, so one slow round ends an unattended pass that had thirty. On
	// 2026-08-21 that happened to three runs, each on its SECOND call — the first round of
	// real work, where the model writes several thousand tokens off the fit analysis.
	//
	// 180s is twice the slowest observed success, and still well under the gateway's own
	// limits (its nginx reads for 300s), so a genuinely hung call is still bounded here.
	assistantLLMTimeout = 180 * time.Second
)

// API holds the cross-cutting dependencies every route shares: the DB pool, the
// sqlc queries, and the token issuer the auth middleware is built from. Feature
// handlers carry their own dependencies (see the *Handlers structs) and register
// their own routes; Register wires them onto the app.
type API struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	issuer  *auth.Issuer
	// browserTools relays browser-tool frames between a user's harness and that
	// user's browser extension (the /tools/ws wire). In-memory and per-instance:
	// both ends of a channel are live connections to this process.
	browserTools *browsertools.Hub
}

// middleware bundles the auth gates the feature handlers mount their routes
// behind. optional attaches the caller when signed in (cookie or key) but never
// rejects, so public detail reads can overlay the caller's own state (my_vote)
// while staying open to anonymous visitors. key accepts the session cookie or an
// API key (RequireAuthOrKey), so a script holding a key can drive the same flow as
// the browser. cookie is the cookie-only gate (RequireAuth) for the
// browser-convenience surfaces where a leaked API key must not act. moderator
// gates on the moderator role and is stacked after key or cookie.
type middleware struct {
	optional fiber.Handler
	key      fiber.Handler
	// cvKey is keyAuth widened to admit the narrow `cv` key. Only the caller's own
	// identity read (/auth/me) mounts it; every other key-accepting route stays
	// on key, which is full-scope-only — so a new endpoint is out of a leaked agent
	// credential's reach unless it opts in.
	cvKey fiber.Handler
	// autoApplyGate is keyAuth (cookie or full-scope API key) widened with a fallback
	// to the shared auto-apply orchestrator secret (see
	// openspec/changes/auto-apply-inngest-orchestration/design.md) — only the two
	// auto-apply tailor/review routes mount it; every other route stays on key.
	autoApplyGate fiber.Handler
	cookie        fiber.Handler
	// optionalCookie attaches a cookie session when there is one but never rejects.
	// It exists for provider callbacks, which are browser navigations: a 401 there
	// renders JSON into the address bar instead of sending the user back to the app.
	optionalCookie fiber.Handler
	moderator      fiber.Handler
	// paidAI is the paid-entitlement gate (paid_ai.go) every route that makes a model call
	// for its caller mounts AFTER its authentication gate. Called with the metered feature the
	// route draws on, so a refusal names it, or with the zero Feature for an AI action that
	// has no allowance of its own. Free gets no AI at all: an unpaid caller is a 402 before
	// the handler runs, and a plan that cannot be read is a 503.
	//
	// The binding (user_llm.go) refuses to hand out a model client without the mark this
	// leaves, so forgetting it on a new route is a 500 in that route's first test rather
	// than model spend nobody paid for.
	paidAI func(plan.Feature) fiber.Handler
}
