package main

import (
	"cmp"
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
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/database"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/handler"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/llmkey"
	"github.com/strelov1/freehire/internal/observability"
	"github.com/strelov1/freehire/internal/pii"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/speech"
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
		// Buffer for reading the request head (request line + all headers). Fiber's
		// 4KB default is too small for our largest legitimate GET: a filter feed
		// carrying dozens of facets (a "use my whole profile" search is 70+ skills)
		// puts a ~1.3KB query string in BOTH the request line AND the same-page
		// Referer, and on top of that ride the auth cookie and Chrome's sec-ch-ua
		// client hints — together over 4KB, so fasthttp rejected the request with a
		// 431 before it ever reached a handler (the feed then showed "Failed to load
		// jobs"). 16KB comfortably fits even an unusually broad filter set.
		ReadBufferSize: 16 * 1024,
		// Cap request bodies at 8MB: résumé PDF uploads are the largest write (design-heavy
		// CVs run past 1MB), and Fiber's BodyLimit is global — there is no per-route limit —
		// so this ceiling applies to every endpoint. Keep it as tight as the résumé path
		// allows to bound a memory-amplification body. The web client rejects oversize
		// résumés up front so users get a clear message instead of a raw 413. Seam: if the
		// broad ceiling ever matters, add a Content-Length guard middleware on the non-upload
		// routes rather than lowering this.
		BodyLimit:    8 * 1024 * 1024,
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
	// The request log goes to journald and on into /var/log/syslog, never to a terminal, so
	// Fiber's default colours are ANSI escapes written into a file nobody reads in a terminal.
	// They were ~28% of syslog's lines on prod, where the API's per-request log is ~71% of a
	// file that reaches 3.9 GB a week — on a host whose facet reindex refuses to run below a
	// 70 GiB free-space floor it clears by about 1 GiB. They also make the log ungreppable.
	app.Use(logger.New(logger.Config{DisableColors: true}))

	// Sentry request middleware, wired only when error reporting is configured. It
	// sits AFTER recover.New so its deferred capture runs first on a panic (reporting
	// it with request context); Repanic re-raises so recover.New still renders the
	// standard 500 envelope. Non-panic 5xx are reported separately in handler.RenderError.
	if cfg.SentryDSN != "" {
		app.Use(sentryfiber.New(sentryfiber.Options{Repanic: true}))
	}

	// Search is optional: without a Meilisearch key the client stays nil and the
	// search endpoint reports 503, leaving the rest of the API fully functional.
	// The embed options wire the EMBED_* env (CV embedding shares the jobs TEI path).
	var searchClient *search.Client
	if cfg.MeiliKey != "" {
		ec := config.LoadEmbedClient()
		searchClient = search.NewClient(cfg.MeiliURL, cfg.MeiliKey,
			search.WithEmbedURL(ec.URL), search.WithEmbedAPIKey(ec.APIKey), search.WithEmbedConcurrency(ec.Concurrency))
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
	llmClient, llmFlush, err := llm.NewClient(cfg.LLM.Settings(cfg.LLM.Model), "cv-ats")
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	defer llmFlush()

	// The in-app assistant runs on its own model: ASSISTANT_MODEL is chosen for
	// reliable tool calling and a large context, where LLM_MODEL is chosen for cheap
	// one-shot JSON extraction. Unset falls back to LLM_MODEL, so a single-model
	// deployment still works. Traced under its own source label.
	assistantLLM, assistantFlush, err := llm.NewClient(cfg.LLM.Settings(cmp.Or(cfg.AssistantModel, cfg.LLM.Model)), "assistant")
	if err != nil {
		log.Fatalf("llm (assistant): %v", err)
	}
	defer assistantFlush()

	// Dictation runs on the same gateway and the same key as the two clients above —
	// an OpenAI-compatible endpoint serves /chat/completions and /audio/transcriptions
	// alike — so there is nothing extra to configure and nothing extra to fail. Nil
	// when the gateway is unset, which the composer reads as "no microphone here".
	speechClient := speech.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.STTModel)

	// The gateway's administrative API, which mints the per-user credential each
	// account's model calls are spent under. Nil when unconfigured, and that is an
	// ordinary deployment rather than a degraded one: every call then goes out on
	// LLM_API_KEY exactly as it did before this existed. It is deliberately a separate
	// endpoint and credential from inference — administration is served at the gateway
	// root, and the key that mints keys has no business on a chat request.
	llmKeys := llmkey.New(llmkey.Config{
		BaseURL:      cfg.LLMAdminURL,
		AdminKey:     cfg.LLMAdminKey,
		MaxBudget:    cfg.LLMUserMaxBudget,
		RPMLimit:     cfg.LLMUserRPMLimit,
		BudgetWindow: cfg.LLMUserBudgetWindow,
	})

	// OAuth sign-in is optional: only providers with full credentials are
	// enabled; the registry may be empty and the server still serves password
	// auth. Redirect URLs are built per request from the request origin, so one
	// deployment can complete the flow on more than one served domain.
	oauthRegistry := oauth.NewRegistry(cfg.OAuth)

	// Connect-Gmail inbox: enabled only when the Google OAuth client and the
	// token-encryption key are configured. Both nil disables the feature.
	gmailConnector, gmailCipher := buildGmail(cfg)

	// PII detector for de-identifying CV text before it reaches the LLM. Nil when
	// PII_FILTER_URL is unset, which fails the CV→LLM paths closed (no analysis) rather
	// than leaking PII (see internal/pii, internal/matchanalysis, internal/resumeextract).
	var piiDetector pii.Detector
	if cfg.PIIFilterURL != "" {
		piiDetector = pii.NewHTTPDetector(cfg.PIIFilterURL, nil)
	}

	// Credits metering config, loaded alongside the other optional dependencies rather
	// than inline in the handler registration.
	creditsConfig := credits.Config(config.LoadCredits())

	handler.Register(app, handler.Config{
		Pool:              pool,
		FrontendOrigin:    cfg.FrontendOrigin,
		JWTSecret:         cfg.JWTSecret,
		JWTTTL:            cfg.JWTTTL,
		CookieSecure:      cfg.CookieSecure,
		CookieDomains:     cfg.CookieDomains,
		OAuthRegistry:     oauthRegistry,
		GmailConnector:    gmailConnector,
		GmailCipher:       gmailCipher,
		MailboxDomain:     cfg.MailboxDomain,
		Search:            searchClient,
		Blob:              blobStore,
		TypstBin:          cfg.TypstBin,
		TracerLinkSalt:    cfg.TracerLinkSalt,
		LLM:               llmClient,
		AssistantLLM:      assistantLLM,
		AssistantMaxSteps: cfg.AssistantMaxSteps,
		LLMKeys:           llmKeys,
		Speech:            speechClient,
		PIIDetector:       piiDetector,

		TelegramBotToken:      cfg.TelegramBotToken,
		TelegramBotUsername:   cfg.TelegramBotUsername,
		TelegramWebhookSecret: cfg.TelegramWebhookSecret,
		ServedHosts:           cfg.ServedHosts,

		DiscordBotToken:      cfg.DiscordBotToken,
		DiscordApplicationID: cfg.DiscordApplicationID,
		DiscordPublicKey:     cfg.DiscordPublicKey,
		DiscordGuildID:       cfg.DiscordGuildID,

		Credits: creditsConfig,

		AWSRegion:       cfg.AWSRegion,
		NotifyEmailFrom: cfg.NotifyEmailFrom,

		ExtensionRedirectAllowlist: cfg.ExtensionRedirectAllowlist,
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
		// A configured-but-broken key must not silently disable the feature.
		log.Printf("gmail: token cipher init failed, Connect-Gmail disabled: %v", err)
		return nil, nil
	}
	return gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin), cipher
}
