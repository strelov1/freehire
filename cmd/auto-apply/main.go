// Command auto-apply drains auto_apply_queue: for each queued (candidate, job) attempt it
// resolves the candidate's known profile answers against the job's live application form
// through internal/atsapply's headless browser, and submits only when every required question
// is answered. What fills the queue is out of scope; it only drains what is there.
//
// It exits non-zero when the run had any failures or dead letters, so cron can alert.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/atsapply"
	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/browseruse"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	acfg := config.LoadAutoApply()

	// blobStore is nil when S3_* is unconfigured, and that is fine: CV attachment renders the
	// approved tailored CV on demand through Typst, and candidateprofile's Structured() reads
	// only Postgres.
	blobStore, err := blobstore.New(blobstore.Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		log.Printf("blobstore: %v", err)
		return 1
	}

	queries := db.New(pool)
	cvStore := cv.NewStore(cv.NewQueriesRepository(queries))
	resumeStore := resume.New(blobStore, resume.NewQueriesRepository(queries))
	screeningAnswersSvc := screeninganswers.New(screeninganswers.NewQueriesRepository(queries, pool))
	// The candidate's bank of screening-question answers, the same store internal/api/handler
	// wires into its assembler, so a banked answer reaches the form this worker actually fills.
	answerBank := answerbank.NewStore(answerbank.NewQueriesRepository(queries))
	// The same five sources, in the same precedence order, as the extension-autofill path; see
	// internal/candidateprofile's package doc for why this must be the one Assembler.
	answers := assemblerAnswerSource{
		assembler: candidateprofile.NewAssembler(cvStore, resumeStore, queries, screeningAnswersSvc, answerBank).
			WithProfileLocation(userprofile.New(userprofile.NewQueriesRepository(queries))),
	}

	// Question drafting is optional: an unconfigured LLM degrades to no drafting, so every field
	// either resolves deterministically or parks.
	llmClient, llmFlush, err := llm.NewClient(cfg.Settings(cfg.Model), "auto-apply")
	if err != nil {
		log.Printf("llm: %v", err)
		return 1
	}
	defer llmFlush()
	// The gateway's admin API mints the per-candidate credential a draft call spends under
	// (llmkey.Bind in internal/atsapply.Client.resolve). Nil-safe: without it, draft calls spend
	// on the service credential, still tagged.
	llmKeys := llmkey.New(llmkey.Config{
		BaseURL:       cfg.LLMAdminURL,
		AdminUsername: cfg.LLMAdminUsername,
		AdminPassword: cfg.LLMAdminPassword,
		TemplateKey:   cfg.LLMAdminTemplateKey,
		MaxBudget:     cfg.LLMUserMaxBudget,
		RPMLimit:      cfg.LLMUserRPMLimit,
		BudgetWindow:  cfg.LLMUserBudgetWindow,
	})
	llmKeyResolver := llmkey.NewResolver(queries, llmKeys)
	atoms := experience.NewStore(experience.NewQueriesRepository(queries))
	letters := coverletter.NewStore(coverletter.NewQueriesRepository(queries))

	// cvRenderer is nil when no typst binary is configured (config.resolveTypstBin); a CV file
	// field then parks (Client.attachApprovedResume) instead of crashing. Kept as an untyped nil
	// interface: a nil *TypstRenderer assigned into the cv.Renderer interface is not a nil
	// interface, so Client's `c.renderer == nil` check would miss it and panic.
	var cvRenderer cv.Renderer
	if r := cv.NewTypstRenderer(cfg.TypstBin); r != nil {
		cvRenderer = r
	}

	// The same HTTP client as internal/applyform's capture worker: the Greenhouse/Ashby endpoints
	// reused via applyform.Fetchers are public job-board APIs, so the crawl's user agent,
	// timeouts and size caps fit. Requests egress through the proxy so the host's address is
	// never the one a platform sees (sources.NewFormClientFromEnv).
	formClient, proxied, err := sources.NewFormClientFromEnv()
	if err != nil {
		log.Printf("auto-apply: %v", err)
		return 1
	}
	if !proxied {
		log.Printf("auto-apply: WARNING no SOURCES_ROTATING_PROXY_URL or SOURCES_PROXY_URL — application forms are fetched from this host's own address")
	}
	// Drafting checks the plan per attempt that the candidate is still on a paid plan before
	// spending a model call (atsapply.Client.resolve). Only the tier is read, so the allowance
	// figures play no part.
	entitlement := plan.NewStore(queries, pool, plan.ConfigFromEnv())
	sidecar := atsapply.NewClient(formClient, llmClient, llmKeyResolver, cfg.TypesafeAPIKey, atoms, letters, cvStore, cvRenderer, entitlement)
	// Stored-form fallback: a provider with no live schema fetcher (today Recruitee) reaches
	// field resolution by reading the form already captured into apply_forms. Client does not
	// mirror PreviewClient's storage-first order; see fetchSchema.
	sidecar = sidecar.WithStoredFormReader(&dbApplyFormReader{q: queries})
	// browser-use fallback: an empty key leaves Ashby/Workable/Recruitee parking with
	// reasonSubmissionNotImplemented. Its enforce flag and cost caps are read by
	// internal/api/atsapply, not threaded through here.
	if acfg.BrowserUseAPIKey != "" {
		buClient := browseruse.New(acfg.BrowserUseAPIKey, "", nil)
		sidecar = sidecar.WithBrowserUse(atsapply.NewBrowserUseExecutor(buClient))
	}

	// Entries whose tailoring nobody is doing any more become tailor_failed, so they stop
	// reading as work in progress and a retry can start them again. Best-effort: a failure
	// here costs this run's sweep, never the submissions below.
	if n, err := queries.MarkStaleAutoApplyTailoringFailed(ctx, staleTailoringSeconds); err != nil {
		log.Printf("auto-apply: marking stale tailoring: %v", err)
	} else if n > 0 {
		log.Printf("auto-apply: %d attempt(s) had no tailoring progress for %s; marked tailor_failed", n, time.Duration(staleTailoringSeconds)*time.Second)
	}

	store := newDBStore(pool)
	stats, err := autoapply.Run(ctx, store, answers, sidecar, autoapply.RunOptions{
		BatchSize:    acfg.BatchSize,
		LeaseSeconds: acfg.LeaseSeconds,
		MaxAttempts:  acfg.MaxAttempts,
		Concurrency:  acfg.Concurrency,
		MaxPerRun:    acfg.MaxPerRun,
		CallTimeout:  acfg.CallTimeout,
	})
	if err != nil {
		log.Printf("auto-apply: %v", err)
		return 1
	}

	// Second, independent claim pass: resolves the answer preview for entries a tailoring run
	// already produced a CV for, reusing this worker's browser dependency (cmd/auto-apply-
	// orchestrate is built without one). previewSidecar shares no state with sidecar and uses the
	// same stored-form reader, so a non-Greenhouse preview costs a Postgres read, not a second
	// fetch.
	previewSidecar := atsapply.NewPreviewClient(formClient, &dbApplyFormReader{q: queries})
	previewStats, err := autoapply.RunPreviews(ctx, store, answers, previewSidecar, autoapply.RunOptions{
		BatchSize:    acfg.BatchSize,
		LeaseSeconds: acfg.LeaseSeconds,
		MaxAttempts:  acfg.MaxAttempts,
		Concurrency:  acfg.Concurrency,
		MaxPerRun:    acfg.MaxPerRun,
		CallTimeout:  acfg.CallTimeout,
	})
	if err != nil {
		log.Printf("auto-apply previews: %v", err)
		return 1
	}

	log.Printf("auto-apply done: applied=%d blocked=%d failed=%d dead_lettered=%d; previews resolved=%d parked=%d failed=%d dead_lettered=%d",
		stats.Applied, stats.Blocked, stats.Failed, stats.DeadLettered,
		previewStats.Resolved, previewStats.Parked, previewStats.Failed, previewStats.DeadLettered)
	// Deliberately not worker.ExitCode: a parked attempt is not a fault to alert on (see
	// autoapply.RunStats.Degraded and PreviewStats.Degraded).
	if stats.Degraded() || previewStats.Degraded() {
		return 1
	}
	return 0
}

// staleTailoringSeconds is how long an attempt with no tailored CV may go without history
// before it is given up on. Far above one orchestrator run (12 minutes, hireRequestTimeout in
// internal/application/autoapplyorchestrate) plus Inngest's step retries, so only an abandoned
// attempt crosses it.
const staleTailoringSeconds = 6 * 60 * 60
