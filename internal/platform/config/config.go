package config

import (
	"os"
	"strings"
	"time"
)

// Settings holds application configuration read from environment variables.
type Settings struct {
	Env  string
	Port string
	// MetricsPort, if set, serves /metrics on its own listener so a firewall rule for it
	// does not expose the rest of the API. Empty disables /metrics.
	MetricsPort string
	// AgentViewLogDir is where the server writes internal/application/agentview's day files.
	// Unset, nothing is written.
	AgentViewLogDir string
	DatabaseURL     string
	FrontendOrigin  string

	AuthIntrospectionSecret string

	// JWTSecret has no default: the server fails fast when it is empty (see cmd/server).
	// Only the server calls Load, so the check lives there.
	JWTSecret string
	JWTTTL    time.Duration

	// CookieSecure marks the auth cookie HTTPS-only. Default false for http://localhost dev;
	// set COOKIE_SECURE=true in any HTTPS deployment.
	CookieSecure bool

	// CookieDomains are the registrable domains the session cookie is scoped to, from the
	// comma-separated COOKIE_DOMAIN (a leading dot is stripped). Empty in dev (host-only cookie).
	// Each request's cookie is scoped to the entry its host falls under. The same set gates
	// the OAuth redirect origin (see handler.requestOrigin).
	CookieDomains []string

	// OAuth holds per-provider client credentials (google, github, linkedin). A provider with
	// incomplete credentials is disabled where the provider registry is built, not here.
	OAuth map[string]OAuthCredentials

	// AuthV2 enables verifier-bound native OAuth endpoints. Browser-provider v2
	// can run without Apple; native Apple is advertised only when its separate
	// bundle client id and refresh-grant encryption key ring are complete.
	AuthV2Enabled bool
	// MeFeedEnabled registers GET /me/feed, the match feed (off until it ships).
	MeFeedEnabled       bool
	MobileAuthCallbacks map[string]string
	AppleNativeClientID string
	// OJCPSigningKeyFile is the path to a PEM P-256 private key on the host that signs the
	// OJCP manifest. A path rather than the key itself, because .env cannot hold a multi-line
	// value. Empty leaves the manifest unsigned, which is also the rollback.
	OJCPSigningKeyFile    string
	AppleGrantActiveKeyID string
	AppleGrantKeys        map[string][]byte
	RecentAuthTTL         time.Duration

	// GmailTokenKey is the 32-byte AES key (GMAIL_TOKEN_KEY, base64) that encrypts stored
	// Gmail refresh tokens. Empty or invalid disables the Connect-Gmail feature.
	GmailTokenKey []byte

	// MailboxDomain is the receiving domain hosted mailboxes live on (<handle>@MailboxDomain).
	// Empty turns the hosted-mailbox option off. Not named MAIL_DOMAIN: that reads like the SES
	// sending identity, a different domain.
	MailboxDomain string

	// TypesafeAPIKey backs the Jev provider for discrete-choice LLM calls: the autofill agent's
	// Choose, select-kind auto-apply drafting, the Telegram vacancy judge and inbox mail
	// classification. Empty disables all four, each falling back to its non-Jev path. Unrelated to LLM_*.
	TypesafeAPIKey string

	// Meilisearch backs job search and the reindex command. MeiliKey empty disables search;
	// the server still starts (enforced at the call site, not here).
	MeiliURL string
	MeiliKey string

	// RedisURL backs shared application caches. Required (no disabled mode), so it only has a
	// localhost default, like DatabaseURL.
	RedisURL string

	// LLM backs the CV ATS review and the in-app assistant. Any empty field disables the AI
	// layer: the server builds no LLM client (enforced at the cmd/server call site, so Require is
	// deliberately not called here). Workers share the same loader but apply their own policy.
	LLM

	// LLMAdminURL, LLMAdminUsername and LLMAdminPassword reach the gateway's administrative API,
	// which mints the per-user credential each account's calls are spent under. Separate from
	// LLMBaseURL/LLMAPIKey because inference is served under /v1 and administration under /api,
	// and so LLMAPIKey need not be an admin credential. HTTP Basic: the management surface admits
	// one identity.
	//
	// LLMAdminTemplateKey names the virtual key whose provider policy every minted credential
	// copies. Required: the gateway denies every provider to a key with no policy.
	//
	// Any of them empty disables attribution; every call then goes out on LLMAPIKey.
	LLMAdminURL         string
	LLMAdminUsername    string
	LLMAdminPassword    string
	LLMAdminTemplateKey string

	// LLMUserMaxBudget and LLMUserRPMLimit bound one account at the gateway. Both are unset by
	// default and omitted from a minted credential when zero: a ceiling chosen before the spend
	// distribution is known is a guess. LLMUserBudgetWindow is the budget reset period ("30d").
	LLMUserMaxBudget    float64
	LLMUserRPMLimit     int
	LLMUserBudgetWindow string

	// AssistantModel is the model the in-app agent runs on, separate from LLMModel because it
	// needs reliable tool calling and a large context. Empty falls back to LLMModel.
	// AssistantMaxSteps bounds tool-calling rounds per turn; zero uses the package default.
	AssistantModel    string
	AssistantMaxSteps int
	// SearchIntentModel is the model the AI filter runs on. The job is trivial but a user waits
	// on a spinner, so latency is the bar: use a small fast model. Empty falls back to LLMModel.
	SearchIntentModel string
	// ResumeExtractModel is the model a stored CV is parsed on. Every account gets this call,
	// paid or not, so it runs on a cheap model. Empty falls back to LLMModel.
	ResumeExtractModel string
	// SupportModel is the model the support agent runs on. Open to every account, so it gets a
	// cheap model of its own. Empty falls back to LLMModel.
	SupportModel string
	// AssistantMaxPrompt is the max rune length of one user message to the assistant
	// (ASSISTANT_MAX_PROMPT, default 8000). Values below 1 fall back to the default.
	AssistantMaxPrompt int

	// STTModel transcribes dictated audio through the same gateway and key as the LLM settings.
	// No default: transcription is billed per minute and the assistant is not metered. Empty makes
	// the endpoint answer 501 and hides the microphone.
	STTModel string

	// RealtimeModel is the OpenAI Realtime model voice mode mints call credentials against, via
	// the same gateway and key as STTModel. No default, because Realtime audio is billed per
	// minute. Empty makes the mint endpoint answer 501 and hides voice mode.
	RealtimeModel string

	ElevenLabsAPIKey     string
	ElevenLabsVoiceID    string
	ElevenLabsViaBifrost bool

	// S3 backs CV storage (internal/platform/blobstore); any S3-compatible endpoint works. All four
	// must be set to enable it; any empty field disables it (upload then only extracts skills
	// in-request). Enforced at the cmd/server call site.
	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

	// Catalogue export (internal/search/catalogexport): a separate bucket from S3_* because
	// that one holds résumés. Optional and inert when any of the first four is empty — the
	// worker refuses to run and the download endpoint answers 501. ExportS3Region is the
	// provider's region name (Hetzner: hel1, fsn1, nbg1). ExportKeep is how many complete
	// snapshots the worker leaves in the bucket.
	ExportS3Endpoint  string
	ExportS3Bucket    string
	ExportS3AccessKey string
	ExportS3SecretKey string
	ExportS3Region    string
	ExportKeep        int

	// TypstBin is the resolved path to the typst binary used to render CV PDFs. Empty disables
	// PDF rendering (the pdf endpoint returns 501). Resolved via exec.LookPath, so empty means
	// the binary is absent, not misconfigured.
	TypstBin string

	// CVEditAllowBulletTruncation restores the old Sanitize behaviour of keeping the first
	// MaxBullets and silently dropping the rest. Default false: Commit refuses that loss.
	// CV_EDIT_ALLOW_BULLET_TRUNCATION=true is an ops kill switch (restart required).
	CVEditAllowBulletTruncation bool

	// CVMaxBullets is the per-experience / per-project bullet ceiling (cv.MaxBullets), default 20.
	// Values below 1 fall back to the default.
	CVMaxBullets int

	// MatchAnalysis holds sanitize caps for the fit-analysis LLM output; override via
	// MATCH_ANALYSIS_*. Values below 1 fall back to the package default at SetBounds time.
	MatchAnalysis MatchAnalysisSettings

	// TracerLinkSalt keys the visitor hash of a click on a traced CV link. Empty disables the
	// consent toggle: without a salt the hash is reversible by walking the address space.
	TracerLinkSalt string

	// Telegram bot for outbound notifications. An empty TelegramBotToken disables the feature.
	// TelegramBotUsername builds the t.me deep link and marks the feature enabled in the public
	// config. TelegramWebhookSecret is the inbound webhook's secret_token; empty keeps the feature
	// off, since an empty expected secret would admit any caller.
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string

	// Email notifications via AWS SES. The email channel is registered only when both AWSRegion
	// and NotifyEmailFrom are set (enforced at the cmd/notify call site); otherwise only Telegram
	// delivers. AWS credentials come from the default AWS chain. NotifyEmailFrom is the verified
	// SES sender.
	AWSRegion       string
	NotifyEmailFrom string
	// SESEngagementConfigurationSet is the SES configuration set publishing clicks and
	// delivery outcomes for campaigns and digests. Empty tracks nothing.
	SESEngagementConfigurationSet string

	// DiscordDigestWebhookURL is the incoming-webhook URL cmd/social-digest posts the daily
	// "most viewed" list to. Unset means not configured, not a failed run. The URL is itself the
	// credential and does not expire.
	DiscordDigestWebhookURL string

	// The daily digest's LinkedIn channel: the application that posts to our company page, and
	// the page. Optional as a set: a deployment holding only some values has not configured it
	// (see LinkedInDigestConfigured). The access token is not here: it expires every 60 days and a
	// worker renews it, so it lives in social_tokens.
	//
	// LinkedInRedirectURI must be registered on the LinkedIn app and absolute HTTPS (LinkedIn
	// refuses http, even on localhost), so the sign-in flow hands a code to a command rather than
	// a loopback listener.
	LinkedInClientID       string
	LinkedInClientSecret   string
	LinkedInRedirectURI    string
	LinkedInOrganizationID string

	// OnboardingReplyTo is the human inbox answering the founder signup sequence. Unset disables
	// the sequence: the send address is a no-reply, so replies would reach nobody.
	OnboardingReplyTo string

	// ServedHosts are the exact hostnames this deployment answers on. Only these are honoured as
	// an OAuth redirect origin; anything else falls back to FrontendOrigin. Empty defaults to the
	// frontend origin's host.
	ServedHosts []string

	// ExtensionRedirectAllowlist bounds where the extension connect flow may hand back a minted
	// token: only https://<id>.chromiumapp.org (Chrome) or https://<id>.extensions.allizom.org
	// (Firefox) redirects with a listed <id>. Empty disables the flow.
	ExtensionRedirectAllowlist []string

	// AutoApplyOrchestratorSecret is the static credential cmd/auto-apply-orchestrate presents on
	// POST /me/auto-apply/:queueId/{tailor,review} to authenticate itself, not a candidate. Empty
	// disables that path; the routes then accept only a session cookie or a live api_keys row.
	AutoApplyOrchestratorSecret string

	// InngestEventAPIURL, InngestEventKey and InngestSigningKey point PostAutoApplyReview's
	// best-effort event publish at the self-hosted Inngest server. The publish itself uses only
	// InngestEventKey; InngestSigningKey is consumed by cmd/auto-apply-orchestrate. Any of the
	// three empty makes the publish log and continue, while the orchestrator fails startup.
	InngestEventAPIURL string
	InngestEventKey    string
	InngestSigningKey  string

	// AutoApplyOrchestratePort is the port cmd/auto-apply-orchestrate serves the Inngest SDK
	// callback on. Internal-only, reached by the self-hosted Inngest server on the same host.
	AutoApplyOrchestratePort string
}

