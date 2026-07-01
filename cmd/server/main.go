package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/database"
	"github.com/strelov1/freehire/internal/handler"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/search"
)

func main() {
	cfg := config.Load()

	// Never boot the auth surface with a guessable signing key. HS256 security rests
	// entirely on secret entropy, so a short secret is brute-forceable offline against
	// any captured token; require at least 32 bytes.
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("config: JWT_SECRET is required and must be at least 32 bytes")
	}

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

	app.Use(recover.New())
	app.Use(logger.New())

	// Search is optional: without a Meilisearch key the client stays nil and the
	// search endpoint reports 503, leaving the rest of the API fully functional.
	var searchClient *search.Client
	if cfg.MeiliKey != "" {
		searchClient = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	// The LLM is optional: only when all three settings are present do we build a
	// client for the résumé-verdict coherence analysis. Absent it, the client stays
	// nil and the verdict serves its deterministic core (no coherence). A build error
	// on a configured endpoint is fatal — a misconfigured gateway should not boot silently.
	var llmClient *llm.Client
	if cfg.LLMBaseURL != "" && cfg.LLMAPIKey != "" && cfg.LLMModel != "" {
		llmClient, err = llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
		if err != nil {
			log.Fatalf("llm: %v", err)
		}
	}

	// OAuth sign-in is optional: only providers with full credentials are
	// enabled; the registry may be empty and the server still serves password
	// auth. Redirect URLs derive from the same-origin frontend origin.
	oauthProviders := oauth.NewRegistry(cfg.FrontendOrigin, cfg.OAuth)

	handler.Register(app, handler.Config{
		Pool:           pool,
		FrontendOrigin: cfg.FrontendOrigin,
		JWTSecret:      cfg.JWTSecret,
		JWTTTL:         cfg.JWTTTL,
		CookieSecure:   cfg.CookieSecure,
		OAuthProviders: oauthProviders,
		Search:         searchClient,
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
