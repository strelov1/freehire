// Package maillink is the classify-mail worker's core: it drains the email
// classification outbox, resolves each email to one of the caller's applications
// via the deterministic mailmatch cascade (falling through to the LLM), classifies
// its status via mailclassify, and persists the confidence-tiered link + a
// monotonic-forward stage advancement. Store and Classifier are ports so the
// runner is unit-tested with fakes; cmd/classify-mail wires the real adapters.
package maillink

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/mailclassify"
	"github.com/strelov1/freehire/internal/mailmatch"
	"github.com/strelov1/freehire/internal/outbox"
)

const (
	defaultLeaseSeconds = 120
	defaultBatchSize    = 20
	defaultMaxAttempts  = 3
)

var defaultThresholds = thresholds{autoLink: 0.85, stage: 0.8}

// Claimed is one leased outbox entry joined with its email.
type Claimed struct {
	OutboxID int64
	EmailID  int64
	UserID   int64
	// Source is the message's store (see inbox.Sources). It rides along so the
	// ledger event Save writes can name which mailbox observed the reply.
	Source   string
	ThreadID string
	FromAddr string
	FromName string
	Subject  string
	Body     string // plain-text part; empty for HTML-only mail (e.g. many ATS templates)
	BodyHTML string // HTML part, stripped to text as the classifier body fallback
}

// Application is one of the caller's tracked applications offered to the matcher.
type Application struct {
	JobID   int64
	Company string
}

// Result is the persisted outcome for one email.
type Result struct {
	EmailID        int64
	JobID          int64 // 0 = unlinked
	SuggestedJobID int64
	LinkSource     string // "auto" | "" (empty for a suggestion/unlinked)
	Confidence     float64
	Signal         mailclassify.StatusSignal
	AdvanceStageTo string // non-empty → move the linked application forward
	// MailSource is the message's store, carried through so the ledger event
	// records the mailbox that observed the reply rather than guessing.
	MailSource string
}

// Store is the persistence port.
type Store interface {
	EnqueuePending(ctx context.Context) (int64, error)
	ClaimBatch(ctx context.Context, leaseSeconds, batchSize int) ([]Claimed, error)
	Applications(ctx context.Context, userID int64) ([]Application, error)
	ThreadLinks(ctx context.Context, userID int64) (map[string]int64, error)
	CurrentStage(ctx context.Context, userID, jobID int64) (string, error)
	// Save persists the result and deletes the outbox row in one transaction.
	Save(ctx context.Context, outboxID, userID int64, r Result, model string) error
	// Fail records a failed attempt and reports whether the entry dead-lettered — reached
	// max_attempts and will not be retried. The bool is not decoration: it is what the
	// worker's exit code is built from, and without it a queue can dead-letter every entry
	// and still exit 0.
	Fail(ctx context.Context, outboxID int64, cause string, maxAttempts int) (deadLettered bool, err error)
}

// Classifier is the LLM port.
type Classifier interface {
	Classify(ctx context.Context, in mailclassify.Input) (mailclassify.Classification, error)
}

// Learner records the sender of a confidently-classified application email so a
// recurring unknown ATS domain can be learned into the sync allowlist. Optional:
// a nil learner disables the feedback loop. Kept minimal so maillink stays
// decoupled from the sync package that implements it.
type Learner interface {
	Learn(ctx context.Context, fromAddr string) error
}

// Runner drains the outbox.
type Runner struct {
	store        Store
	classifier   Classifier
	learner      Learner
	model        string
	cfg          thresholds
	leaseSeconds int
	batchSize    int
	maxAttempts  int
}

// New builds a Runner with the default lease/batch/threshold tuning.
func New(store Store, classifier Classifier, model string) *Runner {
	return &Runner{
		store:        store,
		classifier:   classifier,
		model:        model,
		cfg:          defaultThresholds,
		leaseSeconds: defaultLeaseSeconds,
		batchSize:    defaultBatchSize,
		maxAttempts:  defaultMaxAttempts,
	}
}