// OAuthCredentials is one OAuth provider's credentials. Apple authenticates with a self-signed
// JWT, so it uses ClientID (Services ID) plus TeamID/KeyID/PrivateKey and leaves ClientSecret
// empty. PrivateKey is the decoded PEM (the env var is base64).
type OAuthCredentials struct {
	ClientID     string
	ClientSecret string
	TeamID       string
	KeyID        string
	PrivateKey   string
}

// MatchAnalysisSettings carries the MATCH_ANALYSIS_* overrides for fit-analysis sanitize
// ceilings; cmd/server applies them via matchanalysis.SetBounds at boot (restart required).
//
// Zero means not overridden: SetBounds substitutes the field's Default* from
// internal/candidate/matchanalysis. No defaults live here, so a copy cannot shadow the domain's.
type MatchAnalysisSettings struct {
	// MaxCommentRunes caps each dimension's Comment.
	MaxCommentRunes int
	// MaxListItemRunes caps each Strengths / Gaps bullet.
	MaxListItemRunes int
	// MaxRecommendRunes caps the free-text Recommendation.
	MaxRecommendRunes int
	// MaxReqTextRunes caps Requirement.Text.
	MaxReqTextRunes int
	// MaxReqEvidenceRunes caps Requirement.Evidence.
	MaxReqEvidenceRunes int
	// MaxStrengths caps the Strengths list length.
	MaxStrengths int
	// MaxGaps caps the Gaps list length.
	MaxGaps int
	// MaxRequirements caps the RequirementMatch list length.
	MaxRequirements int
	// MaxSignals caps the HiddenSignals list length.
	MaxSignals int
	// MaxSignalQuoteRunes caps Signal.Quote.
	MaxSignalQuoteRunes int
	// MaxSignalInsightRunes caps Signal.Insight.
	MaxSignalInsightRunes int
}

