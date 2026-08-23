package handler

import (
	"cmp"
	"context"
	"log"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/ai/credits"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/speech"
	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/api/realtime"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/mailrecall"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/pii"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/engage/companyfeedback"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/referral"
	"github.com/strelov1/freehire/internal/engage/report"
	"github.com/strelov1/freehire/internal/identity/accountdelete"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/ingest/boardresolve"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/jdresolve"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/privatejob"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
	"github.com/strelov1/freehire/internal/search/search"
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
	//
	// It is a budget for ONE attempt, not for the stage: matchanalysis retries a timed-out
	// stage once, so the worst case per stage is twice this. 90s sits well past the observed
	// spread (healthy stages answer in 3–25s) while leaving room for that retry — a call
	// still running at 90s is hung rather than slow, and in production the retry after such
	// a call answered in under eight seconds.
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
	cvKey  fiber.Handler
	cookie fiber.Handler
	// optionalCookie attaches a cookie session when there is one but never rejects.
	// It exists for provider callbacks, which are browser navigations: a 401 there
	// renders JSON into the address bar instead of sending the user back to the app.
	optionalCookie fiber.Handler
	moderator      fiber.Handler
	// outboundFetch throttles every endpoint that makes the server fetch a
	// caller-supplied URL, so one user's budget is spent across them rather than
	// granted once per route.
	outboundFetch fiber.Handler
	// throttler backs every other per-route rate limiter built inside a feature
	// handler's own register method (mail recall, photo upload, JD resolve, match
	// analysis) — the same shared instance outboundFetch and the auth routes use.
	throttler ratelimit.Throttler
}

// pageParams reads and clamps the shared limit/offset pagination query params.
func pageParams(c *fiber.Ctx) (limit, offset int) {
	return pageParamsBounded(c, defaultLimit, maxLimit)
}

