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

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/ai/speech"
	"github.com/strelov1/freehire/internal/api/handler"
	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/api/realtime"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/pii"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/database"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
	"github.com/strelov1/freehire/internal/search/search"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	cv.SetMaxBullets(cfg.CVMaxBullets)
	matchanalysis.SetBounds(matchanalysis.Bounds{
		MaxCommentRunes:       cfg.MatchAnalysis.MaxCommentRunes,
		MaxListItemRunes:      cfg.MatchAnalysis.MaxListItemRunes,
		MaxRecommendRunes:     cfg.MatchAnalysis.MaxRecommendRunes,
		MaxReqTextRunes:       cfg.MatchAnalysis.MaxReqTextRunes,
		MaxReqEvidenceRunes:   cfg.MatchAnalysis.MaxReqEvidenceRunes,
		MaxStrengths:          cfg.MatchAnalysis.MaxStrengths,
		MaxGaps:               cfg.MatchAnalysis.MaxGaps,
		MaxRequirements:       cfg.MatchAnalysis.MaxRequirements,
		MaxSignals:            cfg.MatchAnalysis.MaxSignals,
		MaxSignalQuoteRunes:   cfg.MatchAnalysis.MaxSignalQuoteRunes,
		MaxSignalInsightRunes: cfg.MatchAnalysis.MaxSignalInsightRunes,
	})

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
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

	// Redis is a required dependency, like Postgres — it backs the shared rate
	// limiter (internal/api/ratelimit) with no in-memory fallback mode. A malformed
	// REDIS_URL is fatal at startup rather than degrading rate limiting silently.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer func() { _ = redisClient.Close() }()
	throttler := ratelimit.NewRedisThrottler(redisClient)

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
		BodyLimit: 8 * 1024 * 1024,
		// Wrapped so an error-derived response is counted with the status actually sent:
		// Fiber renders errors here, after the middleware chain has unwound, so the counter
		// in observability.HTTPMetrics cannot see them.
		ErrorHandler: observability.CountErrors(handler.RenderError),
		// The app sits behind the in-network nginx proxy (web/nginx.conf). Key c.IP()
		// (and thus the rate limiter) on X-Real-IP, which nginx OVERWRITES with the real
		// peer — a single value the client cannot spoof. X-Forwarded-For is deliberately
		// NOT used here: Fiber returns the whole (client-appendable) XFF list, so a
		// spoofed prefix would mint a fresh limiter key per request and evade the limit.
		// The header is only honoured when the immediate peer is a trusted private range
		// (the nginx container); a direct public caller is not trusted.
		ProxyHeader:             "X-Real-IP", // Fiber has no constant for this header
		EnableTrustedProxyCheck: true,
		// Shared with internal/api/ratelimit, which does not count a peer we trust to
		// assert someone else's address — chiefly our own SSR, which reaches the
		// API over loopback. One definition so the two cannot disagree.
		TrustedProxies: ratelimit.TrustedCIDRs,
	})

	// Counting responses sits OUTSIDE recover.New on purpose: recover converts a panic into a
	// 500 further in, so only a middleware the response passes through on its way back out
	// records the status the client actually got.
	app.Use(observability.HTTPMetrics())

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
	// search endpoint reports 503, leaving the rest of the API fully functional. No
	// WithEmbed* options: the HTTP server's search client only ever runs plain
	// keyword/facet queries — nothing reachable from it embeds via TEI (the CV-embedding
	// write path and the Meili semantic index it fed were both removed, see
	// openspec/changes/drop-hybrid-search-pgvector-similar).
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
	llmClient, llmFlush, err := llm.NewClient(cfg.Settings(cfg.Model), "cv-ats")
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	defer llmFlush()

	// The in-app assistant runs on its own model: ASSISTANT_MODEL is chosen for
	// reliable tool calling and a large context, where LLM_MODEL is chosen for cheap
	// one-shot JSON extraction. Unset falls back to LLM_MODEL, so a single-model
	// deployment still works. Traced under its own source label.
	assistantLLM, assistantFlush, err := llm.NewClient(cfg.Settings(cmp.Or(cfg.AssistantModel, cfg.Model)), "assistant")
	if err != nil {
		log.Fatalf("llm (assistant): %v", err)
	}
	defer assistantFlush()

	// The AI filter runs on its own model too, for the opposite reason to the
	// assistant's: the task is trivial — sort one sentence into named fields — but a
	// person is watching a spinner while it happens, so latency is the whole quality
	// bar. Measured against this gateway on the real prompt and schema, a small fast
	// model answers in ~2.3s where the shared one takes ~4s and spikes, at the same
	// accuracy. Unset falls back to LLM_MODEL.
	intentLLM, intentFlush, err := llm.NewClient(cfg.Settings(cmp.Or(cfg.SearchIntentModel, cfg.Model)), "search-intent")
	if err != nil {
		log.Fatalf("llm (search intent): %v", err)
	}
	defer intentFlush()

	// Dictation runs on the same gateway and the same key as the two clients above —
	// an OpenAI-compatible endpoint serves /chat/completions and /audio/transcriptions
	// alike — so there is nothing extra to configure and nothing extra to fail. Nil
	// when the gateway is unset, which the composer reads as "no microphone here".
	speechClient := speech.New(cfg.BaseURL, cfg.APIKey, cfg.STTModel)

	// Voice mode's Realtime credential rides the same gateway and key as the clients
	// above — the litellm proxy already fronts OpenAI's /v1/realtime/client_secrets
	// the same way it fronts /audio/transcriptions, so nothing new to configure here
	// either. Nil when the gateway or REALTIME_MODEL is unset, which the interview
	// session view reads as "no voice mode here".
	realtimeClient := realtime.New(cfg.BaseURL, cfg.APIKey, cfg.RealtimeModel)

	// The gateway's administrative API, which mints the per-user credential each
	// account's model calls are spent under. Nil when unconfigured, and that is an
	// ordinary deployment rather than a degraded one: every call then goes out on
	// LLM_API_KEY exactly as it did before this existed. It is deliberately a separate
	// endpoint and credential from inference — administration is served under /api, and
	// the administrator that mints keys has no business on a chat request.
	llmKeys := llmkey.New(llmkey.Config{
		BaseURL:       cfg.LLMAdminURL,
		AdminUsername: cfg.LLMAdminUsername,
		AdminPassword: cfg.LLMAdminPassword,
		TemplateKey:   cfg.LLMAdminTemplateKey,
		MaxBudget:     cfg.LLMUserMaxBudget,
		RPMLimit:      cfg.LLMUserRPMLimit,
		BudgetWindow:  cfg.LLMUserBudgetWindow,
	})

	// OAuth sign-in is optional: only providers with full credentials are
	// enabled; the registry may be empty and the server still serves password
	// auth. Redirect URLs are built per request from the request origin, so one
	// deployment can complete the flow on more than one served domain.
	oauthRegistry := oauth.NewRegistry(cfg.OAuth)
	var appleNative *appleauth.Client
	var appleGrantKeys *appleauth.KeyRing
	if cfg.AppleNativeClientID != "" {
		appleCreds := cfg.OAuth["apple"]
		appleNative, err = appleauth.New(appleauth.Config{TeamID: appleCreds.TeamID, KeyID: appleCreds.KeyID, PrivateKeyPEM: appleCreds.PrivateKey, ClientIDs: []string{cfg.AppleNativeClientID}})
		if err != nil {
			log.Fatalf("native Apple: %v", err)
		}
		appleGrantKeys, err = appleauth.NewKeyRing(cfg.AppleGrantActiveKeyID, cfg.AppleGrantKeys)
		if err != nil {
			log.Fatalf("native Apple grant encryption: %v", err)
		}
	}

	// Connect-Gmail inbox: enabled only when the Google OAuth client and the
	// token-encryption key are configured. Both nil disables the feature.
	gmailConnector, gmailCipher := buildGmail(cfg)

	// PII detector for de-identifying CV text before it reaches the LLM. Nil when
	// PII_FILTER_URL is unset, which fails the CV→LLM paths closed (no analysis) rather
	// than leaking PII (see internal/candidate/pii, internal/candidate/matchanalysis, internal/candidate/resumeextract).
	var piiDetector pii.Detector
	if cfg.PIIFilterURL != "" {
		piiDetector = pii.NewHTTPDetector(cfg.PIIFilterURL, nil)
	}

	// Credits metering config, loaded alongside the other optional dependencies rather
	// than inline in the handler registration.
	planConfig := plan.ConfigFromEnv()

	handler.Register(app, handler.Config{
		Pool:                        pool,
		Throttler:                   throttler,
		Cache:                       cache.NewRedisCache(redisClient),
		FrontendOrigin:              cfg.FrontendOrigin,
		JWTSecret:                   cfg.JWTSecret,
		JWTTTL:                      cfg.JWTTTL,
		CookieSecure:                cfg.CookieSecure,
		CookieDomains:               cfg.CookieDomains,
		OAuthRegistry:               oauthRegistry,
		AuthV2Enabled:               cfg.AuthV2Enabled,
		MobileAuthCallbacks:         cfg.MobileAuthCallbacks,
		RecentAuthTTL:               cfg.RecentAuthTTL,
		AppleNative:                 appleNative,
		AppleGrantKeys:              appleGrantKeys,
		GmailConnector:              gmailConnector,
		GmailCipher:                 gmailCipher,
		MailboxDomain:               cfg.MailboxDomain,
		Search:                      searchClient,
		Blob:                        blobStore,
		TypstBin:                    cfg.TypstBin,
		CVEditAllowBulletTruncation: cfg.CVEditAllowBulletTruncation,
		TracerLinkSalt:              cfg.TracerLinkSalt,
		LLM:                         llmClient,
		AssistantLLM:                assistantLLM,
		SearchIntentLLM:             intentLLM,
		AssistantMaxSteps:           cfg.AssistantMaxSteps,
		AssistantMaxPrompt:          cfg.AssistantMaxPrompt,
		LLMKeys:                     llmKeys,
		Speech:                      speechClient,
		Realtime:                    realtimeClient,
		PIIDetector:                 piiDetector,

		TelegramBotToken:      cfg.TelegramBotToken,
		TelegramBotUsername:   cfg.TelegramBotUsername,
		TelegramWebhookSecret: cfg.TelegramWebhookSecret,
		ServedHosts:           cfg.ServedHosts,

		DiscordBotToken:      cfg.DiscordBotToken,
		DiscordApplicationID: cfg.DiscordApplicationID,
		DiscordPublicKey:     cfg.DiscordPublicKey,
		DiscordGuildID:       cfg.DiscordGuildID,

		Plan: planConfig,

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

	observability.StartMetricsServer(cfg.MetricsPort)

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
