// Command auto-apply drains auto_apply_queue: for each queued (candidate, job) attempt it
// resolves the candidate's known profile answers against the job's live application form,
// through internal/atsapply's headless browser, and submits only when every required
// question is answered. What populates the queue is out of scope for this worker — see
// openspec/changes/auto-apply-worker/design.md — it only ever drains what is already there.
//
// It exits non-zero when the run had any failures or dead letters, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/api/atsapply"
	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/blobstore"
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

	// blobStore is nil when S3_* is unconfigured, and that is fine here: résumé attachment
	// (openspec/changes/auto-apply-tailored-resume) renders the approved tailored CV
	// on demand through the Typst renderer and never touches object storage — the only
	// résumé-adjacent read this worker makes elsewhere is candidateprofile's Structured(),
	// which reads the parsed structure from Postgres (resume.Store.Structured reads only
	// its repo). S3_* stays optional here.
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
	screeningAnswersSvc := screeninganswers.New(screeninganswers.NewQueriesRepository(queries))
	// The same four sources, in the same precedence order, internal/handler's
	// extension-autofill path already resolves a candidate's profile through — see
	// internal/candidateprofile's package doc for why this must be the one Assembler.
	answers := assemblerAnswerSource{
		assembler: candidateprofile.NewAssembler(cvStore, resumeStore, queries, screeningAnswersSvc),
	}

	// Question drafting (openspec/changes/auto-apply-llm-drafting) is optional: an
	// unconfigured LLM degrades to no drafting, the same convention llm.NewClient gives
	// every other feature — every field either resolves deterministically or parks,
	// exactly as it did before this capability existed.
	llmClient, llmFlush, err := llm.NewClient(cfg.Settings(cfg.Model), "auto-apply")
	if err != nil {
		log.Printf("llm: %v", err)
		return 1
	}
	defer llmFlush()
	// The gateway's administrative API mints the per-candidate credential a draft call
	// spends under (llmkey.Bind, called per attempt in internal/atsapply.Client.resolve).
	// Nil-safe: an unconfigured admin API just means every draft call spends on the
	// service credential instead, still tagged.
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

	// cvRenderer is nil when no typst binary is configured (config.resolveTypstBin), the
	// same nil-safe gating internal/api/handler's PDF-download endpoint uses. A résumé file
	// field then simply cannot be filled — Client.attachApprovedResume degrades that to a
	// park, the same outcome an unresolved field already produces, never a crash.
	//
	// Left as the untyped nil interface rather than the nil *TypstRenderer directly: a nil
	// concrete pointer assigned into the cv.Renderer INTERFACE parameter is not a nil
	// interface (boundRunner's own comment in internal/api/handler/assistant.go documents
	// the same trap), so Client's own `c.renderer == nil` check would never see it as
	// unconfigured and would panic on the first call instead.
	var cvRenderer cv.Renderer
	if r := cv.NewTypstRenderer(cfg.TypstBin); r != nil {
		cvRenderer = r
	}

	// The same HTTP client the crawl and internal/applyform's own capture worker use: the
	// Greenhouse/Ashby endpoints internal/atsapply reuses via applyform.Fetchers are the
	// platforms' own public job-board APIs, so its user agent, timeouts and size caps are
	// exactly right here too.
	sidecar := atsapply.NewClient(sources.NewClient(), llmClient, llmKeyResolver, atoms, cvStore, cvRenderer)

	stats, err := autoapply.Run(ctx, newDBStore(pool), answers, sidecar, autoapply.RunOptions{
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

	log.Printf("auto-apply done: applied=%d blocked=%d failed=%d dead_lettered=%d",
		stats.Applied, stats.Blocked, stats.Failed, stats.DeadLettered)
	// Deliberately not worker.ExitCode: see autoapply.RunStats.Degraded for why a parked
	// attempt is not a fault this worker's exit code should alert on.
	if stats.Degraded() {
		return 1
	}
	return 0
}
