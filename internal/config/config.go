package config

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Settings holds application configuration read from environment variables.
type Settings struct {
	Port           string
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

	// GmailTokenKey is the 32-byte AES key (from GMAIL_TOKEN_KEY, base64) that
	// encrypts stored Gmail refresh tokens. Empty/invalid = the Connect-Gmail
	// inbox feature is disabled (like an unset optional integration).
	GmailTokenKey []byte

	// MailboxDomain is the receiving domain hosted mailboxes live on
	// (<handle>@MailboxDomain, e.g. inbox.freehire.me). Empty = the hosted-mailbox
	// option is off (claim route unregistered, status reports unavailable). It is
	// deliberately NOT named MAIL_DOMAIN — that shared var already holds the SES
	// *sending* identity (notifications), a different, non-receiving domain.
	MailboxDomain string

	// Meilisearch backs the job search endpoint and the reindex command. Shared
	// via Load (both cmd/server and cmd/reindex read it). Search is optional:
	// MeiliKey empty ⇒ search is disabled and the server still starts (see
	// cmd/server), so the requirement is enforced at the call site, not here.
	MeiliURL string
	MeiliKey string

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

	// STTModel transcribes dictated audio, through the same gateway and key the LLM
	// settings above name — an OpenAI-compatible endpoint serves /chat/completions and
	// /audio/transcriptions alike. Named explicitly like every other model here, with
	// no default: transcription is billed per minute of audio against an assistant that
	// is not metered, so a deployment that never said which model to use should not
	// find itself paying for one. Empty leaves the endpoint answering 501 and the
	// composer rendering no microphone.
	STTModel string

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

// OAuthCredentials is one OAuth provider's client id/secret pair.
type OAuthCredentials struct {
	ClientID     string
	ClientSecret string
}

// oauthProviders are the providers whose credentials Load reads from the
// environment (OAUTH_<PROVIDER>_CLIENT_ID / OAUTH_<PROVIDER>_CLIENT_SECRET).
var oauthProviders = []string{"google", "github", "linkedin"}

// Load reads configuration from the environment, falling back to sensible defaults.
func Load() Settings {
	return Settings{
		LLM:            LoadLLM(),
		Port:           env("PORT", "8080"),
		DatabaseURL:    env("DATABASE_URL", "postgres://hire:hire@localhost:5432/hire?sslmode=disable"),
		FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:5173"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTTTL:         envDuration("JWT_TTL", 30*24*time.Hour),
		CookieSecure:   envBool("COOKIE_SECURE", false),
		CookieDomains:  splitDomains(os.Getenv("COOKIE_DOMAIN")),
		OAuth:          loadOAuth(),
		GmailTokenKey:  decodeKey(os.Getenv("GMAIL_TOKEN_KEY")),
		MailboxDomain:  os.Getenv("MAILBOX_DOMAIN"),
		MeiliURL:       env("MEILI_URL", "http://localhost:7700"),
		MeiliKey:       os.Getenv("MEILI_MASTER_KEY"),

		LLMAdminURL:         os.Getenv("LLM_ADMIN_URL"),
		LLMAdminKey:         os.Getenv("LLM_ADMIN_KEY"),
		LLMUserMaxBudget:    envFloat("LLM_USER_MAX_BUDGET", 0),
		LLMUserRPMLimit:     envInt("LLM_USER_RPM_LIMIT", 0),
		LLMUserBudgetWindow: env("LLM_USER_BUDGET_WINDOW", "30d"),

		AssistantModel:    os.Getenv("ASSISTANT_MODEL"),
		AssistantMaxSteps: envInt("ASSISTANT_MAX_STEPS", 0),

		STTModel: os.Getenv("STT_MODEL"),

		PIIFilterURL: os.Getenv("PII_FILTER_URL"),

		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),

		TypstBin: resolveTypstBin(env("TYPST_BIN", "typst")),

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
