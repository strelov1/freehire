// Command backfill-resume-structured re-derives the read-only structured résumé for
// users whose stored CV has none (or a stale one) — the one-off, run-once-and-exit
// counterpart of the on-upload derivation in handler.extractStructuredResume. It exists
// because the structured résumé has no reconciler: a decode failure during the background
// derivation (e.g. the model emitting a numeric year that aborted the whole unmarshal,
// fixed in resumeextract) leaves resume_structured NULL forever until a re-upload. This
// worker reads the stored CV text, re-runs the extractor, and persists the result stamped
// with the current résumé upload time (so Store.Structured serves it).
//
// Idempotent: it only touches users missing a current structured résumé, and re-running
// is safe. Needs DATABASE_URL, the S3 (résumé storage) settings, and the LLM_* settings.
//
//	go run ./cmd/backfill-resume-structured                # all eligible users
//	go run ./cmd/backfill-resume-structured --user 291     # a single user
//	go run ./cmd/backfill-resume-structured --dry-run      # list, don't persist
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/database"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// extractTimeout bounds a single structured-extraction LLM call, matching the on-upload
// path's ceiling (handler.resumeExtractLLMTimeout).
const extractTimeout = 120 * time.Second

func main() {
	var userID int64
	var dryRun bool
	flag.Int64Var(&userID, "user", 0, "backfill a single user id (0 = every eligible user)")
	flag.BoolVar(&dryRun, "dry-run", false, "list eligible users without extracting or persisting")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	blobStore, err := blobstore.New(blobstore.Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		log.Fatalf("blobstore: %v", err)
	}
	if blobStore == nil {
		log.Fatal("backfill-resume-structured: résumé storage (S3_*) is not configured")
	}

	llmClient, llmFlush, err := llm.NewClient(llm.Settings{
		BaseURL:           cfg.LLMBaseURL,
		APIKey:            cfg.LLMAPIKey,
		Model:             cfg.LLMModel,
		LangfuseBaseURL:   cfg.LangfuseBaseURL,
		LangfusePublicKey: cfg.LangfusePublicKey,
		LangfuseSecretKey: cfg.LangfuseSecretKey,
	}, "resume-structured")
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	defer llmFlush()
	if llmClient == nil {
		log.Fatal("backfill-resume-structured: LLM (LLM_*) is not configured")
	}

	store := resume.New(blobStore, resume.NewQueriesRepository(queries))
	extractor := resumeextract.NewExtractor(llmClient.WithTimeout(extractTimeout))

	// Eligible: a stored CV whose structured résumé is missing or stale (its stamp no
	// longer equals the current upload time — the same freshness rule Store.Structured reads).
	rows, err := pool.Query(ctx, `
		SELECT id, resume_uploaded_at
		FROM users
		WHERE resume_object_key IS NOT NULL
		  AND resume_uploaded_at IS NOT NULL
		  AND ($1 = 0 OR id = $1)
		  AND (resume_structured IS NULL
		       OR resume_structured_uploaded_at IS DISTINCT FROM resume_uploaded_at)
		ORDER BY id`, userID)
	if err != nil {
		log.Fatalf("query eligible users: %v", err)
	}
	type target struct {
		id         int64
		uploadedAt time.Time
	}
	var targets []target
	for rows.Next() {
		var id int64
		var uploadedAt pgtype.Timestamptz
		if err := rows.Scan(&id, &uploadedAt); err != nil {
			rows.Close()
			log.Fatalf("scan: %v", err)
		}
		targets = append(targets, target{id: id, uploadedAt: uploadedAt.Time})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("iterate: %v", err)
	}

	log.Printf("backfill-resume-structured: %d eligible user(s)%s", len(targets), dryRunSuffix(dryRun))

	var ok, failed int
	for _, t := range targets {
		text, err := store.Text(ctx, t.id)
		if err != nil {
			log.Printf("user %d: read CV text: %v", t.id, err)
			failed++
			continue
		}
		if dryRun {
			log.Printf("user %d: would extract from %d chars of CV text", t.id, len(text))
			continue
		}

		cctx, cancel := context.WithTimeout(ctx, extractTimeout+30*time.Second)
		st, err := extractor.Extract(cctx, text)
		cancel()
		if err != nil {
			log.Printf("user %d: extract: %v", t.id, err)
			failed++
			continue
		}
		if err := store.SetStructured(ctx, t.id, st, extractor.ModelID(), t.uploadedAt); err != nil {
			log.Printf("user %d: persist: %v", t.id, err)
			failed++
			continue
		}
		log.Printf("user %d: structured backfilled (%d skills, %d experience, %d education)",
			t.id, len(st.Skills), len(st.Experience), len(st.Education))
		ok++
	}
	log.Printf("backfill-resume-structured: done — %d succeeded, %d failed", ok, failed)
}

func dryRunSuffix(dry bool) string {
	if dry {
		return " (dry-run)"
	}
	return ""
}
