package handler

import (
	"math"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/search"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// API holds dependencies shared across HTTP handlers.
type API struct {
	pool         *pgxpool.Pool
	queries      *db.Queries
	issuer       *auth.Issuer
	cookieSecure bool
	// oauth maps enabled OAuth provider names to their implementations; empty
	// when no provider is configured (the routes then 404 / list empty).
	oauth map[string]oauth.Provider
	// frontendOrigin is where OAuth callbacks send the browser back to.
	frontendOrigin string
	// search is the job-search backend. Nil when Meilisearch is unconfigured —
	// the search endpoint then reports 503 and the rest of the API is unaffected.
	search searcher
	// facets is the analytics (facet-distribution) backend — the same Meilisearch
	// client viewed through a narrower interface, kept separate from search so the
	// two concerns stay decoupled. Nil when unconfigured (endpoint reports 503).
	facets facetCounter
	// tracking owns the per-user job-interaction use cases (view/apply/save/
	// unsave/track); the handlers translate wire ↔ domain and delegate to it.
	tracking *jobtracking.Service
	// accounts resolves external OAuth identities into local user accounts
	// (identity-first lookup, verified-email gate, link-or-create, race retry).
	accounts *accounts.Service
	// moderation owns the moderator-authored job use cases (create/edit a manual
	// vacancy); the handlers translate wire ↔ domain and delegate to it.
	moderation *moderation.Service
}

// pageParams reads and clamps the shared limit/offset pagination query params.
// The offset is clamped into int32 range because the column binds as a Postgres
// int4, and an unbounded query value would otherwise overflow on the conversion.
func pageParams(c *fiber.Ctx) (limit, offset int) {
	limit = min(max(c.QueryInt("limit", defaultLimit), 1), maxLimit)
	offset = min(max(c.QueryInt("offset", 0), 0), math.MaxInt32)
	return limit, offset
}

// listResponse writes the shared paginated-list envelope: the data slice plus a
// meta block carrying the filtered total and the limit/offset echoed back. It is
// the single source of the list wire shape, so the jobs/companies/search list
// endpoints cannot drift from one another.
func listResponse(c *fiber.Ctx, data any, total int64, limit, offset int) error {
	return c.JSON(fiber.Map{
		"data": data,
		"meta": fiber.Map{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// Config is the dependency bundle Register wires onto the app: the DB pool, the
// single browser origin allowed cross-origin (FrontendOrigin), the token-issuer
// settings (JWTSecret/JWTTTL), the HTTPS-only cookie flag (CookieSecure), the
// enabled OAuth providers, and the optional search client (nil disables search).
type Config struct {
	Pool           *pgxpool.Pool
	FrontendOrigin string
	JWTSecret      string
	JWTTTL         time.Duration
	CookieSecure   bool
	OAuthProviders map[string]oauth.Provider
	Search         *search.Client
}

// Register wires all routes onto the application from cfg. Auth is same-origin
// only: the SPA reaches the API under one origin (a dev Vite proxy mirrors the
// production reverse proxy), so the cookie rides along with no CORS. The CORS
// allowlist is not credentialed — it only permits non-credentialed cross-origin
// reads of the public endpoints.
func Register(app *fiber.App, cfg Config) {
	queries := db.New(cfg.Pool)
	a := &API{
		pool:           cfg.Pool,
		queries:        queries,
		issuer:         auth.NewIssuer(cfg.JWTSecret, cfg.JWTTTL),
		cookieSecure:   cfg.CookieSecure,
		oauth:          cfg.OAuthProviders,
		frontendOrigin: cfg.FrontendOrigin,
		tracking:       jobtracking.New(jobtracking.NewQueriesRepository(queries)),
		accounts:       accounts.New(accounts.NewQueriesRepository(queries, cfg.Pool), authHasher{}),
		moderation:     moderation.New(moderation.NewQueriesRepository(queries, cfg.Pool, enrich.Version)),
	}
	// Assign only when configured: a nil *search.Client wrapped in the searcher
	// interface would be a non-nil interface and defeat the nil check.
	if cfg.Search != nil {
		a.search = cfg.Search
		a.facets = cfg.Search
	}

	app.Use(cors.New(cors.Config{AllowOrigins: cfg.FrontendOrigin}))

	app.Get("/health", a.Health)

	api := app.Group("/api/v1")
	api.Get("/jobs", a.ListJobs)
	// Literal routes before the :slug param route so they are not read as slugs.
	api.Get("/jobs/search", a.SearchJobs)
	api.Get("/jobs/facets", a.JobFacets)
	api.Get("/jobs/:slug", a.GetJob)
	api.Get("/companies", a.ListCompanies)
	api.Get("/companies/:slug", a.GetCompany)

	// Per-user job interactions and the user-scoped reads accept either the
	// session cookie or an API key (RequireAuthOrKey), so a script holding a key
	// can drive the same flow as the browser. The public job reads above stay
	// unauthenticated. Jobs are addressed by their public slug; the handlers
	// resolve it to the internal id before writing user_jobs.
	keyAuth := auth.RequireAuthOrKey(a.issuer, a.queries)
	api.Post("/jobs/:slug/view", keyAuth, a.RecordView)
	api.Post("/jobs/:slug/apply", keyAuth, a.MarkApplied)
	api.Post("/jobs/:slug/save", keyAuth, a.SaveJob)
	api.Delete("/jobs/:slug/save", keyAuth, a.UnsaveJob)
	api.Patch("/jobs/:slug/track", keyAuth, a.TrackJob)
	api.Delete("/jobs/:slug/stage", keyAuth, a.ClearStage)
	api.Delete("/jobs/:slug/track", keyAuth, a.Untrack)

	// Moderator-authored jobs: create a hand-curated vacancy and edit it. Authenticated
	// by cookie or API key (the CLI uses a key), then gated on the moderator role. The
	// public job reads above stay unauthenticated; a non-moderator gets 403.
	requireModerator := auth.RequireRole(a.queries, "moderator")
	api.Post("/jobs", keyAuth, requireModerator, a.CreateJob)
	api.Patch("/jobs/:slug", keyAuth, requireModerator, a.UpdateJob)

	// User-scoped reads live under /me (consistent with /auth/me): the my-jobs
	// listing joins the caller's interactions with the jobs they touch, and the
	// viewed-slugs set lets the SPA dim already-seen cards in the public browse
	// list without making that list authenticated.
	api.Get("/me/jobs", keyAuth, a.ListMyJobs)
	api.Get("/me/jobs/viewed", keyAuth, a.ListViewedSlugs)

	// API-key management is cookie-only (RequireAuth): a leaked key must not be
	// able to create, list, or revoke keys. The create endpoint returns the
	// plaintext token exactly once.
	api.Post("/me/api-keys", auth.RequireAuth(a.issuer), a.CreateAPIKey)
	api.Get("/me/api-keys", auth.RequireAuth(a.issuer), a.ListAPIKeys)
	api.Delete("/me/api-keys/:id", auth.RequireAuth(a.issuer), a.RevokeAPIKey)

	// Auth: register/login/logout are public (logout just clears the cookie).
	// me is guarded and accepts a session cookie OR an API key, so a non-browser
	// client (e.g. the CLI) can resolve its own identity with its key. It stays a
	// read of the caller's own user — not key management, which is cookie-only.
	// Throttle the credential endpoints against online brute-force / credential
	// stuffing. Keyed on c.IP() (the real client, via the trusted-proxy config); the
	// per-instance in-memory window is enough friction for a single-node deployment.
	authLimiter := limiter.New(limiter.Config{Max: 10, Expiration: time.Minute})
	authGroup := api.Group("/auth")
	authGroup.Post("/register", authLimiter, a.Register)
	authGroup.Post("/login", authLimiter, a.Login)
	authGroup.Post("/logout", a.Logout)
	authGroup.Get("/me", auth.RequireAuthOrKey(a.issuer, a.queries), a.Me)

	// OAuth sign-in: provider listing plus the authorization-code start and
	// callback redirects. All public; the callback sets the session cookie.
	authGroup.Get("/oauth/providers", a.ListOAuthProviders)
	authGroup.Get("/oauth/:provider/start", a.OAuthStart)
	authGroup.Get("/oauth/:provider/callback", a.OAuthCallback)
}
