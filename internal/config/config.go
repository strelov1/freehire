package config

import (
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Settings holds application configuration read from environment variables.
type Settings struct {
	Env  string
	Port string
	// MetricsPort, if set, serves /metrics on its own listener instead of the
	// main API port — so an internal-only firewall rule for this port never
	// exposes the rest of the API surface too. Empty disables /metrics.
	MetricsPort    string
	DatabaseURL    string
	FrontendOrigin string

	// JWT settings for the auth surface. JWTSecret has no default: it is read
	// as-is and the server fails fast when it is empty (see cmd/server). Only
	// the server calls Load — the enrich worker reads its own LoadEnrich — so
	// the requirement lives at the server entry point, not here.
	JWTSecret string
	JWTTTL    time.Duration

	// CookieSecure marks the auth cookie Secure (HTTPS-only). Default false so
	// the cookie works over http://localhost in dev; set COOKIE_SECURE=true in
	// any HTTPS deployment.
	CookieSecure bool

	// CookieDomains are the registrable domains the session cookie is scoped to,
	// one per served domain (e.g. "freehire.dev", "freehire.me"). Read from
	// COOKIE_DOMAIN as a comma-separated list; a leading dot is accepted and
	// stripped ("​.freehire.dev,.freehire.me"). Empty in dev (host-only cookie).
	// Per request the cookie is scoped to whichever entry the request host falls
	// under, so one deployment can serve — and keep SSO on — more than one domain
	// during a migration. This same set gates the OAuth redirect origin (see
	// handler.requestOrigin), since it is exactly the set of domains we serve.
	CookieDomains []string

	// OAuth holds per-provider client credentials keyed by provider name
	// (google, github, linkedin). OAuth sign-in is optional: a provider with
	// incomplete credentials is simply disabled (enforced where the provider
	// registry is built, not here), and the server starts either way.
	OAuth map[string]OAuthCredentials

	// AuthV2 enables verifier-bound native OAuth endpoints. Browser-provider v2
	// can run without Apple; native Apple is advertised only when its separate
	// bundle client id and refresh-grant encryption key ring are complete.
	AuthV2Enabled         bool
	MobileAuthCallbacks   map[string]string
	AppleNativeClientID   string
	AppleGrantActiveKeyID string
	AppleGrantKeys        map[string][]byte
	RecentAuthTTL         time.Duration

	// GmailTokenKey is the 32-byte AES key (from GMAIL_TOKEN_KEY, base64) that
	// encrypts stored Gmail refresh tokens. Empty/invalid = the Connect-Gmail
	// inbox feature is disabled (like an unset optional integration).
	GmailTokenKey []byte

	// MailboxDomain is the receiving domain hosted mailboxes live on
	// (<handle>@MailboxDomain, e.g. inbox.freehire.me). Empty = the hosted-mailbox
	// option is off (claim route unregistered, status reports unavailable). It is
	// deliberately NOT named MAIL_DOMAIN — that name reads like the SES *sending*
	// identity (notifications, NOTIFY_EMAIL_FROM), a different, non-receiving domain.
	MailboxDomain string

	// Meilisearch backs the job search endpoint and the reindex command. Shared
	// via Load (both cmd/server and cmd/reindex read it). Search is optional:
	// MeiliKey empty ⇒ search is disabled and the server still starts (see
	// cmd/server), so the requirement is enforced at the call site, not here.
	MeiliURL string
	MeiliKey string

	// RedisURL backs the shared rate limiter (internal/ratelimit). Unlike Meili,
	// Redis is a required dependency — there is no optional/disabled mode — so
	// this only has a localhost default, the same posture as DatabaseURL.
	RedisURL string

	// LLM backs the optional CV ATS qualitative review and the in-app assistant. Optional
	// and provider-agnostic: any empty field disables the AI layer — the server builds no
	// LLM client and the ATS score stays deterministic (enforced at the cmd/server call
	// site, not here, which is why Require is deliberately NOT called on it). The workers
	// share the same six values and the same loader; what differs is that policy.
	LLM

	// LLMAdminURL and LLMAdminKey reach the gateway's administrative API, which mints
	// the per-user credential each account's calls are spent under. They are named
	// apart from LLMBaseURL/LLMAPIKey for two reasons: the endpoints genuinely differ
	// (inference is served under /v1, administration at the root), and separating them
	// is what later lets LLMAPIKey stop being an administrative credential without a
	// code change. Either empty disables attribution entirely — every call then goes
	// out on LLMAPIKey exactly as it did before.
	LLMAdminURL string
	LLMAdminKey string

	// LLMUserMaxBudget and LLMUserRPMLimit bound one account at the gateway. Both are
	// unset by default and omitted from a minted credential when zero: a ceiling
	// chosen before the spend distribution is known is a guess, and a guess that
	// refuses people is expensive to be wrong about. LLMUserBudgetWindow is the period
	// a budget resets over ("30d"); it only matters once a budget exists.
	LLMUserMaxBudget    float64
	LLMUserRPMLimit     int
	LLMUserBudgetWindow string

	// AssistantModel is the model the in-app agent runs on. It is separate from
	// LLMModel because the two jobs differ: LLMModel is chosen for cheap, one-shot
	// JSON extraction, while the assistant needs reliable tool calling and a large
	// context. Empty falls back to LLMModel. AssistantMaxSteps bounds how many
	// tool-calling rounds one turn may take before the agent is forced to answer;
	// zero uses the package default.
	AssistantModel    string
	AssistantMaxSteps int
	// AssistantMaxPrompt is the max rune length of one user message to the
	// assistant. Default 8000; override with ASSISTANT_MAX_PROMPT. Values below 1
	// fall back to the default so a typo cannot erase the ceiling.
	AssistantMaxPrompt int

	// STTModel transcribes dictated audio, through the same gateway and key the LLM
	// settings above name — an OpenAI-compatible endpoint serves /chat/completions and
	// /audio/transcriptions alike. Named explicitly like every other model here, with
	// no default: transcription is billed per minute of audio against an assistant that
	// is not metered, so a deployment that never said which model to use should not
	// find itself paying for one. Empty leaves the endpoint answering 501 and the
	// composer rendering no microphone.
	STTModel string

	// RealtimeModel is the OpenAI Realtime API model voice mode mints call credentials
	// against, through the same gateway and key as STTModel above. Same posture as
	// STTModel: no default, because Realtime audio is billed per minute and a
	// deployment that never named a model should get no voice mode rather than a bill
	// for one nobody chose. Empty leaves the mint endpoint answering 501 and the
	// interview session view offering no voice mode.
	RealtimeModel string

	// PIIFilterURL is the co-located openai/privacy-filter span-detection endpoint used to
	// mask PII out of CV text before it reaches the LLM (internal/pii). Optional here, but
	// the fit-analysis and structured-résumé paths are fail-closed: an empty value disables
	// them entirely (no CV is sent to the LLM) rather than leaking PII — enforced at the
	// call site, not here.
	PIIFilterURL string

	// S3 backs résumé storage (internal/blobstore). Optional and provider-agnostic:
	// any S3-compatible endpoint works, and no bucket/host/provider is baked into code —
	// freehire-ops owns those. All four must be set to enable storage; any empty field
	// disables it (résumé upload then only extracts skills in-request, no regression).
	// Enforced at the cmd/server call site, not here.
	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

	// TypstBin is the resolved path to the typst binary used to render CV PDFs
	// (internal/cv). Optional: an empty value disables PDF rendering (the /me/cvs/:id/pdf
	// endpoint returns 501, the rest of the CV builder still works). Resolved via
	// exec.LookPath(TYPST_BIN|"typst") so "disabled" means the binary is genuinely
	// absent, not merely misconfigured — enforced at the cmd/server call site, not here.
	TypstBin string

	// CVEditAllowBulletTruncation restores the old Sanitize behaviour that keeps the
	// first MaxBullets and silently drops the rest. Default false — Commit refuses
	// that class of loss. Set CV_EDIT_ALLOW_BULLET_TRUNCATION=true as an ops kill
	// switch (restart required). Named as the escape hatch so an unset Go bool keeps
	// the safety on.
	CVEditAllowBulletTruncation bool

	// CVMaxBullets is the per-experience / per-project bullet ceiling (cv.MaxBullets).
	// Default 20. Override with CV_MAX_BULLETS; values below 1 fall back to the
	// package default so a typo cannot erase the ceiling.
	CVMaxBullets int

	// MatchAnalysis holds sanitize caps for the fit-analysis LLM output
	// (internal/matchanalysis). Override any field via MATCH_ANALYSIS_*; values
	// below 1 fall back to that field's package default at SetBounds time.
	MatchAnalysis MatchAnalysisSettings

	// TracerLinkSalt keys the visitor hash of a click on a traced CV link. Empty disables the
	// feature's consent toggle outright: without a salt there is no way to tell one visitor from
	// another that is not reversible by walking the address space, and a hash that only looks
	// anonymous is worse than no count at all.
	TracerLinkSalt string

	// Sentry backs optional error reporting for the server and every cron worker
	// (internal/observability). Optional: an empty SentryDSN disables the integration
	// entirely (no init, no delivery) — enforced at the observability call site, not
	// here. SentryEnvironment tags every event (defaults to "development" for local runs).
	SentryDSN         string
	SentryEnvironment string

	// Telegram bot for outbound notifications (filter subscriptions). Optional:
	// an empty TelegramBotToken disables the feature — the linking endpoints and
	// webhook are inert and the notify worker has nothing to deliver through.
	// TelegramBotUsername builds the t.me deep link the SPA shows; its presence
	// is what the public config reports as "enabled". TelegramWebhookSecret is the
	// shared secret verified on the inbound webhook (the bot's secret_token); without
	// it the whole feature stays off, because an empty expected secret would make the
	// webhook admit any caller.
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string

	// Discord bot for slash-command board contributions, mirroring the Telegram bot's
	// role. All four values are required together: the feature is disabled unless
	// DiscordBotToken, DiscordApplicationID, DiscordPublicKey, and DiscordGuildID are
	// ALL set — see newDiscordHandlers/discordEnabled in internal/handler/discord.go.
	// DiscordApplicationID and DiscordPublicKey are the app's identity and the key
	// used to verify inbound interaction signatures. DiscordGuildID scopes the
	// one-time slash-command registration (cmd/discord-register-commands, not
	// cmd/server) to a single guild — that command is the only place commands are
	// ever registered, and it too requires a guild id; there is no global-registration
	// code path.
	DiscordBotToken      string
	DiscordApplicationID string
	DiscordPublicKey     string
	DiscordGuildID       string

	// Email notifications for filter subscriptions, sent via AWS SES by the notify
	// worker. Optional: the email channel is registered only when both AWSRegion and
	// NotifyEmailFrom are set — either empty and the worker still delivers Telegram
	// (enforced at the cmd/notify call site, not here). AWS credentials come from the
	// default AWS chain (env/role), never config. NotifyEmailFrom is the verified SES
	// sender address (e.g. notifications@freehire.me).
	AWSRegion       string
	NotifyEmailFrom string

	// ServedHosts are the exact hostnames this deployment answers on (e.g.
	// "freehire.me,apply.freehire.me"). Only these are honoured as an OAuth redirect
	// origin; anything else falls back to FrontendOrigin. Empty defaults to the
	// frontend origin's own host, so a deployment that never sets it keeps working.
	ServedHosts []string

	// ExtensionRedirectAllowlist bounds where the browser-extension connect flow
	// may hand back a minted token: only https://<id>.chromiumapp.org redirects
	// whose <id> is listed here are accepted. Empty disables the connect flow.
	ExtensionRedirectAllowlist []string
}

// OAuthCredentials is one OAuth provider's credentials. Google/GitHub/LinkedIn
// use only ClientID/ClientSecret; Apple authenticates with a self-signed JWT
// instead of a static secret, so it uses ClientID (its Services ID) plus
// TeamID/KeyID/PrivateKey and leaves ClientSecret empty. PrivateKey is the
// decoded PEM (the OAUTH_APPLE_PRIVATE_KEY env var carries it base64-encoded).
type OAuthCredentials struct {
	ClientID     string
	ClientSecret string
	TeamID       string
	KeyID        string
	PrivateKey   string
}

// MatchAnalysisSettings holds the sanitize ceilings for fit-analysis model output.
// Defaults match internal/matchanalysis.DefaultBounds(); cmd/server applies them via
// matchanalysis.SetBounds at boot. Restart required after any change.
type MatchAnalysisSettings struct {
	// MaxCommentRunes caps each dimension's Comment (default 240).
	MaxCommentRunes int
	// MaxListItemRunes caps each Strengths / Gaps bullet (default 200).
	MaxListItemRunes int
	// MaxRecommendRunes caps the free-text Recommendation (default 1200).
	MaxRecommendRunes int
	// MaxReqTextRunes caps Requirement.Text (default 200).
	MaxReqTextRunes int
	// MaxReqEvidenceRunes caps Requirement.Evidence (default 240).
	MaxReqEvidenceRunes int
	// MaxStrengths caps the Strengths list length (default 6).
	MaxStrengths int
	// MaxGaps caps the Gaps list length (default 6).
	MaxGaps int
	// MaxRequirements caps the RequirementMatch list length (default 30).
	MaxRequirements int
	// MaxSignals caps the HiddenSignals list length (default 5).
	MaxSignals int
	// MaxSignalQuoteRunes caps Signal.Quote (default 200).
	MaxSignalQuoteRunes int
	// MaxSignalInsightRunes caps Signal.Insight (default 200).
	MaxSignalInsightRunes int
}

// oauthProviders are the providers whose credentials Load reads from the
// environment (OAUTH_<PROVIDER>_CLIENT_ID / OAUTH_<PROVIDER>_CLIENT_SECRET,
// plus OAUTH_APPLE_TEAM_ID / OAUTH_APPLE_KEY_ID / OAUTH_APPLE_PRIVATE_KEY for apple).
var oauthProviders = []string{"google", "github", "linkedin", "apple"}

// defaultAssistantMaxPrompt is the rune ceiling on one user message when
// ASSISTANT_MAX_PROMPT is unset or invalid.
const defaultAssistantMaxPrompt = 8000

// Load reads configuration from the environment, falling back to sensible defaults.
func Load() Settings {
	s := Settings{
		Env:                   env("ENV", "development"),
		LLM:                   LoadLLM(),
		Port:                  env("PORT", "8080"),
		MetricsPort:           os.Getenv("METRICS_PORT"),
		DatabaseURL:           env("DATABASE_URL", "postgres://hire:hire@localhost:5432/hire?sslmode=disable"),
		FrontendOrigin:        env("FRONTEND_ORIGIN", "http://localhost:5173"),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		JWTTTL:                envDuration("JWT_TTL", 30*24*time.Hour),
		CookieSecure:          envBool("COOKIE_SECURE", false),
		CookieDomains:         splitDomains(os.Getenv("COOKIE_DOMAIN")),
		OAuth:                 loadOAuth(),
		AuthV2Enabled:         envBool("AUTH_V2_ENABLED", false),
		MobileAuthCallbacks:   parseNamedValues(os.Getenv("MOBILE_AUTH_CALLBACKS")),
		AppleNativeClientID:   strings.TrimSpace(os.Getenv("APPLE_NATIVE_CLIENT_ID")),
		AppleGrantActiveKeyID: strings.TrimSpace(os.Getenv("APPLE_GRANT_ACTIVE_KEY_ID")),
		AppleGrantKeys:        parseKeyRing(os.Getenv("APPLE_GRANT_KEYS")),
		RecentAuthTTL:         envDuration("RECENT_AUTH_TTL", 10*time.Minute),
		GmailTokenKey:         decodeKey(os.Getenv("GMAIL_TOKEN_KEY")),
		MailboxDomain:         os.Getenv("MAILBOX_DOMAIN"),
		MeiliURL:              env("MEILI_URL", "http://localhost:7700"),
		MeiliKey:              os.Getenv("MEILI_MASTER_KEY"),
		RedisURL:              env("REDIS_URL", "redis://localhost:6379/0"),

		LLMAdminURL:         os.Getenv("LLM_ADMIN_URL"),
		LLMAdminKey:         os.Getenv("LLM_ADMIN_KEY"),
		LLMUserMaxBudget:    envFloat("LLM_USER_MAX_BUDGET", 0),
		LLMUserRPMLimit:     envInt("LLM_USER_RPM_LIMIT", 0),
		LLMUserBudgetWindow: env("LLM_USER_BUDGET_WINDOW", "30d"),

		AssistantModel:     os.Getenv("ASSISTANT_MODEL"),
		AssistantMaxSteps:  envInt("ASSISTANT_MAX_STEPS", 0),
		AssistantMaxPrompt: envInt("ASSISTANT_MAX_PROMPT", defaultAssistantMaxPrompt),

		STTModel: os.Getenv("STT_MODEL"),

		RealtimeModel: os.Getenv("REALTIME_MODEL"),

		PIIFilterURL: os.Getenv("PII_FILTER_URL"),

		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),

		TypstBin:                    resolveTypstBin(env("TYPST_BIN", "typst")),
		CVEditAllowBulletTruncation: envBool("CV_EDIT_ALLOW_BULLET_TRUNCATION", false),
		CVMaxBullets:                envInt("CV_MAX_BULLETS", 20),

		MatchAnalysis: MatchAnalysisSettings{
			MaxCommentRunes:       envInt("MATCH_ANALYSIS_MAX_COMMENT_RUNES", 240),
			MaxListItemRunes:      envInt("MATCH_ANALYSIS_MAX_LIST_ITEM_RUNES", 200),
			MaxRecommendRunes:     envInt("MATCH_ANALYSIS_MAX_RECOMMEND_RUNES", 1200),
			MaxReqTextRunes:       envInt("MATCH_ANALYSIS_MAX_REQ_TEXT_RUNES", 200),
			MaxReqEvidenceRunes:   envInt("MATCH_ANALYSIS_MAX_REQ_EVIDENCE_RUNES", 240),
			MaxStrengths:          envInt("MATCH_ANALYSIS_MAX_STRENGTHS", 6),
			MaxGaps:               envInt("MATCH_ANALYSIS_MAX_GAPS", 6),
			MaxRequirements:       envInt("MATCH_ANALYSIS_MAX_REQUIREMENTS", 30),
			MaxSignals:            envInt("MATCH_ANALYSIS_MAX_SIGNALS", 5),
			MaxSignalQuoteRunes:   envInt("MATCH_ANALYSIS_MAX_SIGNAL_QUOTE_RUNES", 200),
			MaxSignalInsightRunes: envInt("MATCH_ANALYSIS_MAX_SIGNAL_INSIGHT_RUNES", 200),
		},

		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryEnvironment: env("SENTRY_ENVIRONMENT", "development"),

		TelegramBotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramBotUsername:   os.Getenv("TELEGRAM_BOT_USERNAME"),
		TelegramWebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),

		DiscordBotToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordApplicationID: os.Getenv("DISCORD_APPLICATION_ID"),
		DiscordPublicKey:     os.Getenv("DISCORD_PUBLIC_KEY"),
		DiscordGuildID:       os.Getenv("DISCORD_GUILD_ID"),

		AWSRegion:       os.Getenv("AWS_REGION"),
		NotifyEmailFrom: os.Getenv("NOTIFY_EMAIL_FROM"),

		ServedHosts:                splitCSV(os.Getenv("SERVED_HOSTS")),
		TracerLinkSalt:             os.Getenv("TRACER_LINK_SALT"),
		ExtensionRedirectAllowlist: splitCSV(os.Getenv("EXTENSION_REDIRECT_ALLOWLIST")),
	}
	if s.AssistantMaxPrompt < 1 {
		s.AssistantMaxPrompt = defaultAssistantMaxPrompt
	}
	return s
}

