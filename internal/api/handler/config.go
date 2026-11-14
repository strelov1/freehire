package handler

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/ai/speech"
	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/api/realtime"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/candidate/pii"
	"github.com/strelov1/freehire/internal/identity/apiusage"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/job/recentfeed"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
	"github.com/strelov1/freehire/internal/search/embedprogress"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"
)

// Config is the dependency bundle Register wires onto the app: the DB pool, the
// single browser origin allowed cross-origin
// (FrontendOrigin), the token-issuer settings (JWTSecret/JWTTTL), the HTTPS-only
// cookie flag (CookieSecure), the enabled OAuth providers, and the optional
// search client (nil disables search).
type Config struct {
	Pool *pgxpool.Pool
	// AgentViews receives the postings served to agents (internal/application/agentview).
	// Optional: nil records nothing.
	AgentViews agentview.Sink
	// APIUsage counts each account's Bearer-authenticated API requests per day
	// (internal/identity/apiusage). Optional: nil counts nothing.
	APIUsage *apiusage.Counter
	// Cache holds values that are expensive to compute and identical for every
	// caller — today the catalogue-scale snapshot (internal/ingest/catalogstats). Optional:
	// nil degrades the figures it backs to their approximations, exactly as an
	// unreachable backend does.
	Cache                   cache.Cache
	FrontendOrigin          string
	AuthIntrospectionSecret string
	JWTSecret               string
	JWTTTL                  time.Duration
	CookieSecure            bool
	CookieDomains           []string
	OAuthRegistry           *oauth.Registry
	AuthV2Enabled           bool
	MobileAuthCallbacks     map[string]string
	RecentAuthTTL           time.Duration
	AppleNative             *appleauth.Client
	AppleGrantKeys          *appleauth.KeyRing
	Search                  *search.Client
	// Suggest answers the search box's completions from the dictionary index
	// (cmd/build-suggestions writes it). Nil disables /suggest with a 503, the same
	// shape as an unconfigured search — the rest of the site is unaffected, and the
	// box falls back to offering nothing rather than to erroring.
	Suggest *suggest.Service
	// Blob backs résumé storage (internal/platform/blobstore). Nil disables storage: résumé
	// upload only extracts skills in-request (no regression).
	Blob blobstore.Store
	// DirectApplyDailyLimit caps how many on-site applications one candidate sends per UTC
	// day (config.LoadDirectApplyDailyLimit, read and validated in cmd/server).
	DirectApplyDailyLimit int64
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
	// ResumeExtractLLM backs the structured parse of a stored CV — free for every
	// account, so on its own cheap model (config.ResumeExtractModel). Nil falls back to
	// LLM.
	ResumeExtractLLM *llm.Client
	// SupportLLM backs the support agent — open to every account, so on its own cheap
	// model (config.SupportModel). Nil leaves support unavailable.
	SupportLLM *llm.Client
	// SupportReplyTo is the one inbox a support handover is emailed to
	// (ONBOARDING_REPLY_TO). Empty makes the handover tool report itself unavailable.
	SupportReplyTo string
	// TypesafeAPIKey backs the Jev provider for the autofill agent's Choose call
	// (autofillagent.JevPlanner). Empty disables it: Choose answers through LLM alone.
	TypesafeAPIKey string
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
	// DiscordLinker enables the paid Discord channels: linking an account, and the role
	// that follows the subscription. Nil = the feature is off and its routes are not
	// registered at all, so they 404 rather than reporting a feature that half exists.
	// DiscordClientID builds the consent URL; it is only read when the linker is present.
	DiscordLinker   DiscordLinker
	DiscordClientID string
	// GmailConnector + GmailCipher enable the Connect-Gmail inbox. Both nil = the
	// feature is off (connect routes unregistered, inbox empty).
	GmailConnector *gmailsync.Connector
	GmailCipher    *tokencrypt.Cipher
	// OJCPSigner signs the OJCP manifest. Nil = the manifest is unsigned and no key set is
	// published.
	OJCPSigner *ojcp.Signer
	// MailboxDomain enables the hosted-mailbox option: the receiving domain user
	// addresses live on (<handle>@MailboxDomain). Empty = the feature is off.
	MailboxDomain string
	// Plan carries what each plan allows per day, per feature, and whether a refusal is
	// enforced yet. Every metered AI surface draws on it.
	Plan plan.Config
	// Billing connects the Pro subscription at the payment provider to users.pro_until,
	// which is what Plan then reads. Optional and off by default: with no provider
	// credentials the billing routes are not mounted at all, so a deployment that never
	// configures it cannot tell they exist.
	Billing billing.Config
	// AWSRegion + NotifyEmailFrom enable the SES email channel for referral pings, reusing
	// the notify worker's config. Both must be set; either empty leaves referral pings
	// Telegram-only (email disabled). NotifyEmailFrom is the verified SES sender address.
	AWSRegion       string
	NotifyEmailFrom string
	// ExtensionRedirectAllowlist bounds the browser-extension connect flow: only
	// https://<id>.chromiumapp.org or <id>.extensions.allizom.org redirects whose <id> is listed may receive a
	// minted token. Empty leaves the connect endpoint refusing every redirect.
	ExtensionRedirectAllowlist []string
	// ServedHosts are the exact hostnames honoured as an OAuth redirect origin.
	// Empty defaults to the frontend origin's own host.
	ServedHosts []string
	// AutoApplyOrchestratorSecret is the shared, static credential
	// cmd/auto-apply-orchestrate presents to authenticate itself (not any particular
	// candidate) on POST /me/auto-apply/:queueId/{tailor,review} — see
	// openspec/changes/auto-apply-inngest-orchestration/design.md. Empty disables the
	// path entirely: the two routes then behave exactly as they did under plain
	// cookie/API-key auth, with no fallback.
	AutoApplyOrchestratorSecret string
	// InngestEventAPIURL and InngestEventKey point PostAutoApplyReview's best-effort
	// event publish (auto-apply/review.decided) at the self-hosted Inngest server. Either
	// empty disables the publish entirely — the review decision itself is still recorded,
	// exactly as it is today.
	InngestEventAPIURL string
	InngestEventKey    string
	// RecentJobsFeed backs the homepage's live "recently added jobs" SSE feed
	// (internal/job/recentfeed). cmd/server owns the Poller that publishes into it;
	// this handler only subscribes. Nil disables the route with a 503 rather than
	// serving a feed that will never receive anything.
	RecentJobsFeed *recentfeed.Broadcaster
	// EmbedProgress backs the public /embeddings page's figures and stream
	// (internal/search/embedprogress). cmd/server owns the Sampler's tick; this handler
	// only reads it. Nil serves 503 on both routes.
	EmbedProgress *embedprogress.Sampler
}
