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
	ThreadID string
	FromName string
	Subject  string
	Body     string
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
	Fail(ctx context.Context, outboxID int64, cause string, maxAttempts int) error
}

// Classifier is the LLM port.
type Classifier interface {
	Classify(ctx context.Context, in mailclassify.Input) (mailclassify.Classification, error)
}

// Runner drains the outbox.
type Runner struct {
	store        Store
	classifier   Classifier
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

// Run enqueues every unclassified email, then drains the outbox wave by wave
// until it is empty.
func (r *Runner) Run(ctx context.Context) error {
	if _, err := r.store.EnqueuePending(ctx); err != nil {
		return err
	}
	for {
		batch, err := r.store.ClaimBatch(ctx, r.leaseSeconds, r.batchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, c := range batch {
			if err := r.process(ctx, c); err != nil {
				if ferr := r.store.Fail(ctx, c.OutboxID, err.Error(), r.maxAttempts); ferr != nil {
					log.Printf("maillink: fail outbox %d: %v", c.OutboxID, ferr)
				}
			}
		}
	}
}

func (r *Runner) process(ctx context.Context, c Claimed) error {
	apps, err := r.store.Applications(ctx, c.UserID)
	if err != nil {
		return err
	}
	links, err := r.store.ThreadLinks(ctx, c.UserID)
	if err != nil {
		return err
	}

	m := mailmatch.Resolve(
		mailmatch.Email{ThreadID: c.ThreadID, FromName: c.FromName, Subject: c.Subject},
		matchCandidates(apps, links),
	)
	autoLinked := (m.Tier == mailmatch.TierThread || m.Tier == mailmatch.TierName) && m.Confidence >= r.cfg.autoLink

	// Only spend the LLM's disambiguation on the ambiguous/unmatched tail; a
	// confident deterministic match still needs the status, so classify either way.
	var candidates []mailclassify.Candidate
	if !autoLinked {
		candidates = classifyCandidates(apps)
	}
	cls, err := r.classifier.Classify(ctx, mailclassify.Input{
		FromName: c.FromName, Subject: c.Subject, Body: c.Body, Candidates: candidates,
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

	return r.store.Save(ctx, c.OutboxID, c.UserID, Result{
		EmailID:        c.EmailID,
		JobID:          job,
		SuggestedJobID: suggested,
		LinkSource:     source,
		Confidence:     conf,
		Signal:         cls.Signal,
		AdvanceStageTo: advanceTo,
	}, r.model)
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