// WithLearner wires the self-learning ATS-domain feedback loop, returning the
// runner for chaining.
func (r *Runner) WithLearner(l Learner) *Runner {
	r.learner = l
	return r
}

// Run enqueues every unclassified email, then drains the outbox wave by wave
// until it is empty.
// Stats is what a run reports upward. Failed and DeadLettered are mutually
// exclusive — a dead-lettered entry counts only in DeadLettered, matching
// internal/outbox.Outcome's one-outcome-per-item shape (and applyform/adzunadesc's
// RunStats, migrated onto the same shared runner just before this package: see
// applyform.RunStats' doc comment for the pre-migration double-count this convention
// corrects). cmd/classify-mail turns them into its exit code, so a mail queue that
// quietly stops working is visible to the scheduler rather than only in journalctl.
type Stats struct {
	Failed       int
	DeadLettered int
}

// claimAdapter adapts Store to outbox.Claimer: Store.ClaimBatch takes its two size
// arguments in the opposite order (leaseSeconds, batchSize) from outbox.Claimer's
// Claim(ctx, batch, leaseSeconds).
type claimAdapter struct {
	store Store
}

func (c claimAdapter) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	return c.store.ClaimBatch(ctx, leaseSeconds, batch)
}

func (r *Runner) Run(ctx context.Context) (Stats, error) {
	if _, err := r.store.EnqueuePending(ctx); err != nil {
		return Stats{}, err
	}
	// A wave is mostly one user's mail, and their application list is the same for
	// every message in it — see appCache for why the thread links are NOT cached
	// alongside it. Created once for the whole Run (not per wave), exactly as before:
	// the closure below captures it, and outbox.RunPool calls that same closure across
	// every wave in this Run.
	cache := appCache{store: r.store, byUser: map[int64][]Application{}}
	result, err := outbox.RunPool(ctx, claimAdapter{store: r.store}, outbox.RunOptions{
		BatchSize:    r.batchSize,
		LeaseSeconds: r.leaseSeconds,
		// Strictly sequential — no goroutine spawned at all (see internal/outbox's
		// Claimer doc comment on RunPool's Concurrency<=1 branch). appCache.byUser is
		// a plain, non-synchronized map, and a later message in the same thread must
		// see the same wave's earlier link (ThreadLinks is read live, not cached) —
		// both require true in-order processing, not just a small pool.
		Concurrency: 1,
	}, func(ctx context.Context, c Claimed) outbox.Outcome {
		err := r.process(ctx, cache, c)
		if err == nil {
			return outbox.Succeeded
		}
		deadLettered, ferr := r.store.Fail(ctx, c.OutboxID, err.Error(), r.maxAttempts)
		if ferr != nil {
			// The dead-letter state is unknown and is not guessed — it still counts as
			// failed (the entry is left to its lease expiry either way).
			log.Printf("maillink: fail outbox %d: %v", c.OutboxID, ferr)
			return outbox.Failed
		}
		if deadLettered {
			return outbox.DeadLettered
		}
		return outbox.Failed
	})
	stats := Stats{Failed: result.Failed, DeadLettered: result.DeadLettered}
	if err != nil {
		return stats, err
	}
	return stats, nil
}

// appCache memoises one user's applications for the length of a run.
//
// Only this half of the per-user context may be cached. Application membership is
// "applied, saved or staged", and the only write a run makes to it is a forward
// stage move on a row that is already a member — so the list cannot change under
// us. The thread links CAN: Save writes job_id onto the message it just linked,
// and a later message in the same thread has to see that link to continue it.
// Caching both reads as the same optimisation and silently costs thread continuity
// within a wave.
type appCache struct {
	store  Store
	byUser map[int64][]Application
}

