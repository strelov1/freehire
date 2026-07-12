package config

import (
	"encoding/base64"
	"os"
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
	// as-is and the server fails fast when it is empty (see cmd/server). The
	// enrich worker shares Load but ignores these, so the requirement lives at
	// the server entry point, not here.
	JWTSecret string
	JWTTTL    time.Duration

	// CookieSecure marks the auth cookie Secure (HTTPS-only). Default false so
	// the cookie works over http://localhost in dev; set COOKIE_SECURE=true in
	// any HTTPS deployment.
	CookieSecure bool

	// CookieDomain scopes the session cookie via its Domain attribute. Empty in
	// dev (host-only). In prod set COOKIE_DOMAIN=.freehire.dev so a session minted
	// on freehire.dev is also sent to apply.freehire.dev (unified SSO).
	CookieDomain string

	// OAuth holds per-provider client credentials keyed by provider name
	// (google, github, linkedin). OAuth sign-in is optional: a provider with
	// incomplete credentials is simply disabled (enforced where the provider
	// registry is built, not here), and the server starts either way.
	OAuth map[string]OAuthCredentials

	// GmailTokenKey is the 32-byte AES key (from GMAIL_TOKEN_KEY, base64) that
	// encrypts stored Gmail refresh tokens. Empty/invalid = the Connect-Gmail
	// inbox feature is disabled (like an unset optional integration).
	GmailTokenKey []byte

	// Meilisearch backs the job search endpoint and the reindex command. Shared
	// via Load (both cmd/server and cmd/reindex read it). Search is optional:
	// MeiliKey empty ⇒ search is disabled and the server still starts (see
	// cmd/server), so the requirement is enforced at the call site, not here.
	MeiliURL string
	MeiliKey string

	// LLM backs the optional CV ATS qualitative review on the HTTP server (the enrich
	// worker reads its own LLM settings via config.Enrich). Optional and provider-
	// agnostic: any empty field disables the AI layer — the server builds no LLM client
	// and the ATS score stays deterministic (enforced at the cmd/server call site, not
	// here). Langfuse tracing is optional observability, wired only when all three set.
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	LangfuseBaseURL   string
	LangfusePublicKey string
	LangfuseSecretKey string

	// S3 backs résumé storage (internal/blobstore). Optional and provider-agnostic:
	// any S3-compatible endpoint works, and no bucket/host/provider is baked into code —
	// freehire-ops owns those. All four must be set to enable storage; any empty field
	// disables it (résumé upload then only extracts skills in-request, no regression).
	// Enforced at the cmd/server call site, not here.
	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

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
	// shared secret verified on the inbound webhook (the bot's secret_token).
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string

	// Email notifications for filter subscriptions, sent via AWS SES by the notify
	// worker. Optional: the email channel is registered only when both AWSRegion and
	// NotifyEmailFrom are set — either empty and the worker still delivers Telegram
	// (enforced at the cmd/notify call site, not here). AWS credentials come from the
	// default AWS chain (env/role), never config. NotifyEmailFrom is the verified SES
	// sender address (e.g. notifications@freehire.dev).
	AWSRegion       string
	NotifyEmailFrom string
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
		Port:           env("PORT", "8080"),
		DatabaseURL:    env("DATABASE_URL", "postgres://hire:hire@localhost:5432/hire?sslmode=disable"),
		FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:5173"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTTTL:         envDuration("JWT_TTL", 30*24*time.Hour),
		CookieSecure:   envBool("COOKIE_SECURE", false),
		CookieDomain:   os.Getenv("COOKIE_DOMAIN"),
		OAuth:          loadOAuth(),
		GmailTokenKey:  decodeKey(os.Getenv("GMAIL_TOKEN_KEY")),
		MeiliURL:       env("MEILI_URL", "http://localhost:7700"),
		MeiliKey:       os.Getenv("MEILI_MASTER_KEY"),

		LLMBaseURL: os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:  os.Getenv("LLM_API_KEY"),
		LLMModel:   os.Getenv("LLM_MODEL"),

		LangfuseBaseURL:   os.Getenv("LANGFUSE_BASE_URL"),
		LangfusePublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
		LangfuseSecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),

		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),

		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryEnvironment: env("SENTRY_ENVIRONMENT", "development"),

		TelegramBotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramBotUsername:   os.Getenv("TELEGRAM_BOT_USERNAME"),
		TelegramWebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),

		AWSRegion:       os.Getenv("AWS_REGION"),
		NotifyEmailFrom: os.Getenv("NOTIFY_EMAIL_FROM"),
	}
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

func envDuration(key string, fallback time.Duration) time.Duration {
	// Reuse env()'s "unset or empty -> fallback" rule; an unparseable value
	// also falls back.
	if d, err := time.ParseDuration(env(key, "")); err == nil {
		return d
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