// Validate checks that the configuration values are safe to boot the auth surface with.
// The JWT secret check runs in every environment, not just production: HS256 security
// rests entirely on secret entropy, so a short secret is brute-forceable offline against
// any captured token regardless of what ENV happens to be set to.
func (s Settings) Validate() error {
	if s.Env == "production" && !s.CookieSecure {
		return errors.New("COOKIE_SECURE must be true in production")
	}
	if strings.HasPrefix(s.FrontendOrigin, "https://") && !s.CookieSecure {
		return errors.New("COOKIE_SECURE must be true when using HTTPS origin")
	}
	if len(s.JWTSecret) < 32 {
		return errors.New("JWT_SECRET is required and must be at least 32 bytes")
	}
	if s.RecentAuthTTL != 0 && (s.RecentAuthTTL < time.Minute || s.RecentAuthTTL > time.Hour) {
		return errors.New("RECENT_AUTH_TTL must be between 1m and 1h")
	}
	appleWeb := s.OAuth["apple"]
	appleFields := []bool{s.AppleNativeClientID != "", s.AppleGrantActiveKeyID != "", len(s.AppleGrantKeys) != 0}
	partialApple := (appleFields[0] || appleFields[1] || appleFields[2]) && !(appleFields[0] && appleFields[1] && appleFields[2])
	if partialApple {
		return errors.New("native Apple requires APPLE_NATIVE_CLIENT_ID, APPLE_GRANT_ACTIVE_KEY_ID, and APPLE_GRANT_KEYS")
	}
	if appleFields[0] {
		if appleWeb.TeamID == "" || appleWeb.KeyID == "" || appleWeb.PrivateKey == "" {
			return errors.New("native Apple requires OAUTH_APPLE_TEAM_ID, OAUTH_APPLE_KEY_ID, and OAUTH_APPLE_PRIVATE_KEY")
		}
		if len(s.AppleGrantKeys[s.AppleGrantActiveKeyID]) != 32 {
			return errors.New("APPLE_GRANT_ACTIVE_KEY_ID must identify a 32-byte key in APPLE_GRANT_KEYS")
		}
	}
	if s.AuthV2Enabled && len(s.MobileAuthCallbacks) == 0 {
		return errors.New("MOBILE_AUTH_CALLBACKS is required when AUTH_V2_ENABLED=true")
	}
	if s.AuthV2Enabled && s.MobileAuthCallbacks["web"] == "" {
		return errors.New("MOBILE_AUTH_CALLBACKS must include web when AUTH_V2_ENABLED=true")
	}
	for name, raw := range s.MobileAuthCallbacks {
		u, err := url.Parse(raw)
		validName := name == "ios" || name == "android" || name == "web"
		if err != nil || !validName || !u.IsAbs() || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errors.New("MOBILE_AUTH_CALLBACKS must contain valid ios, android, or web URLs")
		}
		if s.Env == "production" && u.Scheme != "https" {
			return errors.New("MOBILE_AUTH_CALLBACKS must use HTTPS in production")
		}
	}
	if raw := s.MobileAuthCallbacks["web"]; raw != "" {
		callback, callbackErr := url.Parse(raw)
		frontend, frontendErr := url.Parse(s.FrontendOrigin)
		if callbackErr != nil || frontendErr != nil || callback.Scheme != frontend.Scheme || callback.Host != frontend.Host || callback.Path != "/my/reauth" {
			return errors.New("MOBILE_AUTH_CALLBACKS web must be FRONTEND_ORIGIN/my/reauth")
		}
	}
	return nil
}

