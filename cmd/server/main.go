package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	// The mentorship slot engine resolves IANA zone names at request time, so a missing
	// zone database would silently label every mentor's schedule UTC — a wrong answer
	// shaped exactly like a right one. The runtime image ships tzdata today; this makes
	// the binary independent of that.
	_ "time/tzdata"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	recovermiddleware "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/ai/speech"
	"github.com/strelov1/freehire/internal/api/handler"
	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/api/realtime"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/identity/apiusage"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/job/recentfeed"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/database"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/search/embedprogress"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"

	"github.com/redis/go-redis/v9"
)

func main() { os.Exit(runMain()) }

func runMain() int {
	cfg := config.Load()
	shutdown, err := observability.InitLogs("freehire-api")
	if err != nil {
		log.Printf("exception reporting: %v", err)
		return 1
	}
	defer shutdown()
	defer func() {
		if value := recover(); value != nil {
			observability.CapturePanic(context.Background(), value)
			panic(value)
		}
	}()
	if err := runServer(cfg); err != nil {
		observability.CaptureError(context.Background(), err)
		log.Print(err)
		return 1
	}
	return 0
}

func runServer(cfg config.Settings) error {
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
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// One signal-bound context drives both startup and shutdown: it cancels the pool
	// connect if a signal arrives mid-startup, and its Done channel is the shutdown
	// trigger below.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, database.WithStatementTimeout(apiStatementTimeout))
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	// Redis backs the shared application caches.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer func() { _ = redisClient.Close() }()

	app := fiber.New(fiber.Config{
		AppName:      "hire",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		// Buffer for the request head. Fiber's 4KB default is too small for our largest legitimate GET:
		// a broad filter feed puts a ~1.3KB query in both the request line and the Referer, plus the
		// auth cookie and client hints, and fasthttp rejected it with 431. 16KB fits.
		ReadBufferSize: 16 * 1024,
		// Cap request bodies at 8MB: CV PDF uploads are the largest write (design-heavy ones pass 1MB)
		// and Fiber's BodyLimit is global, so this ceiling applies to every endpoint. The web client
		// rejects oversize CVs up front. Seam: if the broad ceiling ever matters, add a Content-Length
		// guard on the non-upload routes rather than lowering this.
		BodyLimit: 8 * 1024 * 1024,
		// Wrapped so an error-derived response is counted with the status actually sent:
		// Fiber renders errors here, after the middleware chain has unwound, so the counter
		// in observability.HTTPMetrics cannot see them.
		ErrorHandler: observability.CountErrors(handler.RenderError),
		// Only private reverse proxies may assert the visitor's address.
		ProxyHeader:             "X-Real-IP",
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.1/32"},
	})

	// Counting responses sits OUTSIDE recover.New on purpose: recover converts a panic into a
	// 500 further in, so only a middleware the response passes through on its way back out
	// records the status the client actually got.
	app.Use(observability.HTTPMetrics())
	app.Use(observability.RequestIDs())

	// Report the panic while its stack is available and mark it so RenderError cannot report it a
	// second time. The callback preserves the no-stderr-stack behavior.
	app.Use(recovermiddleware.New(recovermiddleware.Config{
		EnableStackTrace:  true,
		StackTraceHandler: handler.ReportPanic,
	}))
	// The request log goes to journald and syslog, never a terminal, so Fiber's default ANSI
	// colours are noise in a file and make it ungreppable.
	app.Use(logger.New(logger.Config{DisableColors: true,
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${error} | request_id=${locals:diagnostics.request_id}\n",
	}))

	// Search is optional: without a Meilisearch key the client stays nil and the search endpoint
	// reports 503. No WithEmbed* options: this server only runs plain keyword/facet queries.
	var searchClient *search.Client
	// suggestSvc answers the search box's completions. It carries an in-process
	// recognition set — the phrases a visitor may have already typed — which is why it
	// is a long-lived object here rather than something built per request.
	var suggestSvc *suggest.Service
	if cfg.MeiliKey != "" {
		facetOpts, err := facetEngineOptions(ctx, redisClient)
		if err != nil {
			return err
		}
		searchClient = search.NewClient(cfg.MeiliURL, cfg.MeiliKey, facetOpts...)
		suggestSvc = suggest.New(suggest.NewIndex(searchClient))
		startSuggestRefresh(ctx, suggestSvc)
	}

	// How many on-site applications one candidate may send a day. A value that is set but
	// unreadable fails the start, for the same reason the semantic-search knobs below do.
	directApplyLimit, err := config.LoadDirectApplyDailyLimit()
	if err != nil {
		return fmt.Errorf("direct apply: %w", err)
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
		return fmt.Errorf("blobstore: %w", err)
	}

	// The catalogue export bucket is separate from the résumé one and optional the same way.
	exportBucket, err := blobstore.NewBucket(blobstore.Config{
		Endpoint: cfg.ExportS3Endpoint, Bucket: cfg.ExportS3Bucket,
		AccessKey: cfg.ExportS3AccessKey, SecretKey: cfg.ExportS3SecretKey, Region: cfg.ExportS3Region,
	})
	if err != nil {
		return fmt.Errorf("catalogue export bucket: %w", err)
	}

	// The LLM is optional and built through the shared construction path
	// (llm.NewClient): it powers the CV ATS qualitative review, wires Langfuse
	// tracing (source "cv-ats") when LANGFUSE_* are set, and returns a flush func for
	// shutdown. Nil client when unconfigured — the ATS score stays deterministic. A
	// build error on a configured endpoint is fatal (a misconfigured gateway must not
	// boot silently).
	llmClient, llmFlush, err := llm.NewClient(cfg.Settings(cfg.Model), "cv-ats")
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	defer llmFlush()

	// The in-app assistant runs on its own model: ASSISTANT_MODEL is chosen for
	// reliable tool calling and a large context, where LLM_MODEL is chosen for cheap
	// one-shot JSON extraction. Unset falls back to LLM_MODEL, so a single-model
	// deployment still works. Traced under its own source label.
	assistantLLM, assistantFlush, err := llm.NewClient(cfg.Settings(cmp.Or(cfg.AssistantModel, cfg.Model)), "assistant")
	if err != nil {
		return fmt.Errorf("llm (assistant): %w", err)
	}
	defer assistantFlush()

	// The AI filter runs on its own small, fast model: the task is trivial and a person is watching
	// a spinner, so latency is the quality bar. Unset falls back to LLM_MODEL.
	intentLLM, intentFlush, err := llm.NewClient(cfg.Settings(cmp.Or(cfg.SearchIntentModel, cfg.Model)), "search-intent")
	if err != nil {
		return fmt.Errorf("llm (search intent): %w", err)
	}
	defer intentFlush()

	// Parsing a stored CV is the one model call every account gets, paid or not, so it
	// runs on its own, cheap model rather than on the paid features' choice. Unset falls
	// back to LLM_MODEL.
	resumeExtractLLM, resumeExtractFlush, err := llm.NewClient(cfg.Settings(cmp.Or(cfg.ResumeExtractModel, cfg.Model)), "resume-extract")
	if err != nil {
		return fmt.Errorf("llm (resume extract): %w", err)
	}
	defer resumeExtractFlush()

	// The support agent is open to every account, so like the CV parse it runs on its own
	// cheap model. Unset falls back to LLM_MODEL.
	supportLLM, supportFlush, err := llm.NewClient(cfg.Settings(cmp.Or(cfg.SupportModel, cfg.Model)), "support")
	if err != nil {
		return fmt.Errorf("llm (support): %w", err)
	}
	defer supportFlush()

	// Dictation uses the same gateway and key (/audio/transcriptions). Nil when the gateway is
	// unset, which the composer reads as "no microphone here".
	speechClient := speech.New(cfg.BaseURL, cfg.APIKey, cfg.STTModel)

	// Voice mode's Realtime credential rides the same gateway and key (litellm fronts
	// /v1/realtime/client_secrets). Nil when the gateway or REALTIME_MODEL is unset, which the
	// interview session view reads as "no voice mode here".
	realtimeClient := realtime.New(cfg.BaseURL, cfg.APIKey, cfg.RealtimeModel)

	// The gateway's administrative API, which mints the per-user credential each account's model
	// calls are spent under. Nil when unconfigured, an ordinary deployment: every call then goes
	// out on LLM_API_KEY. A separate endpoint and credential from inference on purpose.
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
			return fmt.Errorf("native Apple: %w", err)
		}
		appleGrantKeys, err = appleauth.NewKeyRing(cfg.AppleGrantActiveKeyID, cfg.AppleGrantKeys)
		if err != nil {
			return fmt.Errorf("native Apple grant encryption: %w", err)
		}
	}

	// The OJCP manifest signature. A key file that is set but cannot be read fails the start
	// rather than serving unsigned: an operator who configured a key would never notice the
	// signature had quietly gone missing.
	var ojcpSigner *ojcp.Signer
	if cfg.OJCPSigningKeyFile != "" {
		ojcpSigner, err = loadOJCPSigner(cfg.OJCPSigningKeyFile, cfg.FrontendOrigin)
		if err != nil {
			return fmt.Errorf("ojcp signing key: %w", err)
		}
	}

	// Connect-Gmail inbox: enabled only when the Google OAuth client and the
	// token-encryption key are configured. Both nil disables the feature.
	gmailConnector, gmailCipher := buildGmail(cfg)

	// Credits metering config, loaded alongside the other optional dependencies rather
	// than inline in the handler registration.
	planConfig := plan.ConfigFromEnv()

	// The homepage's live recently-added feed: an in-process poller drains recent_feed_outbox
	// (filled by cmd/ingest) into a Broadcaster the SSE handler subscribes to. Always on; it needs
	// no external credential.
	recentFeedBroadcaster := recentfeed.NewBroadcaster(recentFeedRingBufferSize)
	recentFeedPoller := recentfeed.NewPoller(db.New(pool), recentFeedBroadcaster, recentFeedBatchSize)
	go recentFeedPoller.Run(ctx, recentFeedPollInterval)

	// The public /embeddings page: samples semantic_outbox's claimable depth and combines it with
	// the coverage snapshot cmd/rollup-stats publishes.
	embedProgress := embedprogress.NewSampler(db.New(pool), cache.NewRedisCache(redisClient), time.Now)
	go embedProgress.Run(ctx, embedprogress.SampleInterval)

	// Postings served to agents, logged for cmd/rollup-views (internal/application/agentview).
	// Closed explicitly after app.Shutdown below, not deferred: a Write after Close panics,
	// and only once the server has stopped serving can none arrive.
	var agentViews agentview.Sink
	var agentViewWriter *agentview.Writer
	if agentViewWriter = agentview.NewWriter(cfg.AgentViewLogDir); agentViewWriter != nil {
		agentViews = agentViewWriter
	}

	// Per-account daily API request counts for the account's API page. The flush loop has
	// its own context so shutdown can stop it, wait for an in-flight flush to settle, and
	// only then flush the rest (below).
	apiUsage := apiusage.New(db.New(pool))
	usageCtx, stopUsage := context.WithCancel(context.Background())
	defer stopUsage()
	usageDone := make(chan struct{})
	go func() {
		defer close(usageDone)
		apiUsage.Run(usageCtx, apiUsageFlushInterval)
	}()

	handler.Register(app, handler.Config{
		Pool:                        pool,
		AgentViews:                  agentViews,
		APIUsage:                    apiUsage,
		Cache:                       cache.NewRedisCache(redisClient),
		FrontendOrigin:              cfg.FrontendOrigin,
		AuthIntrospectionSecret:     cfg.AuthIntrospectionSecret,
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
		OJCPSigner:                  ojcpSigner,
		MailboxDomain:               cfg.MailboxDomain,
		Search:                      searchClient,
		Suggest:                     suggestSvc,
		Blob:                        blobStore,
		Export:                      exportBucket,
		TypstBin:                    cfg.TypstBin,
		DirectApplyDailyLimit:       directApplyLimit,
		CVEditAllowBulletTruncation: cfg.CVEditAllowBulletTruncation,
		TracerLinkSalt:              cfg.TracerLinkSalt,
		LLM:                         llmClient,
		AssistantLLM:                assistantLLM,
		SearchIntentLLM:             intentLLM,
		ResumeExtractLLM:            resumeExtractLLM,
		SupportLLM:                  supportLLM,
		SupportReplyTo:              cfg.OnboardingReplyTo,
		TypesafeAPIKey:              cfg.TypesafeAPIKey,
		AssistantMaxSteps:           cfg.AssistantMaxSteps,
		AssistantMaxPrompt:          cfg.AssistantMaxPrompt,
		LLMKeys:                     llmKeys,
		Speech:                      speechClient,
		Realtime:                    realtimeClient,

		TelegramBotToken:      cfg.TelegramBotToken,
		TelegramBotUsername:   cfg.TelegramBotUsername,
		TelegramWebhookSecret: cfg.TelegramWebhookSecret,

		DiscordLinker:   buildDiscordLinker(cfg, pool),
		DiscordClientID: cfg.DiscordClientID,
		// Billing reads its own environment: the credentials are the provider's, nothing
		// else in the fleet needs them, and an absent one means the subsystem is simply off.
		Billing:     billing.ConfigFromEnv(),
		ServedHosts: cfg.ServedHosts,

		Plan: planConfig,

		AWSRegion:       cfg.AWSRegion,
		NotifyEmailFrom: cfg.NotifyEmailFrom,

		ExtensionRedirectAllowlist: cfg.ExtensionRedirectAllowlist,

		AutoApplyOrchestratorSecret: cfg.AutoApplyOrchestratorSecret,
		InngestEventAPIURL:          cfg.InngestEventAPIURL,
		InngestEventKey:             cfg.InngestEventKey,

		RecentJobsFeed: recentFeedBroadcaster,
		EmbedProgress:  embedProgress,
	})

	// Site-status daily history sampler: needs only the database, so no gate.
	handler.StartSiteStatusSampler(ctx, pool, db.New(pool), siteStatusSampleInterval)

	// Run the server in a goroutine so main can wait for a shutdown signal.
	// Fiber's Listen returns nil on graceful shutdown, so any error is fatal.
	serveErr := make(chan error, 1)
	go func() { serveErr <- app.Listen(":" + cfg.Port) }()
	log.Printf("hire listening on :%s", cfg.Port)

	// Registered here because the collector reads the pool at scrape time, so /metrics reports the
	// pool's occupancy as of the request with no sampling goroutine.
	prometheus.MustRegister(observability.NewPoolCollector(pool))

	observability.StartMetricsServer(cfg.MetricsPort)

	// Graceful shutdown on SIGINT/SIGTERM: block until the signal-bound context is
	// cancelled (the signal arrived) or startup cancelled it.
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	log.Println("shutting down...")

	shutdownErr := app.ShutdownWithTimeout(10 * time.Second)
	// The usage counts are flushed whether or not shutdown finished in time: what is
	// buffered is already counted. Run is stopped first so its flush cannot land after this one.
	stopUsage()
	<-usageDone
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFlush()
	if err := apiUsage.Flush(flushCtx); err != nil {
		log.Printf("apiusage: final flush: %v", err)
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown: %w", shutdownErr)
	}
	// Flushes the buffered agent-view events; no handler can write after the shutdown above.
	agentViewWriter.Close()
	return nil
}