// pageParamsBounded is pageParams with caller-supplied bounds, for endpoints whose page is
// sized differently from the shared list cap (the tracking board, which is unpaginated and
// needs the whole set; the role-cluster copies list, which pages a handful of city openings).
//
// It is the ONLY place the offset query param is read, and a test enforces that. The clamp to
// MaxInt32 is the reason: every paginated column binds as a Postgres int4, Fiber's QueryInt is
// a plain strconv.Atoi that happily accepts a 64-bit value, and the int32 conversion then wraps
// it NEGATIVE — which Postgres rejects, turning ?offset=3000000000 into a 500 rather than an
// empty page. Naming that in a comment did not stop a second call site re-implementing the
// parse without it.
func pageParamsBounded(c *fiber.Ctx, fallback, ceiling int) (limit, offset int) {
	limit = min(max(c.QueryInt("limit", fallback), 1), ceiling)
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

// listResponseWithHidden is listResponse plus the count a default filter suppressed.
//
// Separate from listResponse rather than a widened one: every other listing hides nothing,
// and a `hidden: 0` on all of them would be a field readers have to learn to ignore.
func listResponseWithHidden(c *fiber.Ctx, data any, total, hidden int64, limit, offset int) error {
	return c.JSON(fiber.Map{
		"data": data,
		"meta": fiber.Map{
			"total":  total,
			"limit":  limit,
			"offset": offset,
			"hidden": hidden,
		},
	})
}

// listResponseWithIgnored is listResponse plus the query params the filter did
// not read, omitted entirely when there are none.
//
// Omitted rather than sent empty: a caller only needs the key when something
// actually went wrong, and an always-present empty array is a field every reader
// learns to skip — at which point the one response that does carry a warning
// gets skipped too.
func listResponseWithIgnored(c *fiber.Ctx, data any, total int64, limit, offset int, ignored []search.UnknownParam) error {
	meta := fiber.Map{
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}
	if len(ignored) > 0 {
		meta["ignored_params"] = ignored
	}
	return c.JSON(fiber.Map{"data": data, "meta": meta})
}

// dataResponseWithIgnored is listResponseWithIgnored for the single-item
// endpoints: they answer a bare {"data": ...} with no meta at all, so a clean
// request keeps exactly that shape and the block appears only to carry a
// warning.
func dataResponseWithIgnored(c *fiber.Ctx, data any, ignored []search.UnknownParam) error {
	if len(ignored) == 0 {
		return c.JSON(fiber.Map{"data": data})
	}
	return c.JSON(fiber.Map{"data": data, "meta": fiber.Map{"ignored_params": ignored}})
}

// Config is the dependency bundle Register wires onto the app: the DB pool, the
// required rate-limit Throttler, the single browser origin allowed cross-origin
// (FrontendOrigin), the token-issuer settings (JWTSecret/JWTTTL), the HTTPS-only
// cookie flag (CookieSecure), the enabled OAuth providers, and the optional
// search client (nil disables search).
type Config struct {
	Pool *pgxpool.Pool
	// Throttler backs every rate-limited route in the API (internal/api/ratelimit).
	// Required — there is no degraded/nil mode, unlike the optional dependencies
	// below.
	Throttler ratelimit.Throttler
	// Cache holds values that are expensive to compute and identical for every
	// caller — today the catalogue-scale snapshot (internal/ingest/catalogstats). Optional:
	// nil degrades the figures it backs to their approximations, exactly as an
	// unreachable backend does.
	Cache               cache.Cache
	FrontendOrigin      string
	JWTSecret           string
	JWTTTL              time.Duration
	CookieSecure        bool
	CookieDomains       []string
	OAuthRegistry       *oauth.Registry
	AuthV2Enabled       bool
	MobileAuthCallbacks map[string]string
	RecentAuthTTL       time.Duration
	AppleNative         *appleauth.Client
	AppleGrantKeys      *appleauth.KeyRing
	Search              *search.Client
	// Blob backs résumé storage (internal/platform/blobstore). Nil disables storage: résumé
	// upload only extracts skills in-request (no regression).
	Blob blobstore.Store
	// TypstBin is the resolved path to the typst binary for CV PDF rendering. Empty
	// disables rendering: the /me/cvs/:id/pdf endpoint returns 501, the rest works.
	TypstBin string

	// CVEditAllowBulletTruncation restores silent Sanitize truncation past the
	// per-role bullet ceiling. Default false (refuse on). Ops kill switch.
	CVEditAllowBulletTruncation bool

	// TracerLinkSalt keys the visitor hash of a traced click; empty means the CV tracing
	// toggle cannot be enabled at all.
	TracerLinkSalt string
	// LLM backs the optional CV ATS qualitative review. Nil disables the AI layer:
	// the ATS score stays deterministic (the report just omits content-quality).
	LLM *llm.Client
	// AssistantLLM backs the in-app agent. It is a separate client because the agent
	// runs on its own model (ASSISTANT_MODEL) — chosen for tool calling and context
	// size rather than for cheap JSON extraction. Nil disables new turns.
	AssistantLLM *llm.Client
	// SearchIntentLLM backs the AI filter. Separate for latency, not capability — see
	// config.SearchIntentModel.
	SearchIntentLLM *llm.Client
	// AssistantMaxSteps bounds the tool-calling rounds of one turn; zero uses the
	// assistant package's default.
	AssistantMaxSteps int
	// AssistantMaxPrompt bounds the rune length of one user message. Zero falls
	// back to the handler default (8000).
	AssistantMaxPrompt int
	// LLMKeys mints the per-user gateway credential each account's model calls are
	// spent under. Nil is the deployment that does not attribute spend: every call goes
	// out on the service credential exactly as it did before, and no behaviour changes.
	LLMKeys *llmkey.Client
	// Speech transcribes dictated audio for the composer. Nil is the deployment with
	// no speech gateway: the endpoint answers 501 and the SPA offers no microphone.
	Speech *speech.Client
	// Realtime mints voice mode's short-lived OpenAI Realtime API credentials. Nil is
	// the deployment with no Realtime gateway configured: the mint endpoint answers
	// 501 and the interview session view offers no voice mode.
	Realtime *realtime.Client
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
	// Discord bot for slash-command board contributions, mirroring the Telegram bot's
	// role. Optional: empty DiscordBotToken disables the feature — the linking
	// endpoints and interaction webhook are inert. DiscordApplicationID and
	// DiscordPublicKey are the app's identity and the key used to verify inbound
	// interaction signatures. DiscordGuildID scopes slash-command registration to a
	// single guild.
	DiscordBotToken      string
	DiscordApplicationID string
	DiscordPublicKey     string
	DiscordGuildID       string
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
	authH := newAuthHandlers(queries, cfg.Pool, cfg.Throttler, a.issuer, cfg.CookieSecure, cfg.CookieDomains, cfg.OAuthRegistry, cfg.FrontendOrigin, cfg.ExtensionRedirectAllowlist,
		servedHostsOrDefault(cfg.ServedHosts, cfg.FrontendOrigin))
	authH.authV2Enabled = cfg.AuthV2Enabled
	authH.mobileCallbacks = cfg.MobileAuthCallbacks
	authH.mobileAuth = mobileauth.NewStore(cfg.Pool)
	authH.recentAuth = recentauth.NewStore(cfg.Pool, cfg.RecentAuthTTL)
	authH.appleNative = cfg.AppleNative
	authH.appleGrantKeys = cfg.AppleGrantKeys
	if needsExplicitServedHosts(cfg.ServedHosts, cfg.CookieDomains) {
		log.Printf("oauth: COOKIE_DOMAIN is set (%v) but SERVED_HOSTS is not — "+
			"the redirect origin falls back to %s for every other host, which breaks "+
			"sign-in on them (state mismatch). List every served host in SERVED_HOSTS.",
			cfg.CookieDomains, cfg.FrontendOrigin)
	}
	// Moderation is shared by the jobs surface and the submission queue, so it is built
	// here rather than inside whichever of the two is constructed first.
	moderationSvc := moderation.New(moderation.NewQueriesRepository(queries, cfg.Pool, enrich.Version))
	// One SSRF-guarded client for everything that fetches a user-supplied page: the imports
	// below dial through it, it backs the ingest registry board coverage reads a vacancy
	// through (so an import and a crawl of the same board share transport and rate limits),
	// and the posting-URL resolver asks an ATS which posting an apply link is through it too.
	ingestClient := sources.NewClient()
	// Turning a link into the posting it is: offline for nearly every ATS, one platform call
	// for the shapes that hide the posting behind a second id (see sources.PostingURLResolver).
	// Shared by /jobs/find and the link intake, which must agree on what a page is.
	postingURLs := sources.NewPostingURLResolver(ingestClient)
	jobsH := newJobsHandlers(queries, moderationSvc, postingURLs, cfg.Cache)
	statsH := newStatsHandlers(queries, cfg.Cache)
	ogH := newOGHandlers(queries, cfg.Cache)
	votesH := newVoteHandlers(queries, cfg.Pool)
	communityH := newCommunityHandlers(queries)
	// Feedback reuses communityH's persona minting (via the communityPersonas
	// adapter) so a user's pseudonym stays the same one discussion threads show,
	// which is why it is constructed after communityH rather than alongside it.
	companyFeedbackH := newCompanyFeedbackHandlers(companyfeedback.New(queries, cfg.Pool, communityPersonas{svc: communityH.community}, companyfeedback.Config{}))
	// Contributions detect the ATS board from the URL alone (network-free, board.go), with a
	// network fallback (boardresolve) that fetches a company careers page and detects an
	// embedded ATS — so vanity-domain links (company.com/careers?gh_jid=…) resolve too.
	contributionSvc := contribution.New(contribution.NewQueriesRepository(queries), boardresolve.New())
	reportsH := newReportHandlers(queries)
	ghostReportsH := newGhostReportHandlers(queries)
	savedSearchH := newSavedSearchHandlers(queries)
	subscriptionH := newSubscriptionHandlers(queries)
	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	// The candidate's own screening answers (visa, salary, notice period, relocation, …) —
	// a distinct singleton from profileSvc above (search/targeting preferences, a
	// different lifecycle; see internal/ingest/screeninganswers/AGENTS.md).
	screeningAnswersSvc := screeninganswers.New(screeninganswers.NewQueriesRepository(queries))
	screeningAnswersH := newScreeningAnswersHandlers(screeningAnswersSvc)
	// Résumé storage is nil-safe: a nil Blob (S3 unconfigured) yields a disabled service
	// whose Enabled() is false, so the upload/verdict paths degrade to in-request parsing.
	resumeStore := resume.New(cfg.Blob, resume.NewQueriesRepository(queries))
	// The headshot rides the same bucket and the same nil-degradation: no Blob means the
	// photo endpoints 501 while the photo-bearing CV templates still render a placeholder.
	photoStore := headshot.New(cfg.Blob, headshot.NewQueriesRepository(queries))
	// The profile read serves the structured résumé beside the profile, so it needs the
	// résumé store — hence constructed after it.
	profileH := newProfileHandlers(profileSvc, resumeStore, newCandidateProfiler(queries))
	// Personal skill-demand trend: the caller's own profile skills joined against the
	// weekly insights_skill_history snapshots cmd/rollup-stats writes (see
	// me_market_pulse.go). Reuses profileSvc rather than a second userprofile.Service.
	marketPulseH := newMarketPulseHandlers(profileSvc, queries)
	// The Talent Network visibility toggle is a distinct singleton on `users`, not part
	// of the user_profiles-backed profileHandlers above (see me_talent_network.go).
	talentNetworkH := newTalentNetworkHandlers(queries)
	// The public, unauthenticated counterpart to talentNetworkH above — a separate
	// handler struct (not a route on talentNetworkH) because it carries no auth
	// middleware at all (see talent_network_profile.go).
	talentNetworkProfileH := newTalentNetworkProfileHandlers(queries)
	// One bank for the whole surface. It is stateless over the shared queries, but the
	// single value is what keeps the evidence gate from being anyone's to attach later:
	// the CV editor is constructed with it below, not handed it by the assistant.
	// SetProfileSkills folds every atom this bank ever writes into the owner's search
	// profile, so a skill proven in the bank reaches job filters without a manual step.
	bank := experience.NewStore(experience.NewQueriesRepository(queries)).SetProfileSkills(profileSvc)
	experienceH := newExperienceHandlers(bank).withRequireContext(queries)
	// Nil-safe: NewAnalyzer(nil) is a no-op analyzer, so the ATS report works whether
	// or not the LLM is configured.
	atsAnalyzer := atscheck.NewAnalyzer(cfg.LLM)
	// The fit analysis shares the same LLM client but with a longer per-call timeout:
	// its reasoning model is slow (tens of seconds per stage), so the default would time
	// out mid-stage. Nil-safe (a nil client stays nil → Analyze is a no-op).
	matchAnalyzer := matchanalysis.NewAnalyzer(cfg.LLM.WithTimeout(matchAnalysisLLMTimeout))
	structuredExtractor := resumeextract.NewExtractor(cfg.LLM.WithTimeout(resumeExtractLLMTimeout), cfg.PIIDetector)
	creditsStore := credits.NewStore(queries, cfg.Pool, cfg.Credits)
	// Imports fetch a user-supplied page, so they dial through ingestClient (built above,
	// where the comment on it is). cfg.Search may be nil (no engine configured), which only
	// skips the index push.
	importer := linkimport.New(cfg.Pool, queries, cfg.Search, ingestClient, sources.All(ingestClient), boardresolve.New())
	contributionsH := newContributionHandlers(contributionSvc, creditsStore, queries, importer, postingURLs)
	// Prefill reuses the SAME importer (its Resolve half, which never writes) rather than
	// a second parsing registry — see submissionHandlers.PrefillSubmission.
	submissionsH := newSubmissionHandlers(queries, moderationSvc, importer)
	// jd-tailor-intake reuses the SAME importer as the contribution flow (shared SSRF-guarded
	// transport and rate limits — see the comment on ingestClient above) for its recognized-ATS
	// branch, and internal/job/privatejob for its generic-scrape/pasted-text branch.
	jdResolveH := newJDResolveHandlers(jdresolve.New(queries, importer, privatejob.NewWriter(queries)))
	creditsH := newCreditsHandlers(creditsStore, queries)
	matchH := newMatchHandlers(queries, profileSvc, resumeStore, matchAnalyzer, creditsStore)
	// The CV store is shared: the CV surface owns the write path, referrals render from it
	// and autofill reads the base CV's contact header out of it. AGENTS.md puts shared
	// services here rather than inside whichever feature happened to need one first.
	cvStore := cv.NewStore(cv.NewQueriesRepository(queries))
	// The conversation store is shared the same way: the assistant runs turns in it and the
	// tailoring bootstrap mints a session in it. Built here, both are handed the same one —
	// which is what lets the CV handlers take it as an argument instead of being patched
	// with it after the assistant exists.
	assistantStore := assistant.NewStore(queries)
	// The renderer is shared with referrals, and is enabled only when a typst binary was
	// resolved. Assign only a NON-NIL renderer: a typed-nil satisfies cv.Renderer and would
	// defeat the 501 gate that answers when rendering is off.
	var cvRenderer cv.Renderer
	if r := cv.NewTypstRenderer(cfg.TypstBin); r != nil {
		cvRenderer = r
	}
	// Who each model call is spent as. One resolver serves every per-user surface, so the
	// account a call is attributed to is decided in one place rather than per feature. A nil
	// gateway client leaves it inert and every call on the service credential.
	llmKeys := llmkey.NewResolver(queries, cfg.LLMKeys)
	// Same repository the tracking surface uses: tailor bootstrap places the vacancy on
	// the Kanban so a pursued role is not invisible under Activity → Saved alone.
	trackingJobs := trackingBoarder{repo: jobtracking.NewQueriesRepository(queries, cfg.Pool)}
	cvH := newCVHandlers(cfg.Pool, queries, cvStore, assistantStore, cvRenderer, cfg.TracerLinkSalt, cfg.FrontendOrigin, servedHostsOrDefault(cfg.ServedHosts, cfg.FrontendOrigin), resumeStore, photoStore, creditsStore, matchH, bankGate{bank: bank}, trackingJobs, !cfg.CVEditAllowBulletTruncation)
	telegramH := newTelegramHandlers(queries, cfg.JWTSecret, cfg.TelegramBotToken, cfg.TelegramBotUsername, cfg.TelegramWebhookSecret, cfg.FrontendOrigin, contributionsH.intake)
	discordH := newDiscordHandlers(queries, cfg.JWTSecret, cfg.DiscordBotToken, cfg.DiscordApplicationID, cfg.DiscordPublicKey, cfg.DiscordGuildID, cfg.FrontendOrigin, contributionsH.intake)
	inboxH := newInboxHandlers(queries, cfg.Pool, cfg.GmailConnector, cfg.GmailCipher, cfg.FrontendOrigin, cfg.CookieSecure, cfg.MailboxDomain)
	// The pull direction is wired only where there is a model to ask. Left nil, its endpoint
	// reports the feature off — the same way an unconfigured deployment reports every other
	// model-backed surface off, rather than failing at the first press.
	if cfg.LLM != nil {
		// The mailbox factory is present only where a Gmail client and a token cipher both
		// are — the same condition cmd/gmail-sync checks. Absent, every caller falls back to
		// stored mail, which is the path that shipped first.
		inboxH = inboxH.withRecall(
			mailrecall.New(mailrecall.NewDBStore(queries), cfg.LLM),
			llmBinding{client: cfg.LLM, keys: llmKeys},
			newGmailMailboxes(queries, cfg.GmailCipher, cfg.GmailConnector))
	}
	// Account deletion reaches past the FK cascade: cfg.Blob is nil when storage is
	// unconfigured and the revoker is nil when Gmail is — either way there is nothing
	// to erase there, which must not stop a member from leaving.
	// The gateway credential is the third external system erasure spans, after object
	// storage and Google. It is attached below, once the resolver exists.
	accountDeletion := accountdelete.New(accountdelete.NewQueriesRepository(queries), cfg.Blob, inboxH.revokeGmailGrant).
		WithAppleGrants(authH.mobileAuth.ReleaseAppleGrantsForDeletion)
	authH.withAccountDeletion(accountDeletion, queries)
	// Assign only when configured: a nil *search.Client wrapped in the searcher
	// interface would be a non-nil interface and defeat the nil check.
	var jobSearch searcher
	var facets facetCounter
	var companySearch companySearcher
	// Both sitemaps page an index, so they are wired here rather than beside the
	// Postgres-backed handlers above.
	var sitemapJobs, sitemapCompanies sitemapLister
	if cfg.Search != nil {
		jobSearch = cfg.Search
		facets = cfg.Search
		companySearch = cfg.Search
		sitemapJobs = cfg.Search
		sitemapCompanies = companySitemapIndex{c: cfg.Search}
	}
	sitemapH := newSitemapHandlers(sitemapJobs, sitemapCompanies)
	searchH := newSearchHandlers(jobSearch, facets, queries, cfg.Cache)
	// The AI filter reads the same saved profile the assistant does, so a profile-seeded
	// search and a profile-aware conversation cannot disagree about what it says.
	intentH := newIntentHandlers(llmBinding{client: cmp.Or(cfg.SearchIntentLLM, cfg.LLM), keys: llmKeys})
	companiesH := newCompaniesHandlers(queries, companySearch)
	geoH := newGeoHandlers()
	trackingH := newTrackingHandlers(queries, cfg.Pool, jobSearch)
	timelineH := newTimelineHandlers(queries)
	resumeH := newResumeHandlers(resumeStore, structuredExtractor, facets, profileSvc, atsAnalyzer, queries)
	photoH := newPhotoHandlers(photoStore)
	// Same reason as jobSearch above: a nil *speech.Client wrapped in the transcriber
	// interface would be a non-nil interface, and the handler's "no gateway here"
	// check would pass straight into a nil dereference.
	var stt transcriber
	if cfg.Speech != nil {
		stt = cfg.Speech
	}
	speechH := newSpeechHandlers(stt)
	// The in-app agent is a facade over the feature handlers above: its tools call
	// the same services their endpoints do, so a tool result and the API can never
	// disagree. The tailoring bootstrap mints its conversations through the same
	// store, which is why the CV handlers get it back. It also takes the browser-tool
	// hub, which a browsing session reads the caller's open page through.
	assistantH := newAssistantHandlers(queries,
		assistantModels{
			// Its own per-call timeout for the same reason the fit analysis has one, and
			// nil-safe the same way: a turn's rounds are slower than a one-shot extraction,
			// and here a call cut short takes the whole run with it.
			Agent: cfg.AssistantLLM.WithTimeout(assistantLLMTimeout), Keys: llmKeys,
			MaxSteps: cfg.AssistantMaxSteps, MaxPrompt: cfg.AssistantMaxPrompt,
		},
		assistantStore, searchH, resumeH, trackingH, cvH, profileH, a.browserTools, inboxH, bank, screeningAnswersSvc)
	// Same nil-interface trap as stt above: only assign when cfg.Realtime is
	// genuinely non-nil, or "no voice mode here" becomes a panic on the first mint.
	if cfg.Realtime != nil {
		assistantH.realtime = cfg.Realtime
	}
	resumeH.llm = llmBinding{client: cfg.LLM, keys: llmKeys}
	matchH.llm = llmBinding{client: cfg.LLM, keys: llmKeys}
	// The autofill planner is one cheap structured call per run, so it travels on the
	// shared client's default timeout. The contact block it plans over comes from the base
	// CV, then the structured résumé — see autofillHandlers.autofillProfile.
	autofillH := newAutofillHandlers(cvStore, resumeStore, queries, screeningAnswersSvc, a.browserTools, llmBinding{client: cfg.LLM, keys: llmKeys})
	usageH := newUsageHandlers(cfg.LLMKeys, llmKeys)
	accountDeletion.WithGatewayKeys(llmKeys.Revoke)

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
				accounts.NewQueriesCodeStore(queries, cfg.Pool),
				emailnotify.NewAuthMailer(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin),
			)
			// And it tells a reporter what a moderator decided about their report.
			// Without it the queue still decides reports — each decision simply
			// reports that nobody was notified.
			reportsH.report.WithNotifier(report.NewMailNotifier(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin))
		}
	} else {
		log.Print("accounts: AWS_REGION/NOTIFY_EMAIL_FROM unset — email verification and password reset are unavailable")
	}
	var referralTelegram referral.TelegramSender
	if telegramH.telegramBot != nil {
		referralTelegram = telegramH.telegramBot
	}
	referralPinger := referral.NewChannelPinger(referralEmail, cfg.NotifyEmailFrom, referralTelegram, cfg.FrontendOrigin)
	referralCabinetURL := strings.TrimRight(cfg.FrontendOrigin, "/") + "/my/referrals?tab=incoming"
	referralSvc := referral.New(referral.NewQueriesRepository(queries), referralPinger, cfg.Blob,
		referral.Config{CabinetURL: referralCabinetURL})
	referralsH := newReferralHandlers(referralSvc, cfg.Blob, cvRenderer, cvStore, photoStore)

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

	// The public side of CV link tracing. It sits beside /health rather than under /api/v1
	// because it lives inside a PDF a recruiter opens by hand: "freehire.me/cv/acme-x7abc" is a
	// URL a person may read off a hover tooltip, and "/api/v1/..." would not be.
	//
	// Optional cookie auth, never a key: it is what lets a candidate's own click be recognised
	// and excluded, and a leaked API key must not be able to attribute or hide one.
	//
	// Rate-limited because it is the only unauthenticated WRITE in the app: one visit is one
	// click row plus a stamp on the CV. Two things it bounds. Anyone holding a PDF can loop its
	// link and fabricate the one number the feature exists to report — a count nobody can forge
	// is the whole point. And the token is a public company slug plus five random characters, so
	// the space is small enough that an unthrottled guesser would find live links and be handed a
	// candidate's personal URL, one 302 at a time.
	tracerH := newTracerHandlers(queries, cfg.TracerLinkSalt)
	tracerLimiter := ratelimit.Middleware(cfg.Throttler, ratelimit.KeyByIP("tracer"), 60, time.Minute)
	app.Get("/cv/:token", tracerLimiter, auth.OptionalCookieAuth(a.issuer, queries), tracerH.Redirect)

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
		optional:       optionalAuth,
		key:            keyAuth,
		cvKey:          cvKeyAuth,
		cookie:         cookieAuth,
		optionalCookie: auth.OptionalCookieAuth(a.issuer, a.queries),
		moderator:      requireModerator,
		outboundFetch:  contributionLimiter(cfg.Throttler),
		throttler:      cfg.Throttler,
	}

	// Job search surfaces first: their literal /jobs/* routes must precede the
	// /jobs/:slug param route so they are not read as slugs (see searchHandlers).
	searchH.register(api, mw)
	// Beside the search it builds a filter for, though it shares none of its
	// dependencies: interpretation needs a model and the caller's profile, not the
	// index.
	intentH.register(api, mw)
	sitemapH.register(api)
	jobsH.register(api, mw)
	companiesH.register(api, mw)
	geoH.register(api, mw)

	// Saved searches + the public shared-board read (see savedSearchHandlers).
	savedSearchH.register(api, mw)

	// Public catalogue-activity, member-growth, engagement, facet-snapshot, and
	// ingest-status reads (see statsHandlers).
	statsH.register(api)
	// The /open and /about pages' OG preview cards (see ogHandlers) — reads the
	// same snapshot statsH.CatalogScale serves.
	ogH.register(api)

	// Per-user job interactions, tracking reads, and reminder controls
	// (see trackingHandlers). The interaction writes precede the vote routes,
	// mirroring the previous registration order.
	trackingH.register(api, mw)
	// The ledger's dated read, behind the tracking calendar. Its own namespace, not a
	// segment under /me/tracking — see the comment on its register.
	timelineH.register(api, mw)
	votesH.register(api, mw)
	companyFeedbackH.register(api, mw)
	// Per-job skill match + the on-demand LLM fit analysis (see matchHandlers).
	matchH.register(api, mw)
	// The contact block the extension writes into forms, plain and agent-driven (see
	// autofillHandlers).
	autofillH.register(api, mw)
	// The browser-tool wire: a harness on one end, the caller's browser extension
	// on the other, exchanging raw tool frames. Both ends authenticate with the
	// session JWT (Bearer for a server-side harness, the subprotocol for the
	// extension, which can set no headers); the relay routes strictly within one
	// user's channel. See internal/ai/browsertools.
	api.Get("/tools/ws", auth.RequireAuthWS(a.issuer, a.queries, apiKeys{a.queries}), a.BrowserToolsWS())

	// Public job submissions + review queue (see submissionHandlers).
	submissionsH.register(api, mw)

	// Link contributions (see contributionHandlers).
	contributionsH.register(api, mw)
	jdResolveH.register(api, mw)

	// Employee referrals (see referralHandlers).
	referralsH.register(api, mw)

	// Job reports + review queue (see reportHandlers).
	reportsH.register(api, mw)
	ghostReportsH.register(api, mw)

	// Community discussion threads (see communityHandlers).
	communityH.register(api, mw)

	creditsH.register(api, mw)
	usageH.register(api, mw)

	// API-key management and the auth surface (see authHandlers).
	authH.register(api, mw)
	authH.registerV2(app.Group("/api/v2"), mw)

	// The per-user profile singleton (see profileHandlers).
	profileH.register(api, mw)
	// The candidate's own screening answers (see screeningAnswersHandlers).
	screeningAnswersH.register(api, mw)
	marketPulseH.register(api, mw)
	experienceH.register(api, mw)
	talentNetworkH.register(api, mw)
	talentNetworkProfileH.register(api)

	// CV builder + AI tailoring (see cvHandlers).
	cvH.register(api, mw)

	// Mail inbox (Gmail connect + hosted mailbox) and email ↔ application linking
	// (see inboxHandlers). Registered after the static /me/tracking/* routes so
	// /me/tracking/:slug does not shadow them.
	inboxH.register(api, mw)
	// Résumé/CV surfaces: verdict, ATS report, extraction, storage (see
	// resumeHandlers).
	resumeH.register(api, mw)
	// The headshot the photo-bearing CV templates print (see photoHandlers).
	photoH.register(api, mw)
	// Dictation for the assistant composer.
	speechH.register(api, mw)
	assistantH.register(api, mw)

	// Filter subscriptions (see subscriptionHandlers).
	subscriptionH.register(api, mw)

	// Telegram linking + the inbound bot webhook (see telegramHandlers).
	telegramH.register(api, mw)
	// Discord linking + the inbound interaction webhook (see discordHandlers).
	discordH.register(api, mw)

}