func parseNamedValues(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(name) != "" && strings.TrimSpace(value) != "" {
			out[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseKeyRing(raw string) map[string][]byte {
	out := map[string][]byte{}
	for _, part := range strings.Split(raw, ",") {
		id, encoded, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || strings.TrimSpace(id) == "" {
			continue
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			key = nil
		}
		// Preserve malformed named entries so NewKeyRing fails startup instead of
		// silently dropping an old decrypt-only key still referenced by grants.
		out[strings.TrimSpace(id)] = key
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// splitCSV parses a comma-separated env value into trimmed, non-empty entries.
// An empty or all-blank value yields nil.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// decodeKey base64-decodes an AES key, returning nil unless it is exactly 32
// bytes so a missing or malformed key simply disables the feature.
func decodeKey(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil
	}
	return b
}

// decodeBase64 decodes a base64-encoded env value, returning "" for an unset
// or malformed one. Used for OAUTH_APPLE_PRIVATE_KEY: a multi-line PEM does
// not survive a systemd EnvironmentFile reliably, so it is stored
// base64-encoded, the same convention as GMAIL_TOKEN_KEY.
func decodeBase64(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// splitDomains parses a comma-separated COOKIE_DOMAIN into bare registrable
// domains, trimming spaces and any leading dot (".freehire.me" -> "freehire.me").
// Empty entries are dropped, so "" yields nil (host-only cookie, no extra origins).
func splitDomains(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimPrefix(strings.TrimSpace(p), ".")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// resolveTypstBin returns the absolute path to the typst binary named by name, or "" when
// no such binary is on PATH — so a missing binary cleanly disables PDF rendering (501)
// instead of failing every render at exec time (500).
func resolveTypstBin(name string) string {
	if name == "" {
		return ""
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func envDuration(key string, fallback time.Duration) time.Duration {
	// Reuse env()'s "unset or empty -> fallback" rule; an unparseable value
	// also falls back.
	if d, err := time.ParseDuration(env(key, "")); err == nil {
		return d
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	// Same "unset or empty -> fallback" rule as envInt; an unparseable value also
	// falls back, which for a spend ceiling means "no ceiling" rather than "no spend".
	if f, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return f
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	// Same "unset or empty -> fallback" rule; only an explicit "true" enables.
	if b, err := strconv.ParseBool(env(key, "")); err == nil {
		return b
	}
	return fallback
}
