package handler

import (
	"math"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/jobfit"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/report"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/savedsearch"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/submission"
	"github.com/strelov1/freehire/internal/subscription"
	"github.com/strelov1/freehire/internal/telegramnotify"
	"github.com/strelov1/freehire/internal/tokencrypt"
	"github.com/strelov1/freehire/internal/userprofile"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	// telegramLinkTTL bounds how long a deep-link token is valid — long enough to
	// open Telegram and tap Start, short enough to limit a leaked link's window.
	telegramLinkTTL = 10 * time.Minute
	// jobfitLLMTimeout is the per-stage LLM timeout for the fit analysis: its reasoning
	// model spends tens of seconds thinking before answering, so a stage needs more than
	// the shared client's default.
	jobfitLLMTimeout = 180 * time.Second
	// resumeExtractLLMTimeout bounds the single structured-résumé extraction call. It runs
	// off the upload response path (background) so it can be generous, but still bounded so
	// a stalled gateway cannot leak a goroutine indefinitely.
	resumeExtractLLMTimeout = 120 * time.Second
)

// API holds dependencies shared across HTTP handlers.
type API struct {
	pool         *pgxpool.Pool
	queries      *db.Queries
	issuer       *auth.Issuer
	cookieSecure bool
	// cookieDomain scopes the session cookie so freehire.dev and apply.freehire.dev
	// share it (empty = host-only, for dev).
	cookieDomain string
	// oauth maps enabled OAuth provider names to their implementations; empty
	// when no provider is configured (the routes then 404 / list empty).
	oauth map[string]oauth.Provider
	// frontendOrigin is where OAuth callbacks send the browser back to.
	frontendOrigin string
	// gmailConnector + gmailCipher back the "Connect Gmail" inbox. Both nil when
	// the feature is unconfigured (Google creds / token key absent) — the connect
	// routes are then not registered and the inbox reads empty.
	gmailConnector *gmailsync.Connector
	gmailCipher    *tokencrypt.Cipher
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
	// userProfile owns the single-per-user profile use cases (fetch/save/clear a
	// specialization + skills set); the handlers translate wire ↔ domain and delegate
	// to it.
	userProfile *userprofile.Service
	// resume owns the per-user stored-résumé use cases (store/status/delete + derive
	// text for the verdict). Its blob store is nil when S3 is unconfigured; Enabled()
	// then reports false and callers degrade to per-request résumé upload.
	resume *resume.Store
	// structuredExtractor derives the read-only structured résumé from an uploaded CV
	// (best-effort, background). Its client is nil when the LLM is unconfigured; extraction
	// then no-ops and the profile simply shows no structured section.
	structuredExtractor *resumeextract.Extractor
	// atsAnalyzer runs the optional LLM qualitative review for the CV ATS report.
	// Its client is nil when the LLM is unconfigured; Analyze then degrades to a no-op.
	atsAnalyzer *atscheck.Analyzer
	// atsCache reads/writes the per-user cached CV ATS review (backed by *db.Queries).
	atsCache atsReviewStore
	// jobFit runs the on-demand three-stage LLM fit analysis for one (candidate, job).
	// Its client is nil when the LLM is unconfigured; Analyze then degrades to a no-op.
	jobFit *jobfit.Analyzer
	// jobFitCache reads/writes the per-(user, job) cached fit analysis (backed by *db.Queries).
	jobFitCache jobFitStore
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
	CookieDomain   string
	OAuthProviders map[string]oauth.Provider
	Search         *search.Client
	// Blob backs résumé storage (internal/blobstore). Nil disables storage: résumé
	// upload only extracts skills in-request (no regression).
	Blob blobstore.Store
	// LLM backs the optional CV ATS qualitative review. Nil disables the AI layer:
	// the ATS score stays deterministic (the report just omits content-quality).
	LLM *llm.Client
	// Telegram bot for notification linking/delivery confirmations. Optional: an
	// empty TelegramBotToken disables the feature (linking endpoints report off,
	// webhook inert). TelegramBotUsername builds the deep link; TelegramWebhookSecret
	// guards the inbound webhook.
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string
	// GmailConnector + GmailCipher enable the Connect-Gmail inbox. Both nil = the
	// feature is off (connect routes unregistered, inbox empty).
	GmailConnector *gmailsync.Connector
	GmailCipher    *tokencrypt.Cipher
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
		cookieDomain:   cfg.CookieDomain,
		oauth:          cfg.OAuthProviders,
		frontendOrigin: cfg.FrontendOrigin,
		gmailConnector: cfg.GmailConnector,
		gmailCipher:    cfg.GmailCipher,
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
	a.userProfile = userprofile.New(userprofile.NewQueriesRepository(queries))
	// Résumé storage is nil-safe: a nil Blob (S3 unconfigured) yields a disabled service
	// whose Enabled() is false, so the upload/verdict paths degrade to in-request parsing.
	a.resume = resume.New(cfg.Blob, resume.NewQueriesRepository(queries))
	// Nil-safe: NewAnalyzer(nil) is a no-op analyzer, so the ATS report works whether
	// or not the LLM is configured.
	a.atsAnalyzer = atscheck.NewAnalyzer(cfg.LLM)
	a.atsCache = queries
	// The fit analysis shares the same LLM client but with a longer per-call timeout:
	// its reasoning model is slow (tens of seconds per stage), so the default would time
	// out mid-stage. Nil-safe (a nil client stays nil → Analyze is a no-op).
	a.jobFit = jobfit.NewAnalyzer(cfg.LLM.WithTimeout(jobfitLLMTimeout))
	a.structuredExtractor = resumeextract.NewExtractor(cfg.LLM.WithTimeout(resumeExtractLLMTimeout))
	a.jobFitCache = queries
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
	api.Get("/jobs/:slug/copies", a.JobCopies)
	api.Get("/companies", a.ListCompanies)
	api.Get("/companies/sitemap", a.CompanySitemap)
	api.Get("/companies/sitemap/boundaries", a.CompanySitemapBoundaries)
	api.Get("/companies/:slug", a.GetCompany)

	// Public read of a shared saved-search "board" by its slug — unauthenticated, like
	// the job/company reads above. Owner identity is never exposed (see boardResponse).
	api.Get("/boards/:slug", a.GetBoard)

	// Public catalogue-activity time series (added vs. removed vacancies per period),
	// unauthenticated like the other public reads. Served from the job_daily_stats
	// rollup (cmd/rollup-stats); the /trends SPA page renders it as a bar chart.
	api.Get("/stats/jobs-activity", a.JobsActivity)

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
	// Read-only per-job skill match against the caller's profile (no writes).
	api.Get("/jobs/:slug/match", keyAuth, a.JobMatch)
	api.Get("/jobs/:slug/fit", keyAuth, a.GetJobFit)
	api.Post("/jobs/:slug/fit", keyAuth, a.PostJobFit)
	api.Get("/jobs/:slug/fit/stream", keyAuth, a.StreamJobFit)

	// Stateless market-coverage: score a caller-supplied skill list (request body)
	// against the facet-filtered market. Cookie or API key — the CLI drives it with
	// a key. No user data is stored; it is the stateless sibling of the CV verdict.
	api.Post("/market/coverage", keyAuth, a.MarketCoverage)

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

	// User-scoped reads live under /me (consistent with /auth/me): the tracking
	// listing joins the caller's interactions with the jobs they touch, viewed-slugs
	// lets the SPA dim already-seen cards without authenticating the public browse
	// list, and analyses lists the jobs the caller has run the AI fit analysis on.
	api.Get("/me/tracking", keyAuth, a.ListTrackedJobs)
	api.Get("/me/tracking/viewed", keyAuth, a.ListViewedSlugs)
	api.Get("/me/tracking/pipeline", keyAuth, a.TrackingPipeline)
	api.Get("/me/tracking/swipe", keyAuth, a.SwipeDeck)
	api.Get("/me/tracking/analyses", keyAuth, a.ListMyAnalyses)
	api.Get("/me/recommendations", keyAuth, a.Recommendations)

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

	// The user profile is a cookie-only (RequireAuth) singleton — one per user, keyed
	// by the session, no id in the path. GET returns the profile or null; PUT upserts
	// (create-or-replace); DELETE clears it (idempotent).
	api.Get("/me/profile", saved, a.GetProfile)
	api.Put("/me/profile", saved, a.PutProfile)
	api.Delete("/me/profile", saved, a.DeleteProfile)

	// Connect-Gmail inbox. The read + disconnect routes are always available
	// (empty/no-op when not connected); the OAuth connect routes only when the
	// feature is configured (Google creds + token key present). Cookie-only, as
	// they are browser OAuth flows.
	api.Get("/me/gmail", saved, a.GmailStatus)
	api.Delete("/me/gmail", saved, a.GmailDisconnect)
	api.Get("/me/inbox", saved, a.GetInbox)
	api.Get("/me/inbox/group", saved, a.GetInboxGroup)
	api.Get("/me/emails/:id", saved, a.GetEmail)
	if a.gmailReady() {
		api.Get("/me/gmail/connect", saved, a.GmailConnect)
		api.Get("/me/gmail/callback", saved, a.GmailCallback)
		api.Post("/me/gmail/sync", saved, a.SyncGmail)
	}
	// The résumé verdict is a profile sub-resource: GET computes the live
	// market-coverage verdict from the profile's skills against the selected role.
	// Cookie-only and session-scoped, like the profile it hangs off (no profile → 404).
	api.Get("/me/profile/verdict", saved, a.GetResumeVerdict)
	// The CV ATS-readiness report is a sibling profile sub-resource: GET scores the
	// caller's stored CV (structure + role keyword-match); POST runs the optional LLM
	// qualitative review over it and caches it. Cookie-only, session-scoped.
	api.Get("/me/profile/ats-report", saved, a.GetATSReport)
	api.Post("/me/profile/ats-report", saved, a.PostATSReport)

	// Resume skill extraction is cookie-only (RequireAuth): it feeds the profile edit
	// modal (extracted skills merge into the profile). When S3 storage is configured it
	// also stores the résumé once (the single upload point); when not, it stays stateless
	// (parsed and discarded, only canonical slugs returned).
	api.Post("/me/resume/extract", saved, a.ExtractResumeProfile)

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
