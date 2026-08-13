// Command backfill-experience seeds the experience bank for users who already had a CV
// when the bank shipped — the run-once-and-exit counterpart of the on-upload import in
// handler.importExperience. Without it the bank is empty for every existing user, and the
// stage that moves the read path onto it would leave them with no candidate context.
//
// It reuses an already-extracted structured résumé wherever one is current, and calls the
// LLM only for users who have none. That distinction is the point: re-extracting every
// stored CV through the model is the same class of spend that has bitten enrichment
// before, and most users' CVs were parsed at upload time already.
//
// The extractor is OPTIONAL. With the S3/LLM/PII settings unconfigured the worker still
// runs the free pass over users with a current structure, and reports how many it had to
// leave for a later run rather than silently covering less than it claims.
//
// Idempotent by construction: import matches employments on (company, role) and atoms on
// the normalized claim, so a second run creates nothing. Needs DATABASE_URL.
//
//	go run ./cmd/backfill-experience                # every user with a stored CV
//	go run ./cmd/backfill-experience --user 291     # a single user
//	go run ./cmd/backfill-experience --dry-run      # report, don't write
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/pii"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/worker"
)

// extractTimeout bounds a single structured-extraction LLM call, matching the on-upload
// path's ceiling (handler.resumeExtractLLMTimeout).
const extractTimeout = 120 * time.Second

// target is one user to seed, and whether their stored structure can be reused as-is.
type target struct {
	id int64
	// structured is the current structured résumé, present when its stamp still matches
	// the CV upload time. When present the user costs no model call.
	structured *resumeextract.Structured
	uploadedAt time.Time
}

func main() {
	worker.Main(run)
}

// run holds the whole worker so every exit is a return value: the Langfuse trace flush
// and the pool close are deferred here, and os.Exit — which log.Fatalf calls — would
// skip both on exactly the failed run whose traces explain the failure.
func run() int {
	var userID int64
	var dryRun bool
	flag.Int64Var(&userID, "user", 0, "seed a single user id (0 = every user with a stored CV)")
	flag.BoolVar(&dryRun, "dry-run", false, "report what would be imported without writing")
	flag.Parse()

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)

	bank := experience.NewStore(experience.NewQueriesRepository(queries))

	extractor, resumeStore, flush := newExtractor(cfg, queries)
	if flush != nil {
		defer flush()
	}

	targets, err := eligible(ctx, queries, userID)
	if err != nil {
		log.Printf("query eligible users: %v", err)
		return 1
	}

	var reusable, needExtract int
	for _, t := range targets {
		if t.structured != nil {
			reusable++
		} else {
			needExtract++
		}
	}
	log.Printf("backfill-experience: %d user(s) with a stored CV — %d reusable, %d need extraction%s",
		len(targets), reusable, needExtract, dryRunSuffix(dryRun))
	if needExtract > 0 && extractor == nil {
		log.Printf("backfill-experience: the extractor is unconfigured (S3_*/LLM_*/PII_FILTER_URL) — "+
			"%d user(s) will be left for a later run", needExtract)
	}

	var seeded, skipped, failed int
	var cancelled bool
	for i, t := range targets {
		// The root context is signal-bound, so a redeploy's SIGTERM cancels it mid-run.
		// Without this the loop would run to the end of the list turning every remaining
		// user into a logged failure, reporting one cancellation as a fleet of them.
		if ctx.Err() != nil {
			log.Printf("backfill-experience: cancelled — %d target(s) left unprocessed; the import is "+
				"idempotent, so re-running finishes them", len(targets)-i)
			cancelled = true
			break
		}
		st, err := resolveStructured(ctx, t, extractor, resumeStore)
		if err != nil {
			log.Printf("user %d: %v", t.id, err)
			failed++
			continue
		}
		if st == nil {
			skipped++
			continue
		}

		entries := experience.EntriesFromResume(*st)
		if len(entries) == 0 {
			log.Printf("user %d: the parsed CV states no work history — nothing to bank", t.id)
			skipped++
			continue
		}
		if dryRun {
			log.Printf("user %d: would import %d employment(s)", t.id, len(entries))
			continue
		}

		result, err := bank.Import(ctx, t.id, entries, t.uploadedAt.UTC().Format(time.RFC3339))
		if err != nil {
			log.Printf("user %d: import: %v", t.id, err)
			failed++
			continue
		}
		log.Printf("user %d: %d employments created, %d filled, %d atoms created, %d already banked",
			t.id, result.EmploymentsCreated, result.EmploymentsFilled, result.AtomsCreated, result.AtomsSkipped)
		seeded++
	}

	log.Printf("backfill-experience: done — %d seeded, %d skipped, %d failed", seeded, skipped, failed)
	if cancelled {
		return 1
	}
	return worker.ExitCode(failed, 0)
}

// resolveStructured returns the structured résumé to import for this target: the stored
// one when it is current, otherwise a fresh extraction. A nil structure with a nil error
// means "nothing to do for this user" — no stored structure and no configured extractor.
func resolveStructured(ctx context.Context, t target, extractor *resumeextract.Extractor, store *resume.Store) (*resumeextract.Structured, error) {
	if t.structured != nil {
		return t.structured, nil
	}
	if extractor == nil || store == nil {
		return nil, nil
	}

	text, err := store.Text(ctx, t.id)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, extractTimeout+30*time.Second)
	defer cancel()
	st, err := extractor.Extract(cctx, text)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// eligible lists every user with a stored CV, carrying their structured résumé when the
// generated query judged it current.
func eligible(ctx context.Context, queries *db.Queries, userID int64) ([]target, error) {
	rows, err := queries.ListExperienceBackfillTargets(ctx, userID)
	if err != nil {
		return nil, err
	}
	targets := make([]target, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, target{
			id:         row.ID,
			structured: decodeStructured(row.CurrentStructured),
			uploadedAt: row.ResumeUploadedAt.Time,
		})
	}
	return targets, nil
}

func dryRunSuffix(dry bool) string {
	if dry {
		return " (dry-run)"
	}
	return ""
}

// newExtractor builds the optional extraction path. Every piece is optional: with any of
// S3, the LLM or the PII detector missing, the worker still runs its free pass over users
// whose structured résumé is current.
func newExtractor(cfg config.Settings, queries *db.Queries) (*resumeextract.Extractor, *resume.Store, func()) {
	blobStore, err := blobstore.New(blobstore.Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
	})
	if err != nil || blobStore == nil {
		return nil, nil, nil
	}
	if cfg.PIIFilterURL == "" {
		return nil, nil, nil
	}
	llmClient, flush, err := llm.NewClient(cfg.Settings(cfg.Model), "backfill-experience")
	if err != nil || llmClient == nil {
		return nil, nil, nil
	}
	store := resume.New(blobStore, resume.NewQueriesRepository(queries))
	return resumeextract.NewExtractor(llmClient.WithTimeout(extractTimeout), pii.NewHTTPDetector(cfg.PIIFilterURL, nil)), store, flush
}

// decodeStructured turns a stored resume_structured payload into the typed value, or nil
// when it is absent or unreadable — an unreadable structure simply falls through to the
// extraction path rather than failing the user.
func decodeStructured(raw []byte) *resumeextract.Structured {
	if len(raw) == 0 {
		return nil
	}
	var st resumeextract.Structured
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil
	}
	return &st
}
