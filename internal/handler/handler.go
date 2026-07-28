package handler

import (
	"context"
	"log"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/accountdelete"
	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/boardresolve"
	"github.com/strelov1/freehire/internal/browsertools"
	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/linkimport"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/pii"
	"github.com/strelov1/freehire/internal/referral"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/tokencrypt"
	"github.com/strelov1/freehire/internal/userprofile"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	// trackingMaxLimit is the higher ceiling for the caller's own tracked-jobs
	// listing: the Kanban board is unpaginated (it fetches everything at once), so
	// the shared 100 cap would silently hide a heavy user's older applications.
	trackingMaxLimit = 500
	// telegramLinkTTL bounds how long a deep-link token is valid — long enough to
	// open Telegram and tap Start, short enough to limit a leaked link's window.
	telegramLinkTTL = 10 * time.Minute
	// matchAnalysisLLMTimeout is the per-stage LLM timeout for the fit analysis: its reasoning
	// model spends tens of seconds thinking before answering, so a stage needs more than
	// the shared client's default.
	matchAnalysisLLMTimeout = 180 * time.Second
	// resumeExtractLLMTimeout bounds the single structured-résumé extraction call. It runs
	// off the upload response path (background) so it can be generous, but still bounded so
	// a stalled gateway cannot leak a goroutine indefinitely.
	resumeExtractLLMTimeout = 120 * time.Second
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
	// autofillPlanner is the model the agent-driven autofill maps a profile onto a
	// form with. Nil when the LLM is unconfigured: the run then reports the feature
	// is off, and the extension's deterministic autofill still works.
	autofillPlanner *llm.Client
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
	// cvKey is keyAuth widened to admit the narrow `cv` key. Only the CV surface (and
	// the caller's own identity read) mounts it; every other key-accepting route stays
	// on key, which is full-scope-only — so a new endpoint is out of a leaked agent
	// credential's reach unless it opts in.
	cvKey     fiber.Handler
	cookie    fiber.Handler
	moderator fiber.Handler
	// outboundFetch throttles every endpoint that makes the server fetch a
	// caller-supplied URL, so one user's budget is spent across them rather than
	// granted once per route.
	outboundFetch fiber.Handler
}

// pageParams reads and clamps the shared limit/offset pagination query params.
// The offset is clamped into int32 range because the column binds as a Postgres
// int4, and an unbounded query value would otherwise overflow on the conversion.
func pageParams(c *fiber.Ctx) (limit, offset int) {
	return pageParamsMax(c, maxLimit)
}

// pageParamsMax is pageParams with a caller-supplied limit ceiling, for endpoints
// whose page is bounded differently than the shared list cap (e.g. the tracking
// board, which is unpaginated and needs the whole set).
func pageParamsMax(c *fiber.Ctx, ceiling int) (limit, offset int) {
	limit = min(max(c.QueryInt("limit", defaultLimit), 1), ceiling)
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
	CookieDomains  []string
	OAuthRegistry  *oauth.Registry
	Search         *search.Client
	// Blob backs résumé storage (internal/blobstore). Nil disables storage: résumé
	// upload only extracts skills in-request (no regression).
	Blob blobstore.Store
	// TypstBin is the resolved path to the typst binary for CV PDF rendering. Empty
	// disables rendering: the /me/cvs/:id/pdf endpoint returns 501, the rest works.
	TypstBin string
	// LLM backs the optional CV ATS qualitative review. Nil disables the AI layer:
	// the ATS score stays deterministic (the report just omits content-quality).
	LLM *llm.Client
	// AssistantLLM backs the in-app agent. It is a separate client because the agent
	// runs on its own model (ASSISTANT_MODEL) — chosen for tool calling and context
	// size rather than for cheap JSON extraction. Nil disables new turns.
	AssistantLLM *llm.Client
	// AssistantMaxSteps bounds the tool-calling rounds of one turn; zero uses the
	// assistant package's default.
	AssistantMaxSteps int
	// PIIDetector de-identifies CV text before it reaches the LLM (fit analysis and
	// structured extraction). Nil disables those CV→LLM paths (fail-closed): they degrade
	// to no analysis rather than send PII to the model.
	PIIDetector pii.Detector
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
	// MailboxDomain enables the hosted-mailbox option: the receiving domain user
	// addresses live on (<handle>@MailboxDomain). Empty = the feature is off.
	MailboxDomain string
	// Credits carries the AI-points economics (monthly grant + per-action costs) that
	// gate the match and tailor features.
	Credits credits.Config
	// AWSRegion + NotifyEmailFrom enable the SES email channel for referral pings, reusing
	// the notify worker's config. Both must be set; either empty leaves referral pings
	// Telegram-only (email disabled). NotifyEmailFrom is the verified SES sender address.
	AWSRegion       string
	NotifyEmailFrom string
	// ExtensionRedirectAllowlist bounds the browser-extension connect flow: only
	// https://<id>.chromiumapp.org redirects whose <id> is listed may receive a
	// minted token. Empty leaves the connect endpoint refusing every redirect.
	ExtensionRedirectAllowlist []string
	// ServedHosts are the exact hostnames honoured as an OAuth redirect origin.
	// Empty defaults to the frontend origin's own host.
	ServedHosts []string
}

