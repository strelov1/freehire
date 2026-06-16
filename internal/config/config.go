package config

import (
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

	// OAuth holds per-provider client credentials keyed by provider name
	// (google, github, linkedin). OAuth sign-in is optional: a provider with
	// incomplete credentials is simply disabled (enforced where the provider
	// registry is built, not here), and the server starts either way.
	OAuth map[string]OAuthCredentials

	// Meilisearch backs the job search endpoint and the reindex command. Shared
	// via Load (both cmd/server and cmd/reindex read it). Search is optional:
	// MeiliKey empty ⇒ search is disabled and the server still starts (see
	// cmd/server), so the requirement is enforced at the call site, not here.
	MeiliURL string
	MeiliKey string

	// Telegram bot for outbound notifications (filter subscriptions). Optional:
	// an empty TelegramBotToken disables the feature — the linking endpoints and
	// webhook are inert and the notify worker has nothing to deliver through.
	// TelegramBotUsername builds the t.me deep link the SPA shows; its presence
	// is what the public config reports as "enabled". TelegramWebhookSecret is the
	// shared secret verified on the inbound webhook (the bot's secret_token).
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string
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
		JWTTTL:         envDuration("JWT_TTL", 24*time.Hour),
		CookieSecure:   envBool("COOKIE_SECURE", false),
		OAuth:          loadOAuth(),
		MeiliURL:       env("MEILI_URL", "http://localhost:7700"),
		MeiliKey:       os.Getenv("MEILI_MASTER_KEY"),

		TelegramBotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramBotUsername:   os.Getenv("TELEGRAM_BOT_USERNAME"),
		TelegramWebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
	}
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
