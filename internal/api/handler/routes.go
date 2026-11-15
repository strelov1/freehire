package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// routeHandlers is every feature surface Register built: the ones the HTTP contract serves
// (contractHandlers) and the ones that stay on plain Fiber routes.
type routeHandlers struct {
	contractHandlers
	sitemap         *sitemapHandlers
	ojcp            *ojcpHandlers
	mcpapp          *mcpappHandlers
	recentFeed      *recentFeedHandlers
	og              *ogHandlers
	referralMessage *referralMessageHandlers
	photo           *photoHandlers
	speech          *speechHandlers
	webhook         *webhookHandlers
	telegram        *telegramHandlers
	discord         *discordHandlers
}

// mountRoutes mounts every feature surface's routes on api (and the v2 auth routes on app).
// Order matters where a comment says so: Fiber matches routes in registration order.
func (a *API) mountRoutes(app *fiber.App, api fiber.Router, mw middleware, contract fiber.Handler, h routeHandlers) {
	// Job search surfaces first: their literal /jobs/* routes must precede the
	// /jobs/:slug param route so they are not read as slugs (see searchHandlers).
	h.searchHandlers.register(api, mw, contract)
	h.suggestHandlers.register(api, contract)
	h.dictionaryHandlers.register(api, mw, contract)
	// Beside the search it builds a filter for, though it shares none of its
	// dependencies: interpretation needs a model and the caller's profile, not the
	// index.
	h.intentHandlers.register(api, mw, contract)
	h.sitemap.register(api, mw)
	// Before jobsH for the same reason searchH is: /ojcp/v1/jobs/:slug is a literal path,
	// not a slug.
	h.ojcp.register(api, mw)
	// The MCP server (freehire#3114): bearer-gated by an MCP OAuth grant or a full-scope
	// key, reusing assistantH's own tool set so this surface and the in-app assistant can
	// never disagree about what a tool does.
	mountMCP(api, h.assistantHandlers, h.mcpapp)
	h.jobsHandlers.register(api, mw, contract)
	h.companiesHandlers.register(api, mw, contract)
	h.companyRelationHandlers.register(api, mw, contract)
	h.geoHandlers.register(api, contract)

	// Saved searches + the public shared-board read (see savedSearchHandlers).
	h.savedSearchHandlers.register(api, mw, contract)

	// Public catalogue-activity, member-growth, engagement, facet-snapshot, and
	// ingest-status reads (see statsHandlers).
	h.statsHandlers.register(api, mw.optional, contract)
	// The homepage's live "recently added jobs" SSE feed (see recentFeedHandlers).
	h.recentFeed.register(api)
	// The public /embeddings page's coverage figures and their live stream.
	h.embedProgressHandlers.register(api, contract)
	// The /open and /about pages' OG preview cards (see ogHandlers) — reads the
	// same snapshot statsH.CatalogScale serves.
	h.og.register(api)

	// Per-user job interactions, tracking reads, and reminder controls
	// (see trackingHandlers). The interaction writes precede the vote routes,
	// mirroring the previous registration order.
	h.trackingHandlers.register(api, mw, contract)
	// The ledger's dated read, behind the tracking calendar. Its own namespace, not a
	// segment under /me/tracking — see the comment on its register.
	h.timelineHandlers.register(api, mw, contract)
	h.voteHandlers.register(api, mw, contract)
	h.companyWatchHandlers.register(api, mw, contract)
	h.companyFeedbackHandlers.register(api, mw, contract)
	// The manual account ban (see userModerationHandlers).
	h.userModerationHandlers.register(api, mw, contract)
	h.companyProcessReportHandlers.register(api, mw, contract)
	// Per-job skill match + the on-demand LLM fit analysis (see matchHandlers).
	h.matchHandlers.register(api, mw, contract)
	// The contact block the extension writes into forms, plain and agent-driven (see
	// autofillHandlers).
	h.autofillHandlers.register(api, mw, contract)
	// The bank's own management surface: list, save, delete (see me_answer_bank.go).
	h.answerBankHandlers.register(api, mw, contract)
	h.agentPermissionHandlers.register(api, mw, contract)
	// The browser-tool wire: a harness on one end, the caller's browser extension
	// on the other, exchanging raw tool frames. Both ends authenticate with the
	// session JWT (Bearer for a server-side harness, the subprotocol for the
	// extension, which can set no headers); the relay routes strictly within one
	// user's channel. See internal/ai/browsertools.
	api.Get("/tools/ws", auth.RequireAuthWS(a.issuer, a.queries, apiKeys{a.queries}), a.BrowserToolsWS())

	// Public job submissions + review queue (see submissionHandlers).
	h.submissionHandlers.register(api, mw, contract)

	// Verified-employer accounts: claim, curated profile, own vacancies, and the
	// moderator/admin review surface (see employerHandlers).
	h.employerHandlers.register(api, mw, contract)
	h.directApplyHandlers.register(api, mw, contract)

	// Link contributions (see contributionHandlers).
	h.contributionHandlers.register(api, mw, contract)
	h.jdResolveHandlers.register(api, mw, contract)

	// Employee referrals (see referralHandlers).
	h.referralHandlers.register(api, mw, contract)

	// Interview buddies (see buddyHandlers).
	h.buddyHandlers.register(api, mw, contract)

	// Conversations of every kind (see conversationHandlers).
	h.conversationHandlers.register(api, mw, contract)

	// The mentorship marketplace (see mentorshipHandlers).
	h.mentorshipHandlers.register(api, mw, contract)
	h.mentorSuggestionsHandlers.register(api, mw, contract)

	// Job reports + review queue (see reportHandlers).
	h.reportHandlers.register(api, mw, contract)
	h.ghostReportHandlers.register(api, mw, contract)

	// Community discussion threads (see communityHandlers).
	h.communityHandlers.register(api, mw, contract)

	h.planHandlers.register(api, mw, contract)
	// Mounts nothing when billing is unconfigured — see billingHandlers.register.
	h.billingHandlers.register(api, mw, contract)
	// Mounted whatever billing is doing: sharing an invite link and counting who came is
	// not a purchase, and a deployment that sells nothing can still have a referral page.
	h.promoHandlers.register(api, mw, contract)
	h.plansHandlers.register(api, contract)
	h.usageHandlers.register(api, mw, contract)

	// API-key management and the auth surface (see authHandlers).
	authH := h.authHandlers
	authH.register(api, mw, contract)
	authH.registerV2(app.Group("/api/v2"), mw)

	// The per-user profile singleton (see profileHandlers).
	h.profileHandlers.register(api, mw, contract)
	// The candidate's own screening answers (see screeningAnswersHandlers).
	h.screeningAnswersHandlers.register(api, mw, contract)
	h.surveyHandlers.register(api, mw, contract)
	h.marketPulseHandlers.register(api, mw, contract)
	h.experienceHandlers.register(api, mw, contract)
	h.talentNetworkHandlers.register(api, mw, contract)
	h.talentCatalogHandlers.register(api, contract)

	// CV builder + AI tailoring (see cvHandlers).
	h.cvHandlers.register(api, mw, contract)
	h.referralMessage.register(api, mw)

	// Mail inbox (Gmail connect + hosted mailbox) and email ↔ application linking
	// (see inboxHandlers). Registered after the static /me/tracking/* routes so
	// /me/tracking/:slug does not shadow them.
	h.inboxHandlers.register(api, mw, contract)
	// Résumé/CV surfaces: verdict, ATS report, extraction, storage (see
	// resumeHandlers).
	h.resumeHandlers.register(api, mw, contract)
	// The headshot the photo-bearing CV templates print (see photoHandlers).
	h.photo.register(api, mw)
	// Dictation for the assistant composer.
	h.speech.register(api, mw)
	assistantH := h.assistantHandlers
	assistantH.register(api, mw, contract)
	assistantH.registerSupport(api, mw, contract)

	// Filter subscriptions (see subscriptionHandlers).
	h.subscriptionHandlers.register(api, mw, contract)
	h.webhook.register(api, mw)

	// Telegram linking + the inbound bot webhook (see telegramHandlers).
	h.telegram.register(api, mw)

	// Discord linking for the paid channels. Registers nothing when the feature is
	// unconfigured (see discordHandlers).
	h.discord.register(api, mw)
}