// Register wires all routes onto the application from cfg. Auth is same-origin
// only: the SPA reaches the API under one origin (a dev Vite proxy mirrors the
// production reverse proxy), so the cookie rides along with no CORS. The CORS
// allowlist is not credentialed — it only permits non-credentialed cross-origin
// reads of the public endpoints.
func Register(app *fiber.App, cfg Config) {
	queries := db.New(cfg.Pool)
	a := &API{
		pool:         cfg.Pool,
		queries:      queries,
		issuer:       auth.NewIssuer(cfg.JWTSecret, cfg.JWTTTL),
		browserTools: browsertools.New(),
	}
	authH := newAuthHandlers(queries, cfg.Pool, a.issuer, cfg.CookieSecure, cfg.CookieDomains, cfg.OAuthRegistry, cfg.FrontendOrigin, cfg.ExtensionRedirectAllowlist,
		servedHostsOrDefault(cfg.ServedHosts, cfg.FrontendOrigin))
	if needsExplicitServedHosts(cfg.ServedHosts, cfg.CookieDomains) {
		log.Printf("oauth: COOKIE_DOMAIN is set (%v) but SERVED_HOSTS is not — "+
			"the redirect origin falls back to %s for every other host, which breaks "+
			"sign-in on them (state mismatch). List every served host in SERVED_HOSTS.",
			cfg.CookieDomains, cfg.FrontendOrigin)
	}
	jobsH := newJobsHandlers(queries, moderation.New(moderation.NewQueriesRepository(queries, cfg.Pool, enrich.Version)))
	sitemapH := newSitemapHandlers(queries)
	statsH := newStatsHandlers(queries)
	votesH := newVoteHandlers(queries, cfg.Pool)
	communityH := newCommunityHandlers(queries)
	submissionsH := newSubmissionHandlers(queries, jobsH.moderation)
	// Contributions detect the ATS board from the URL alone (network-free, board.go), with a
	// network fallback (boardresolve) that fetches a company careers page and detects an
	// embedded ATS — so vanity-domain links (company.com/careers?gh_jid=…) resolve too.
	contributionSvc := contribution.New(contribution.NewQueriesRepository(queries), boardresolve.New())
	reportsH := newReportHandlers(queries)
	savedSearchH := newSavedSearchHandlers(queries)
	subscriptionH := newSubscriptionHandlers(queries)
	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	// Résumé storage is nil-safe: a nil Blob (S3 unconfigured) yields a disabled service
	// whose Enabled() is false, so the upload/verdict paths degrade to in-request parsing.
	resumeStore := resume.New(cfg.Blob, resume.NewQueriesRepository(queries))
	// The profile read serves the structured résumé beside the profile, so it needs the
	// résumé store — hence constructed after it.
	profileH := newProfileHandlers(profileSvc, resumeStore)
	// Nil-safe: NewAnalyzer(nil) is a no-op analyzer, so the ATS report works whether
	// or not the LLM is configured.
	atsAnalyzer := atscheck.NewAnalyzer(cfg.LLM)
	// The fit analysis shares the same LLM client but with a longer per-call timeout:
	// its reasoning model is slow (tens of seconds per stage), so the default would time
	// out mid-stage. Nil-safe (a nil client stays nil → Analyze is a no-op).
	matchAnalyzer := matchanalysis.NewAnalyzer(cfg.LLM.WithTimeout(matchAnalysisLLMTimeout))
	structuredExtractor := resumeextract.NewExtractor(cfg.LLM.WithTimeout(resumeExtractLLMTimeout), cfg.PIIDetector)
	// The autofill planner is one cheap structured call per run; the shared client's
	// default timeout is right for it.
	a.autofillPlanner = cfg.LLM
	creditsStore := credits.NewStore(queries, cfg.Pool, cfg.Credits)
	// Imports fetch a user-supplied page, so they dial through the same SSRF-guarded
	// client the crawlers use (sources.NewClient). cfg.Search may be nil (no engine
	// configured), which only skips the index push.
	importer := linkimport.New(cfg.Pool, queries, cfg.Search, sources.NewClient())
	contributionsH := newContributionHandlers(contributionSvc, creditsStore, queries, importer)
	creditsH := newCreditsHandlers(creditsStore, queries)
	matchH := newMatchHandlers(queries, profileSvc, resumeStore, matchAnalyzer, creditsStore)
	cvH := newCVHandlers(queries, cfg.TypstBin, resumeStore, creditsStore, matchH)
	telegramH := newTelegramHandlers(queries, cfg.JWTSecret, cfg.TelegramBotToken, cfg.TelegramBotUsername, cfg.TelegramWebhookSecret, cfg.FrontendOrigin, contributionSvc, creditsStore)
	inboxH := newInboxHandlers(queries, cfg.GmailConnector, cfg.GmailCipher, cfg.FrontendOrigin, cfg.CookieSecure, cfg.MailboxDomain)
	// Account deletion reaches past the FK cascade: cfg.Blob is nil when storage is
	// unconfigured and the revoker is nil when Gmail is — either way there is nothing
	// to erase there, which must not stop a member from leaving.
	authH.withAccountDeletion(accountdelete.New(accountdelete.NewQueriesRepository(queries), cfg.Blob, inboxH.revokeGmailGrant), queries)
	// Assign only when configured: a nil *search.Client wrapped in the searcher
	// interface would be a non-nil interface and defeat the nil check.
	var jobSearch searcher
	var facets facetCounter
	var companySearch companySearcher
	if cfg.Search != nil {
		jobSearch = cfg.Search
		facets = cfg.Search
		companySearch = cfg.Search
	}
	searchH := newSearchHandlers(jobSearch, facets, queries)
	companiesH := newCompaniesHandlers(queries, companySearch)
	trackingH := newTrackingHandlers(queries, cfg.Pool, jobSearch)
	resumeH := newResumeHandlers(resumeStore, structuredExtractor, jobSearch, facets, profileSvc, atsAnalyzer, queries)
	// The in-app agent is a facade over the feature handlers above: its tools call
	// the same services their endpoints do, so a tool result and the API can never
	// disagree. The tailoring bootstrap mints its conversations through the same
	// store, which is why the CV handlers get it back.
	assistantH := newAssistantHandlers(queries, cfg.AssistantLLM, cfg.AssistantMaxSteps, searchH, resumeH, trackingH, cvH, profileH)
	cvH.withAssistantSessions(assistantH.store)

	// Referral notifications reuse the SES email transport (email is always present) and
	// the Telegram bot when linked. Each channel is wrapped only when configured so a nil
	// concrete pointer never hides behind a non-nil interface (see the search note above);
	// a referrer with no reachable channel still sees the request in-cabinet.
	var referralEmail referral.EmailSender
	if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		if ec, err := emailnotify.NewClient(context.Background(), cfg.AWSRegion); err != nil {
			log.Printf("referral: email pinger disabled: %v", err)
		} else {
			referralEmail = ec
			// The same SES client carries the account mails (verification and password
			// reset). Without it the accounts service keeps registering and
			// authenticating; only the code-backed flows report 503.
			authH.accounts.WithCodes(
				accounts.NewQueriesCodeStore(queries),
				emailnotify.NewAuthMailer(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin),
			)
		}
	} else {
		log.Print("accounts: AWS_REGION/NOTIFY_EMAIL_FROM unset — email verification and password reset are unavailable")
	}
	var referralTelegram referral.TelegramSender
	if telegramH.telegramBot != nil {
		referralTelegram = telegramH.telegramBot
	}
	referralPinger := referral.NewChannelPinger(referralEmail, cfg.NotifyEmailFrom, referralTelegram)
	referralCabinetURL := strings.TrimRight(cfg.FrontendOrigin, "/") + "/my/referrals?tab=incoming"
	referralSvc := referral.New(referral.NewQueriesRepository(queries), referralPinger,
		referral.Config{CabinetURL: referralCabinetURL})
	referralsH := newReferralHandlers(referralSvc, cfg.Blob, cvH.cvRenderer, cvH.cvStore)

	// Allow the canonical frontend origin plus every served domain's https apex,
	// so a cross-origin (non-credentialed) read works from either domain during a
	// migration. Same-origin app traffic never triggers CORS; this only covers the
	// dev Vite origin and genuine cross-origin callers.
	allowOrigins := cfg.FrontendOrigin
	for _, d := range cfg.CookieDomains {
		allowOrigins += ",https://" + d
	}
	app.Use(cors.New(cors.Config{AllowOrigins: allowOrigins}))

	app.Get("/health", a.Health)

	api := app.Group("/api/v1")
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
	// cookieAuth is the single cookie-only gate (RequireAuth) for the
	// browser-convenience surfaces below — key management, saved searches, the CV
	// builder, the inbox, subscriptions — where a leaked API key must not act.
	cookieAuth := auth.RequireAuth(a.issuer, a.queries)
	requireModerator := auth.RequireRole(a.queries, "moderator")
	mw := middleware{
		optional:      optionalAuth,
		key:           keyAuth,
		cvKey:         cvKeyAuth,
		cookie:        cookieAuth,
		moderator:     requireModerator,
		outboundFetch: contributionLimiter(),
	}

	// Job search surfaces first: their literal /jobs/* routes must precede the
	// /jobs/:slug param route so they are not read as slugs (see searchHandlers).
	searchH.register(api, mw)
	sitemapH.register(api)
	jobsH.register(api, mw)
	companiesH.register(api, mw)

	// Saved searches + the public shared-board read (see savedSearchHandlers).
	savedSearchH.register(api, mw)

	// Public catalogue-activity, member-growth, engagement, facet-snapshot, and
	// ingest-status reads (see statsHandlers).
	statsH.register(api)

	// Per-user job interactions, tracking reads, and reminder controls
	// (see trackingHandlers). The interaction writes precede the vote routes,
	// mirroring the previous registration order.
	trackingH.register(api, mw)
	votesH.register(api, mw)
	// Per-job skill match + the on-demand LLM fit analysis (see matchHandlers).
	matchH.register(api, mw)
	// Canonical autofill fields (name/email/phone/location/links) for the browser
	// extension to write into application forms. keyAuth (Bearer).
	api.Get("/me/autofill-profile", keyAuth, a.AutofillProfile)
	// The browser-tool wire: a harness on one end, the caller's browser extension
	// on the other, exchanging raw tool frames. Both ends authenticate with the
	// session JWT (Bearer for a server-side harness, the subprotocol for the
	// extension, which can set no headers); the relay routes strictly within one
	// user's channel. See internal/browsertools.
	api.Get("/tools/ws", auth.RequireAuthWS(a.issuer, a.queries, apiKeys{a.queries}), a.BrowserToolsWS())
	// Agent-driven autofill: the caller's own browser is driven over that wire.
	// keyAuth (Bearer) so the extension can trigger it.
	api.Post("/me/autofill/run", keyAuth, a.RunAgentAutofill)

	// Public job submissions + review queue (see submissionHandlers).
	submissionsH.register(api, mw)

	// Link contributions (see contributionHandlers).
	contributionsH.register(api, mw)

	// Employee referrals (see referralHandlers).
	referralsH.register(api, mw)

	// Job reports + review queue (see reportHandlers).
	reportsH.register(api, mw)

	// Community discussion threads (see communityHandlers).
	communityH.register(api, mw)

	creditsH.register(api, mw)

	// API-key management and the auth surface (see authHandlers).
	authH.register(api, mw)

	// The per-user profile singleton (see profileHandlers).
	profileH.register(api, mw)

	// CV builder + AI tailoring (see cvHandlers).
	cvH.register(api, mw)

	// Mail inbox (Gmail connect + hosted mailbox) and email ↔ application linking
	// (see inboxHandlers). Registered after the static /me/tracking/* routes so
	// /me/tracking/:slug does not shadow them.
	inboxH.register(api, mw)
	// Résumé/CV surfaces: verdict, ATS report, extraction, storage, recommendations
	// (see resumeHandlers).
	resumeH.register(api, mw)
	assistantH.register(api, mw)

	// Filter subscriptions (see subscriptionHandlers).
	subscriptionH.register(api, mw)

	// Telegram linking + the inbound bot webhook (see telegramHandlers).
	telegramH.register(api, mw)

}
