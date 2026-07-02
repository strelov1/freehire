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
	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/report"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/savedsearch"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/searchprofile"
	"github.com/strelov1/freehire/internal/submission"
	"github.com/strelov1/freehire/internal/subscription"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	// telegramLinkTTL bounds how long a deep-link token is valid — long enough to
	// open Telegram and tap Start, short enough to limit a leaked link's window.
	telegramLinkTTL = 10 * time.Minute
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
	// submission owns the public job-submission queue (submit/list/approve/reject);
	// approval mints a live job by delegating to moderation.
	submission *submission.Service
	// report owns the job-report moderation queue (file/list/resolve/dismiss);
	// resolving may soft-close the reported job through the job-lifecycle close path.
	report *report.Service
	// savedSearch owns the per-user saved-search use cases (list/create/update/delete
	// named filter snapshots); the handlers translate wire ↔ domain and delegate to it.
	savedSearch *savedsearch.Service
	// subscription owns the per-user filter-subscription use cases (subscribe a
	// saved search to a channel, list/toggle/unsubscribe).
	subscription *subscription.Service
	// searchProfile owns the per-user search-profile use cases (list/create/update/
	// delete a named specialization + skills); the handlers translate wire ↔ domain
	// and delegate to it.
	searchProfile *searchprofile.Service
	// resume owns the per-user stored-résumé use cases (store/status/delete + derive
	// text for the verdict). Its blob store is nil when S3 is unconfigured; Enabled()
	// then reports false and callers degrade to per-request résumé upload.
	resume *resume.Store
	// Telegram notification wiring. All nil/empty when the bot is unconfigured —
	// the linking endpoints then report the feature off and the webhook is inert.
	// telegramLinks mints/verifies the deep-link token; telegramBot replies to the
	// inbound /start; telegramBotUsername builds the t.me URL; telegramWebhookSecret
	// guards the inbound webhook.
	telegramLinks         *telegramnotify.LinkTokens
	telegramBot           *telegramnotify.Client
	telegramBotUsername   string
	telegramWebhookSecret string
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
	// Blob backs résumé storage (internal/blobstore). Nil disables storage: résumé
	// upload only extracts skills in-request (no regression).
	Blob blobstore.Store
	// Telegram bot for notification linking/delivery confirmations. Optional: an
	// empty TelegramBotToken disables the feature (linking endpoints report off,
	// webhook inert). TelegramBotUsername builds the deep link; TelegramWebhookSecret
	// guards the inbound webhook.
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string
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
	// submission approval mints through the same moderation service, so derivation,
	// dedup, and the enrichment enqueue are reused rather than duplicated.
	a.submission = submission.New(submission.NewQueriesRepository(queries), a.moderation)
	// The report queue uses one QueriesRepository for both persistence and the
	// job soft-close (it implements report.Repository and report.JobCloser).
	reportRepo := report.NewQueriesRepository(queries)
	a.report = report.New(reportRepo, reportRepo)
	a.savedSearch = savedsearch.New(savedsearch.NewQueriesRepository(queries))
	a.subscription = subscription.New(subscription.NewQueriesRepository(queries))
	a.searchProfile = searchprofile.New(searchprofile.NewQueriesRepository(queries))
	// Résumé storage is nil-safe: a nil Blob (S3 unconfigured) yields a disabled service
	// whose Enabled() is false, so the upload/verdict paths degrade to in-request parsing.
	a.resume = resume.New(cfg.Blob, resume.NewQueriesRepository(queries))
	// Telegram notifications are enabled only with both a bot token and a JWT
	// secret (the link token reuses it). Absent either, the linking endpoints
	// report the feature off and the webhook is inert (see telegramEnabled).
	if cfg.TelegramBotToken != "" && cfg.JWTSecret != "" {
		a.telegramLinks = telegramnotify.NewLinkTokens(cfg.JWTSecret, telegramLinkTTL)
		a.telegramBot = telegramnotify.NewClient(cfg.TelegramBotToken)
		a.telegramBotUsername = cfg.TelegramBotUsername
		a.telegramWebhookSecret = cfg.TelegramWebhookSecret
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
	api.Get("/jobs/sitemap", a.JobSitemap)
	api.Get("/jobs/:slug", a.GetJob)
	api.Get("/jobs/:slug/similar", a.SimilarJobs)
	api.Get("/companies", a.ListCompanies)
	api.Get("/companies/sitemap", a.CompanySitemap)
	api.Get("/companies/sitemap/boundaries", a.CompanySitemapBoundaries)
	api.Get("/companies/:slug", a.GetCompany)

	// Public read of a shared saved-search "board" by its slug — unauthenticated, like
	// the job/company reads above. Owner identity is never exposed (see boardResponse).
	api.Get("/boards/:slug", a.GetBoard)

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
	api.Post("/jobs/:slug/dismiss", keyAuth, a.DismissJob)
	api.Delete("/jobs/:slug/dismiss", keyAuth, a.UndismissJob)
	api.Patch("/jobs/:slug/track", keyAuth, a.TrackJob)
	api.Delete("/jobs/:slug/stage", keyAuth, a.ClearStage)
	api.Delete("/jobs/:slug/track", keyAuth, a.Untrack)

	// Moderator-authored jobs: create a hand-curated vacancy and edit it. Authenticated
	// by cookie or API key (the CLI uses a key), then gated on the moderator role. The
	// public job reads above stay unauthenticated; a non-moderator gets 403.
	requireModerator := auth.RequireRole(a.queries, "moderator")
	api.Post("/jobs", keyAuth, requireModerator, a.CreateJob)
	api.Patch("/jobs/:slug", keyAuth, requireModerator, a.UpdateJob)

	// Public job submissions: any authenticated user submits a vacancy for review
	// (cookie or API key) and reads their own queue; the review actions (the pending
	// queue, approve, reject) are moderator-gated. Approval mints a live job — the same
	// path CreateJob uses — so an approved submission is indistinguishable from a
	// hand-curated one.
	api.Post("/submissions", keyAuth, a.CreateSubmission)
	api.Get("/me/submissions", keyAuth, a.ListMySubmissions)
	api.Get("/submissions", keyAuth, requireModerator, a.ListPendingSubmissions)
	api.Post("/submissions/:id/approve", keyAuth, requireModerator, a.ApproveSubmission)
	api.Post("/submissions/:id/reject", keyAuth, requireModerator, a.RejectSubmission)

	// Job reports: any authenticated user flags a problem with a live vacancy (cookie or
	// API key), addressed by the job's public slug. The review actions (the pending queue,
	// resolve, dismiss) are moderator-gated; resolve may soft-close the reported job.
	api.Post("/jobs/:slug/reports", keyAuth, a.CreateReport)
	api.Get("/reports", keyAuth, requireModerator, a.ListPendingReports)
	api.Post("/reports/:id/resolve", keyAuth, requireModerator, a.ResolveReport)
	api.Post("/reports/:id/dismiss", keyAuth, requireModerator, a.DismissReport)

	// User-scoped reads live under /me (consistent with /auth/me): the my-jobs
	// listing joins the caller's interactions with the jobs they touch, and the
	// viewed-slugs set lets the SPA dim already-seen cards in the public browse
	// list without making that list authenticated.
	api.Get("/me/jobs", keyAuth, a.ListMyJobs)
	api.Get("/me/jobs/viewed", keyAuth, a.ListViewedSlugs)
	api.Get("/me/jobs/pipeline", keyAuth, a.MyPipeline)
	api.Get("/me/jobs/swipe", keyAuth, a.SwipeDeck)

	// API-key management is cookie-only (RequireAuth): a leaked key must not be
	// able to create, list, or revoke keys. The create endpoint returns the
	// plaintext token exactly once.
	api.Post("/me/api-keys", auth.RequireAuth(a.issuer), a.CreateAPIKey)
	api.Get("/me/api-keys", auth.RequireAuth(a.issuer), a.ListAPIKeys)
	api.Delete("/me/api-keys/:id", auth.RequireAuth(a.issuer), a.RevokeAPIKey)

	// Saved searches are cookie-only (RequireAuth) like API-key management: they are a
	// browser convenience (the "My filters" picker), not a scripting primitive. Each
	// operation is owner-scoped; an id that is not the caller's is a 404.
	saved := auth.RequireAuth(a.issuer)
	api.Get("/me/searches", saved, a.ListSavedSearches)
	api.Post("/me/searches", saved, a.CreateSavedSearch)
	api.Patch("/me/searches/:id", saved, a.UpdateSavedSearch)
	api.Delete("/me/searches/:id", saved, a.DeleteSavedSearch)
	// Publish/unpublish a saved search as a public board. Cookie-only (same as the rest
	// of /me/searches); the public read is GET /boards/:slug above.
	api.Post("/me/searches/:id/share", saved, a.ShareSavedSearch)
	api.Delete("/me/searches/:id/share", saved, a.UnshareSavedSearch)

	// Search profiles are cookie-only (RequireAuth) like saved searches: a browser
	// feature (the profile picker), owner-scoped (a non-owned id is a 404).
	api.Get("/me/profiles", saved, a.ListSearchProfiles)
	api.Post("/me/profiles", saved, a.CreateSearchProfile)
	api.Patch("/me/profiles/:id", saved, a.UpdateSearchProfile)
	api.Delete("/me/profiles/:id", saved, a.DeleteSearchProfile)
	// The résumé verdict is a per-profile sub-resource: GET computes the live
	// market-coverage verdict from the profile's skills against the selected role.
	// Cookie-only and owner-scoped, like the profile routes it hangs off.
	api.Get("/me/profiles/:id/verdict", saved, a.GetResumeVerdict)

	// Resume skill extraction is cookie-only (RequireAuth): it feeds the profile
	// picker (extracted skills merge into a profile). When S3 storage is configured it
	// also stores the résumé once (the single upload point); when not, it stays stateless
	// (parsed and discarded, only canonical slugs returned).
	api.Post("/me/resume/extract", saved, a.ExtractResumeSkills)

	// Résumé storage (cookie-only): store the résumé once so the verdict's coherence can
	// reuse it without a second upload. PUT stores/replaces, GET reports status (enabled +
	// present + uploaded_at), DELETE removes it. 501 from PUT/DELETE when S3 is
	// unconfigured — the SPA then falls back to per-request upload on the verdict page.
	api.Put("/me/resume", saved, a.PutResume)
	api.Get("/me/resume", saved, a.GetResume)
	api.Delete("/me/resume", saved, a.DeleteResume)

	// Filter subscriptions + Telegram linking are cookie-only (RequireAuth) like
	// saved searches: a browser convenience, owner-scoped (a non-owned id is 404).
	api.Get("/me/subscriptions", saved, a.ListSubscriptions)
	api.Post("/me/subscriptions", saved, a.CreateSubscription)
	api.Patch("/me/subscriptions/:id", saved, a.SetSubscriptionActive)
	api.Delete("/me/subscriptions/:id", saved, a.DeleteSubscription)
	api.Post("/me/telegram/link", saved, a.LinkTelegram)
	api.Get("/me/telegram", saved, a.TelegramLinkStatus)
	api.Delete("/me/telegram", saved, a.UnlinkTelegram)

	// The Telegram webhook is the only unauthenticated POST: it is guarded by the
	// shared secret token Telegram echoes in a header (see TelegramWebhook).
	api.Post("/telegram/webhook", a.TelegramWebhook)

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
