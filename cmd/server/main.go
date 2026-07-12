package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/database"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/handler"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/observability"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/tokencrypt"
)

func main() {
	cfg := config.Load()

	// Never boot the auth surface with a guessable signing key. HS256 security rests
	// entirely on secret entropy, so a short secret is brute-forceable offline against
	// any captured token; require at least 32 bytes.
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("config: JWT_SECRET is required and must be at least 32 bytes")
	}

	// Error reporting is optional and env-gated: no DSN ⇒ Init is a no-op. A
	// malformed DSN is fatal — a misconfigured gateway must not boot silently.
	sentryFlush, err := observability.Init(cfg.SentryDSN, cfg.SentryEnvironment)
	if err != nil {
		log.Fatalf("sentry: %v", err)
	}
	defer sentryFlush()

	// One signal-bound context drives both startup and shutdown: it cancels the pool
	// connect if a signal arrives mid-startup, and its Done channel is the shutdown
	// trigger below.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	app := fiber.New(fiber.Config{
		AppName:      "hire",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		// Cap request bodies well under Fiber's 4MB default: the largest write is a
		// single moderator job description, so 1MB bounds a memory-amplification body.
		BodyLimit:    1 * 1024 * 1024,
		ErrorHandler: handler.RenderError,
		// The app sits behind the in-network nginx proxy (web/nginx.conf). Key c.IP()
		// (and thus the rate limiter) on X-Real-IP, which nginx OVERWRITES with the real
		// peer — a single value the client cannot spoof. X-Forwarded-For is deliberately
		// NOT used here: Fiber returns the whole (client-appendable) XFF list, so a
		// spoofed prefix would mint a fresh limiter key per request and evade the limit.
		// The header is only honoured when the immediate peer is a trusted private range
		// (the nginx container); a direct public caller is not trusted.
		ProxyHeader:             "X-Real-IP", // Fiber has no constant for this header
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.1/32"},
	})

	// The recover middleware marks each unwound panic via c.Locals so RenderError
	// won't double-report it: the sentryfiber middleware below already captures the
	// panic (with a stack) before Fiber re-delivers the recovered error to the
	// ErrorHandler. The silent handler keeps the previous no-stderr-stack behavior.
	app.Use(recover.New(recover.Config{
		EnableStackTrace:  true,
		StackTraceHandler: func(c *fiber.Ctx, _ any) { c.Locals(handler.LocalPanicReported, true) },
	}))
	app.Use(logger.New())

	// Sentry request middleware, wired only when error reporting is configured. It
	// sits AFTER recover.New so its deferred capture runs first on a panic (reporting
	// it with request context); Repanic re-raises so recover.New still renders the
	// standard 500 envelope. Non-panic 5xx are reported separately in handler.RenderError.
	if cfg.SentryDSN != "" {
		app.Use(sentryfiber.New(sentryfiber.Options{Repanic: true}))
	}

	// Search is optional: without a Meilisearch key the client stays nil and the
	// search endpoint reports 503, leaving the rest of the API fully functional.
	var searchClient *search.Client
	if cfg.MeiliKey != "" {
		searchClient = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	// Résumé storage is optional: only when all four S3 settings are present does
	// blobstore.New build a client (it returns nil otherwise). Nil disables storage —
	// résumé upload still extracts skills in-request and the verdict falls back to a
	// per-request upload. A build error on a configured endpoint is fatal.
	blobStore, err := blobstore.New(blobstore.Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		log.Fatalf("blobstore: %v", err)
	}

	// The LLM is optional and built through the shared construction path
	// (llm.NewClient): it powers the CV ATS qualitative review, wires Langfuse
	// tracing (source "cv-ats") when LANGFUSE_* are set, and returns a flush func for
	// shutdown. Nil client when unconfigured — the ATS score stays deterministic. A
	// build error on a configured endpoint is fatal (a misconfigured gateway must not
	// boot silently).
	llmClient, llmFlush, err := llm.NewClient(llm.Settings{
		BaseURL:           cfg.LLMBaseURL,
		APIKey:            cfg.LLMAPIKey,
		Model:             cfg.LLMModel,
		LangfuseBaseURL:   cfg.LangfuseBaseURL,
		LangfusePublicKey: cfg.LangfusePublicKey,
		LangfuseSecretKey: cfg.LangfuseSecretKey,
	}, "cv-ats")
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	defer llmFlush()

	// OAuth sign-in is optional: only providers with full credentials are
	// enabled; the registry may be empty and the server still serves password
	// auth. Redirect URLs derive from the same-origin frontend origin.
	oauthProviders := oauth.NewRegistry(cfg.FrontendOrigin, cfg.OAuth)

	// Connect-Gmail inbox: enabled only when the Google OAuth client and the
	// token-encryption key are configured. Both nil disables the feature.
	gmailConnector, gmailCipher := buildGmail(cfg)

	handler.Register(app, handler.Config{
		Pool:           pool,
		FrontendOrigin: cfg.FrontendOrigin,
		JWTSecret:      cfg.JWTSecret,
		JWTTTL:         cfg.JWTTTL,
		CookieSecure:   cfg.CookieSecure,
		CookieDomain:   cfg.CookieDomain,
		OAuthProviders: oauthProviders,
		GmailConnector: gmailConnector,
		GmailCipher:    gmailCipher,
		Search:         searchClient,
		Blob:           blobStore,
		LLM:            llmClient,

		TelegramBotToken:      cfg.TelegramBotToken,
		TelegramBotUsername:   cfg.TelegramBotUsername,
		TelegramWebhookSecret: cfg.TelegramWebhookSecret,
	})

	// Run the server in a goroutine so main can wait for a shutdown signal.
	// Fiber's Listen returns nil on graceful shutdown, so any error is fatal.
	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()
	log.Printf("hire listening on :%s", cfg.Port)

	// Graceful shutdown on SIGINT/SIGTERM: block until the signal-bound context is
	// cancelled (the signal arrived) or startup cancelled it.
	<-ctx.Done()
	log.Println("shutting down...")

	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// buildGmail wires the Connect-Gmail inbox from config: it needs the Google OAuth
// client (reused from sign-in) and the 32-byte token-encryption key. Any piece
// missing returns (nil, nil) — the feature stays off and the server runs unchanged.
func buildGmail(cfg config.Settings) (*gmailsync.Connector, *tokencrypt.Cipher) {
	g := cfg.OAuth["google"]
	if g.ClientID == "" || g.ClientSecret == "" || len(cfg.GmailTokenKey) != 32 {
		return nil, nil
	}
	cipher, err := tokencrypt.New(cfg.GmailTokenKey)
	if err != nil {
		return nil, nil
	}
	return gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin), cipher
}