// mountAppRoutes mounts what sits on the app itself rather than under /api/v1: the CORS
// policy, the health check and the public side of CV link tracing.
func (a *API) mountAppRoutes(app *fiber.App, cfg Config, queries *db.Queries) {
	// Allow the canonical frontend origin plus every served domain's https apex,
	// so a cross-origin (non-credentialed) read works from either domain during a
	// migration. Same-origin app traffic never triggers CORS; this only covers the
	// dev Vite origin and genuine cross-origin callers.
	// auth.BrowserOrigins is the ONE place this pair is turned into origins — the
	// request-origin gate on session routes (requireSameOrigin) accepts a browser
	// request's Origin/Referer only when it names a member of the same list, so the
	// two policies cannot drift apart.
	allowOrigins := strings.Join(auth.BrowserOrigins(cfg.FrontendOrigin, cfg.CookieDomains, cfg.ServedHosts...), ",")
	app.Use(cors.New(cors.Config{AllowOrigins: allowOrigins}))

	app.Get("/health", a.Health)

	// The public side of CV link tracing. It sits beside /health rather than under /api/v1
	// because it lives inside a PDF a recruiter opens by hand: "freehire.me/cv/acme-x7abc" is a
	// URL a person may read off a hover tooltip, and "/api/v1/..." would not be.
	//
	// Optional cookie auth, never a key: it is what lets a candidate's own click be recognised
	// and excluded, and a leaked API key must not be able to attribute or hide one.
	//
	tracerH := newTracerHandlers(queries, cfg.TracerLinkSalt)
	app.Get("/cv/:token", auth.OptionalCookieAuth(a.issuer, queries), tracerH.Redirect)
}