// recentFeedRingBufferSize bounds the backlog a new SSE connection to the homepage feed
// replays, so it never starts on a visibly empty feed.
const recentFeedRingBufferSize = 15

// recentFeedPollInterval is how often the poller drains recent_feed_outbox. A few seconds of
// latency is accepted over Postgres LISTEN/NOTIFY or Redis pub/sub.
const recentFeedPollInterval = 10 * time.Second

// recentFeedBatchSize caps one poll tick's claim, so a burst (e.g. INGEST_REFETCH_ALL=1) leaves
// the feed a tick behind instead of doing unbounded work.
const recentFeedBatchSize = 500

// apiStatementTimeout is how long Postgres lets one query on the API server's pool run before
// cancelling it. It is the backstop for endpoints whose cost nobody anticipated; the
// per-endpoint bounds (pagination window, limit ceilings) should do the real work. A cut
// request answers an error rather than holding a connection while other callers queue.
// LLM-backed routes are bounded by their own per-call timeout instead.
const apiStatementTimeout = 30 * time.Second

// siteStatusSampleInterval is how often the /status page's daily history sampler
// (handler.StartSiteStatusSampler) records the site's current status.
const siteStatusSampleInterval = 5 * time.Minute

// suggestRefreshInterval is how often the completion service reloads the phrases it recognises.
// cmd/build-suggestions rebuilds the dictionary daily, so this only has to be shorter; a phrase
// not yet learned falls into the fragment, where the index completes it.
const suggestRefreshInterval = time.Hour

// apiUsageFlushInterval is how often buffered API request counts are written, and so how
// many a crash can lose.
const apiUsageFlushInterval = time.Minute
