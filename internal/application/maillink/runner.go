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

	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/application/mailmatch"
	"github.com/strelov1/freehire/internal/platform/outbox"
)

const (
	defaultLeaseSeconds = 120
	defaultBatchSize    = 20
	defaultMaxAttempts  = 3
	// defaultUpstreamGraceDays bounds failures the message did NOT cause, by queue age
	// rather than tries (see messageAtFault). Sized to outlast an outage plus the time
	// to notice; same default as ENRICH_UPSTREAM_GRACE_DAYS.
	defaultUpstreamGraceDays = 14
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
	// Fail records a failed attempt under the given policy and reports whether the entry
	// dead-lettered (reached its bound, will not be retried). The worker's exit code is
	// built from it; without it a queue could dead-letter everything and still exit 0.
	Fail(ctx context.Context, outboxID int64, cause string, policy FailurePolicy) (deadLettered bool, err error)
}

// FailurePolicy is how one failure should be bounded. MessageAtFault picks the bound:
// MaxAttempts counts tries at a possibly impossible task, UpstreamGraceDays waits out
// a dependency expected back. See messageAtFault and FailEmailClassification.
type FailurePolicy struct {
	MessageAtFault    bool
	MaxAttempts       int
	UpstreamGraceDays int
}

// Classifier is the LLM port.
type Classifier interface {
	Classify(ctx context.Context, in mailclassify.Input) (mailclassify.Classification, error)
}

// Learner records the sender of a confidently-classified application email so a
// recurring unknown ATS domain can be learned into the sync allowlist, and the sender of
// non-application mail so a learned domain that is mostly noise falls back out of it.
// Optional: a nil learner disables the feedback loop. Kept minimal so maillink stays
// decoupled from the sync package that implements it.
type Learner interface {
	Learn(ctx context.Context, fromAddr string) error
	Miss(ctx context.Context, fromAddr string) error
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
	// upstreamGraceDays bounds the failures the message did not cause, by the entry's
	// queue age. See messageAtFault.
	upstreamGraceDays int
}

// New builds a Runner with the default lease/batch/threshold tuning.
func New(store Store, classifier Classifier, model string) *Runner {
	return &Runner{
		store:             store,
		classifier:        classifier,
		model:             model,
		cfg:               defaultThresholds,
		leaseSeconds:      defaultLeaseSeconds,
		batchSize:         defaultBatchSize,
		maxAttempts:       defaultMaxAttempts,
		upstreamGraceDays: defaultUpstreamGraceDays,
	}
}

// WithLearner wires the self-learning ATS-domain feedback loop, returning the
// runner for chaining.
func (r *Runner) WithLearner(l Learner) *Runner {
	r.learner = l
	return r
}

// Stats is what a run reports upward. Failed and DeadLettered are mutually
// exclusive: a dead-lettered entry counts only in DeadLettered, matching
// outbox.Outcome. cmd/classify-mail turns them into its exit code, so a quietly
// broken queue is visible to the scheduler.
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

// Run enqueues every unclassified email, then drains the outbox wave by wave
// until it is empty.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	if _, err := r.store.EnqueuePending(ctx); err != nil {
		return Stats{}, err
	}
	// A wave is mostly one user's mail, so its application list is shared (see appCache
	// for why thread links are not). Created once per Run, not per wave.
	cache := appCache{store: r.store, byUser: map[int64][]Application{}}
	result, err := outbox.RunPool(ctx, claimAdapter{store: r.store}, outbox.RunOptions{
		BatchSize:    r.batchSize,
		LeaseSeconds: r.leaseSeconds,
		// Strictly sequential: appCache.byUser is an unsynchronized map, and a later
		// message in a thread must see the same wave's earlier link.
		Concurrency: 1,
	}, func(ctx context.Context, c Claimed) outbox.Outcome {
		err := r.process(ctx, cache, c)
		if err == nil {
			return outbox.Succeeded
		}
		// Everything but the message's own fault is bounded by entry age, so an outage
		// of any length costs nothing permanently.
		deadLettered, ferr := r.store.Fail(ctx, c.OutboxID, err.Error(), FailurePolicy{
			MessageAtFault:    messageAtFault(err),
			MaxAttempts:       r.maxAttempts,
			UpstreamGraceDays: r.upstreamGraceDays,
		})
		if ferr != nil {
			// Dead-letter state is unknown, so it counts as failed; the lease expires either way.
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
// Only applications may be cached: a run's only write to membership is a forward
// stage move on an existing member. Thread links must not be, because Save writes
// job_id onto the message it just linked and a later message in the thread has to
// see it.
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
	// Only spend LLM disambiguation on the ambiguous/unmatched tail; classify either
	// way for the status. resolveLink uses the same predicate (see autoLinks).
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

	model := r.model
	if cls.Model != "" {
		model = cls.Model
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
	}, model); err != nil {
		return err
	}

	// Feed the self-learning cache, but only at the confidence bar already trusted to
	// act (cfg.stage): repeated low-confidence guesses could otherwise reach
	// PromoteThreshold and promote a domain the pipeline treats as too weak. Best-effort:
	// a learn failure must not fail the email.
	//
	// Gmail only, because the learned domain is read off From and feeds which senders a
	// future sync accepts. Gmail authenticates From; hosted mail is whatever SMTP claimed
	// and external mail is whatever the caller pushed, so there anyone could nominate a
	// domain to be trusted.
	if r.learner != nil && c.Source == "gmail" {
		r.learn(ctx, c.FromAddr, cls)
	}
	return nil
}

// learn feeds one Gmail classification to the learner. `other` counts against the sender
// at any confidence: demoting is the safe direction. The From-authentication argument
// applies to misses too, since a forged From could demote a real ATS.
func (r *Runner) learn(ctx context.Context, fromAddr string, cls mailclassify.Classification) {
	switch {
	case cls.Signal == mailclassify.SignalOther:
		if err := r.learner.Miss(ctx, fromAddr); err != nil {
			log.Printf("maillink: miss %q: %v", fromAddr, err)
		}
	case cls.Signal != "" && cls.Confidence >= r.cfg.stage:
		if err := r.learner.Learn(ctx, fromAddr); err != nil {
			log.Printf("maillink: learn %q: %v", fromAddr, err)
		}
	}
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