func (c appCache) get(ctx context.Context, userID int64) ([]Application, error) {
	if apps, ok := c.byUser[userID]; ok {
		return apps, nil
	}
	apps, err := c.store.Applications(ctx, userID)
	if err != nil {
		return nil, err
	}
	c.byUser[userID] = apps
	return apps, nil
}

func (r *Runner) process(ctx context.Context, cache appCache, c Claimed) error {
	apps, err := cache.get(ctx, c.UserID)
	if err != nil {
		return err
	}
	// Read live on every message: a link this run just wrote must be visible to the
	// rest of the wave.
	links, err := r.store.ThreadLinks(ctx, c.UserID)
	if err != nil {
		return err
	}

	m := mailmatch.Resolve(
		mailmatch.Email{ThreadID: c.ThreadID, FromName: c.FromName, Subject: c.Subject},
		matchCandidates(apps, links),
	)
	// Only spend the LLM's disambiguation on the ambiguous/unmatched tail; a
	// confident deterministic match still needs the status, so classify either way.
	// The same predicate decides what resolveLink persists below — see autoLinks.
	var candidates []mailclassify.Candidate
	if !r.cfg.autoLinks(m) {
		candidates = classifyCandidates(apps)
	}
	cls, err := r.classifier.Classify(ctx, mailclassify.Input{
		FromName: c.FromName, Subject: c.Subject, Body: ReadableBody(c.Body, c.BodyHTML), Candidates: candidates,
	})
	if err != nil {
		return err
	}

	job, suggested, source, conf := resolveLink(m, cls, r.cfg)
	advanceTo := ""
	if job != 0 {
		cur, err := r.store.CurrentStage(ctx, c.UserID, job)
		if err != nil {
			return err
		}
		advanceTo = stageAdvance(job, cur, cls, r.cfg)
	}

	if err := r.store.Save(ctx, c.OutboxID, c.UserID, Result{
		EmailID:        c.EmailID,
		JobID:          job,
		SuggestedJobID: suggested,
		LinkSource:     source,
		Confidence:     conf,
		Signal:         cls.Signal,
		AdvanceStageTo: advanceTo,
		MailSource:     c.Source,
	}, r.model); err != nil {
		return err
	}

	// Feed the self-learning cache: a concrete (non-"other") signal AT the same
	// confidence bar this package already trusts to act automatically (cfg.stage —
	// see stageAdvance) means the classifier confidently recognised
	// application-lifecycle mail, so its sender domain is worth learning.
	// internal/gmailsync/learn.go documents the write side as triggering on
	// "confident job-mail sightings"; without a confidence check here, repeated
	// low-confidence guesses on the same sender could still reach PromoteThreshold
	// and promote a domain the rest of the pipeline treats as too weak to act on.
	// Best-effort — a learn failure must not fail the email.
	if r.learner != nil && cls.Signal != "" && cls.Signal != mailclassify.SignalOther && cls.Confidence >= r.cfg.stage {
		if err := r.learner.Learn(ctx, c.FromAddr); err != nil {
			log.Printf("maillink: learn %q: %v", c.FromAddr, err)
		}
	}
	return nil
}

// matchCandidates attaches each application's already-linked thread ids so the
// thread-continuity tier can fire.
func matchCandidates(apps []Application, links map[string]int64) []mailmatch.Candidate {
	byJob := map[int64][]string{}
	for threadID, jobID := range links {
		byJob[jobID] = append(byJob[jobID], threadID)
	}
	out := make([]mailmatch.Candidate, 0, len(apps))
	for _, a := range apps {
		out = append(out, mailmatch.Candidate{JobID: a.JobID, Company: a.Company, ThreadIDs: byJob[a.JobID]})
	}
	return out
}

func classifyCandidates(apps []Application) []mailclassify.Candidate {
	out := make([]mailclassify.Candidate, 0, len(apps))
	for _, a := range apps {
		out = append(out, mailclassify.Candidate{JobID: a.JobID, Company: a.Company})
	}
	return out
}