// oauthProviders are the providers whose credentials Load reads (OAUTH_<PROVIDER>_*).
var oauthProviders = []string{"google", "github", "linkedin", "apple"}

// defaultAssistantMaxPrompt is the rune ceiling when ASSISTANT_MAX_PROMPT is unset or invalid.
const defaultAssistantMaxPrompt = 8000

// Load reads configuration from the environment, falling back to sensible defaults.
func Load() Settings {
	s := Settings{
		Env:                     env("ENV", "development"),
		LLM:                     LoadLLM(),
		Port:                    env("PORT", "8080"),
		MetricsPort:             os.Getenv("METRICS_PORT"),
		AgentViewLogDir:         os.Getenv("AGENT_VIEW_LOG_DIR"),
		DatabaseURL:             env("DATABASE_URL", "postgres://hire:hire@localhost:5432/hire?sslmode=disable"),
		FrontendOrigin:          env("FRONTEND_ORIGIN", "http://localhost:5173"),
		AuthIntrospectionSecret: strings.TrimSpace(os.Getenv("AUTH_INTROSPECTION_SECRET")),
		JWTSecret:               os.Getenv("JWT_SECRET"),
		JWTTTL:                  envDuration("JWT_TTL", 30*24*time.Hour),
		CookieSecure:            envBool("COOKIE_SECURE", false),
		CookieDomains:           splitDomains(os.Getenv("COOKIE_DOMAIN")),
		OAuth:                   loadOAuth(),
		AuthV2Enabled:           envBool("AUTH_V2_ENABLED", false),
		MeFeedEnabled:           envBool("ME_FEED_ENABLED", false),
		MobileAuthCallbacks:     parseNamedValues(os.Getenv("MOBILE_AUTH_CALLBACKS")),
		AppleNativeClientID:     strings.TrimSpace(os.Getenv("APPLE_NATIVE_CLIENT_ID")),
		OJCPSigningKeyFile:      strings.TrimSpace(os.Getenv("OJCP_SIGNING_KEY_FILE")),
		AppleGrantActiveKeyID:   strings.TrimSpace(os.Getenv("APPLE_GRANT_ACTIVE_KEY_ID")),
		AppleGrantKeys:          parseKeyRing(os.Getenv("APPLE_GRANT_KEYS")),
		RecentAuthTTL:           envDuration("RECENT_AUTH_TTL", 10*time.Minute),
		GmailTokenKey:           decodeKey(os.Getenv("GMAIL_TOKEN_KEY")),
		MailboxDomain:           os.Getenv("MAILBOX_DOMAIN"),
		TypesafeAPIKey:          os.Getenv("TYPESAFE_API_KEY"),
		MeiliURL:                env("MEILI_URL", "http://localhost:7700"),
		MeiliKey:                os.Getenv("MEILI_MASTER_KEY"),
		RedisURL:                env("REDIS_URL", "redis://localhost:6379/0"),

		AutoApplyOrchestratorSecret: os.Getenv("AUTO_APPLY_ORCHESTRATOR_SECRET"),
		InngestEventAPIURL:          os.Getenv("INNGEST_EVENT_API_URL"),
		InngestEventKey:             os.Getenv("INNGEST_EVENT_KEY"),
		InngestSigningKey:           os.Getenv("INNGEST_SIGNING_KEY"),
		AutoApplyOrchestratePort:    env("AUTO_APPLY_ORCHESTRATE_PORT", "3010"),

		LLMAdminURL:         os.Getenv("LLM_ADMIN_URL"),
		LLMAdminUsername:    os.Getenv("LLM_ADMIN_USERNAME"),
		LLMAdminPassword:    os.Getenv("LLM_ADMIN_PASSWORD"),
		LLMAdminTemplateKey: os.Getenv("LLM_ADMIN_TEMPLATE_KEY"),
		LLMUserMaxBudget:    envFloat("LLM_USER_MAX_BUDGET", 0),
		LLMUserRPMLimit:     envInt("LLM_USER_RPM_LIMIT", 0),
		LLMUserBudgetWindow: env("LLM_USER_BUDGET_WINDOW", "30d"),

		AssistantModel:     os.Getenv("ASSISTANT_MODEL"),
		SearchIntentModel:  os.Getenv("SEARCH_INTENT_MODEL"),
		ResumeExtractModel: os.Getenv("RESUME_EXTRACT_MODEL"),
		SupportModel:       os.Getenv("SUPPORT_MODEL"),
		AssistantMaxSteps:  envInt("ASSISTANT_MAX_STEPS", 0),
		AssistantMaxPrompt: envInt("ASSISTANT_MAX_PROMPT", defaultAssistantMaxPrompt),

		STTModel: os.Getenv("STT_MODEL"),

		RealtimeModel: os.Getenv("REALTIME_MODEL"),

		ElevenLabsAPIKey:     os.Getenv("ELEVENLABS_API_KEY"),
		ElevenLabsVoiceID:    os.Getenv("ELEVENLABS_VOICE_ID"),
		ElevenLabsViaBifrost: os.Getenv("ELEVENLABS_VIA_BIFROST") == "true",

		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),

		ExportS3Endpoint:  os.Getenv("CATALOGUE_EXPORT_S3_ENDPOINT"),
		ExportS3Bucket:    os.Getenv("CATALOGUE_EXPORT_S3_BUCKET"),
		ExportS3AccessKey: os.Getenv("CATALOGUE_EXPORT_S3_ACCESS_KEY"),
		ExportS3SecretKey: os.Getenv("CATALOGUE_EXPORT_S3_SECRET_KEY"),
		ExportS3Region:    os.Getenv("CATALOGUE_EXPORT_S3_REGION"),
		ExportKeep:        envInt("CATALOGUE_EXPORT_KEEP", 4),

		TypstBin:                    resolveTypstBin(env("TYPST_BIN", "typst")),
		CVEditAllowBulletTruncation: envBool("CV_EDIT_ALLOW_BULLET_TRUNCATION", false),
		CVMaxBullets:                envInt("CV_MAX_BULLETS", 20),

		// Zero is the "unset" fallback on purpose — see MatchAnalysisSettings.
		MatchAnalysis: MatchAnalysisSettings{
			MaxCommentRunes:       envInt("MATCH_ANALYSIS_MAX_COMMENT_RUNES", 0),
			MaxListItemRunes:      envInt("MATCH_ANALYSIS_MAX_LIST_ITEM_RUNES", 0),
			MaxRecommendRunes:     envInt("MATCH_ANALYSIS_MAX_RECOMMEND_RUNES", 0),
			MaxReqTextRunes:       envInt("MATCH_ANALYSIS_MAX_REQ_TEXT_RUNES", 0),
			MaxReqEvidenceRunes:   envInt("MATCH_ANALYSIS_MAX_REQ_EVIDENCE_RUNES", 0),
			MaxStrengths:          envInt("MATCH_ANALYSIS_MAX_STRENGTHS", 0),
			MaxGaps:               envInt("MATCH_ANALYSIS_MAX_GAPS", 0),
			MaxRequirements:       envInt("MATCH_ANALYSIS_MAX_REQUIREMENTS", 0),
			MaxSignals:            envInt("MATCH_ANALYSIS_MAX_SIGNALS", 0),
			MaxSignalQuoteRunes:   envInt("MATCH_ANALYSIS_MAX_SIGNAL_QUOTE_RUNES", 0),
			MaxSignalInsightRunes: envInt("MATCH_ANALYSIS_MAX_SIGNAL_INSIGHT_RUNES", 0),
		},

		TelegramBotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramBotUsername:   os.Getenv("TELEGRAM_BOT_USERNAME"),
		TelegramWebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),

		AWSRegion:       os.Getenv("AWS_REGION"),
		NotifyEmailFrom: os.Getenv("NOTIFY_EMAIL_FROM"),

		SESEngagementConfigurationSet: os.Getenv("SES_ENGAGEMENT_CONFIGURATION_SET"),

		DiscordDigestWebhookURL: os.Getenv("DISCORD_DIGEST_WEBHOOK_URL"),

		LinkedInClientID:       os.Getenv("LINKEDIN_CLIENT_ID"),
		LinkedInClientSecret:   os.Getenv("LINKEDIN_CLIENT_SECRET"),
		LinkedInRedirectURI:    os.Getenv("LINKEDIN_REDIRECT_URI"),
		LinkedInOrganizationID: os.Getenv("LINKEDIN_ORGANIZATION_ID"),

		OnboardingReplyTo: os.Getenv("ONBOARDING_REPLY_TO"),

		ServedHosts:                splitCSV(os.Getenv("SERVED_HOSTS")),
		TracerLinkSalt:             os.Getenv("TRACER_LINK_SALT"),
		ExtensionRedirectAllowlist: splitCSV(os.Getenv("EXTENSION_REDIRECT_ALLOWLIST")),
	}
	if s.AssistantMaxPrompt < 1 {
		s.AssistantMaxPrompt = defaultAssistantMaxPrompt
	}
	return s
}

func loadOAuth() map[string]OAuthCredentials {
	creds := make(map[string]OAuthCredentials, len(oauthProviders))
	for _, p := range oauthProviders {
		prefix := "OAUTH_" + strings.ToUpper(p)
		creds[p] = OAuthCredentials{
			ClientID:     os.Getenv(prefix + "_CLIENT_ID"),
			ClientSecret: os.Getenv(prefix + "_CLIENT_SECRET"),
			TeamID:       os.Getenv(prefix + "_TEAM_ID"),
			KeyID:        os.Getenv(prefix + "_KEY_ID"),
			PrivateKey:   decodeBase64(os.Getenv(prefix + "_PRIVATE_KEY")),
		}
	}
	return creds
}
