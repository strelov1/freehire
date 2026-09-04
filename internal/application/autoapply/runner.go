// Package autoapply drains the queued submission attempts in auto_apply_queue: claim a
// wave, assemble each candidate's known answers, ask the browser-automation sidecar to
// submit the application, and record what happened. It is deliberately incurious about ATS
// specifics — the sidecar owns DOM structure, react-select verification, upload
// confirmation and submission text markers, exactly as internal/applyform's fetchers own
// each platform's own crawl quirks and this package owns none of them either.
//
// What populates the queue is out of scope here (see
// openspec/changes/auto-apply-worker/design.md) — Run only ever claims what is already
// there.
package autoapply

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/platform/outbox"
)

// Claimed is one submission attempt leased to this run.
type Claimed struct {
	QueueID int64
	UserID  int64
	JobID   int64
	// Provider is the ATS this posting came from (jobs.source), the sidecar's routing key
	// — the same vocabulary internal/applyform's Provider field already uses.
	Provider string
	// ExternalID is the board-namespaced posting id (sources.NamespaceExternalID:
	// "board:id") — what internal/applyform's own schema fetchers need to reuse their
	// existing per-provider API calls.
	ExternalID string
	// JobURL is the posting's own address, the page the sidecar opens to scan and fill the
	// application form.
	JobURL string
	// TailoredCVID is the candidate's approved tailored CV for this vacancy (openspec/
	// changes/auto-apply-tailored-resume). Store.Claim only ever returns entries that carry
	// one — an approved review is part of the claim predicate — so this is never the zero
	// value for a Claimed the sidecar sees; it is what internal/api/atsapply renders to a
	// PDF and attaches to the form's résumé upload field.
	TailoredCVID uuid.UUID
}

// UnmappedField is one required question on the job's form that no known answer covers —
// the reason a sidecar attempt parked rather than submitting.
type UnmappedField struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Reason   string `json:"reason"`
}

// SubmitStatus is the sidecar's verdict for one attempt.
type SubmitStatus string

const (
	// StatusApplied means the sidecar submitted the application for real.
	StatusApplied SubmitStatus = "applied"
	// StatusParked means every required question was NOT resolved, so the sidecar
	// touched nothing on the employer's form.
	StatusParked SubmitStatus = "parked"
	// StatusUnconfirmed means the sidecar pressed submit but could not tell whether the
	// employer accepted it — neither a confirmation nor an explicit refusal appeared.
	// This is NOT a plain transient failure: the click may well have gone through, so
	// retrying it through the ordinary attempts budget risks a second real submission —
	// exactly the "never twice" requirement the forced dead-letter in recordApplied
	// exists to protect on the DB-write side. See runner.go's process/fail handling: an
	// unconfirmed result is dead-lettered immediately, the same way a lost post-submit
	// record is, rather than spending a retry.
	StatusUnconfirmed SubmitStatus = "unconfirmed"
)

// SidecarResult is what the sidecar returned for one attempt.
type SidecarResult struct {
	Status SubmitStatus
	// Unmapped is set on StatusParked: which required questions stopped the attempt.
	Unmapped []UnmappedField
	// Reason is a short summary of why the attempt parked, independent of the per-field
	// detail in Unmapped (e.g. "captcha required").
	Reason string
}

// SidecarClient submits one application attempt through the browser-automation sidecar.
// The real implementation is internal/atsapply, driving a headless browser in-process
// (see design.md's "chromedp, not a Python/Patchright sidecar" decision — this interface
// predates and is unaffected by that choice); tests use a fake. Every ATS-specific decision
// — what the DOM actually requires, whether an answer resolves a field, whether the
// submission was confirmed — is made on the other side of this call; this package only
// interprets the verdict.
//
// Submit takes the whole Claimed rather than its individual fields, mirroring
// internal/applyform.Fetcher.Fetch: what a submission needs is not the same for every
// provider (Greenhouse and Ashby's schema fetch is keyed by ExternalID's board:posting-id,
// not by JobURL alone), so the seam should not have to grow a parameter every time a new
// provider needs one more piece of the claim.
type SidecarClient interface {
	Submit(ctx context.Context, c Claimed, answers map[string]string) (SidecarResult, error)
}

// AnswerSource supplies the candidate's known answers for one attempt — identity and
// work-authorization facts today (see design.md: no Tier C/LLM-drafted answers in this
// package yet). A question the returned map has no entry for always parks rather than being
// guessed.
type AnswerSource interface {
	Answers(ctx context.Context, userID int64) (map[string]string, error)
}

// Store is the persistence the runner needs. The real implementation wraps the generated
// queries and a pool; tests use a fake. The port hides the pool and the transaction
// boundaries, not the queue's semantics.
type Store interface {
	// Claim leases up to batch live, unleased attempts.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Submit records a successful application and retires the queue entry, atomically —
	// so an attempt cannot be both recorded and left queued, nor retired without being
	// recorded. Composes jobtracking's application write with the queue delete under one
	// transaction/lock (design.md), so a double-claim of the same row cannot double-write.
	Submit(ctx context.Context, c Claimed) error
	// Park records an attempt no known answer could complete, without retrying it — it
	// needs new data, not another try.
	Park(ctx context.Context, queueID int64, unmapped []UnmappedField, reason string) error
	// Fail records a transient failure for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// RunOptions are the per-run knobs, the same shape internal/applyform's RunOptions takes.
type RunOptions struct {
	BatchSize    int
	LeaseSeconds int
	MaxAttempts  int
	Concurrency  int
	MaxPerRun    int
	CallTimeout  time.Duration
}

// RunStats is what the run did.
type RunStats struct {
	Applied int
	// Blocked counts attempts the sidecar could not fully resolve — parked, not failed:
	// nothing was wrong, a required question simply had no known answer yet.
	Blocked      int
	Failed       int
	DeadLettered int
}

// Degraded reports whether a run deserves an alert. A parked attempt is not a fault — it is
// the system correctly declining to guess — so it never counts here, the same reasoning
// internal/applyform's RunStats.Degraded applies to a posting that is simply gone. What does
// deserve attention is a dead letter (work abandoned, or — per the forced dead-letter path
// below — a submission whose local record was lost) and a run that failed everything it
// touched.
func (s RunStats) Degraded() bool {
	return s.DeadLettered > 0 || (s.Applied == 0 && s.Blocked == 0 && s.Failed > 0)
}

// Run drains the queue and returns. Claim a wave, resolve each attempt against the
// candidate's known answers through the sidecar, record the outcome, and keep going until a
// wave comes back empty.
func Run(ctx context.Context, s Store, answers AnswerSource, sidecar SidecarClient, opts RunOptions) (RunStats, error) {
	rn := &run{store: s, answers: answers, sidecar: sidecar, opts: opts}
	result, err := outbox.RunPool(ctx, &cancelAwareClaimer{store: s}, outbox.RunOptions{
		BatchSize:    opts.BatchSize,
		LeaseSeconds: opts.LeaseSeconds,
		Concurrency:  opts.Concurrency,
		MaxPerRun:    opts.MaxPerRun,
	}, rn.process)
	stats := RunStats{
		Applied:      result.Succeeded,
		Blocked:      result.Discarded,
		Failed:       result.Failed,
		DeadLettered: result.DeadLettered,
	}
	if err != nil {
		return stats, fmt.Errorf("claim auto-apply attempts: %w", err)
	}
	return stats, nil
}

// cancelAwareClaimer mirrors internal/applyform's: guards every Claim call after the first
// against a cancelled ctx, so a cancelled run stops at the next wave boundary instead of
// burning the remaining backlog's retry attempts on a shutdown.
type cancelAwareClaimer struct {
	store Store
	calls int
}

func (c *cancelAwareClaimer) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	if c.calls > 0 && ctx.Err() != nil {
		return nil, nil
	}
	c.calls++
	return c.store.Claim(ctx, batch, leaseSeconds)
}

type run struct {
	store   Store
	answers AnswerSource
	sidecar SidecarClient
	opts    RunOptions
}

// process resolves one attempt end to end and reports what happened; outbox.RunPool tallies
// the result. Blocked reuses outbox.Discarded — the same "left the active claim rotation
// without being retried" meaning internal/applyform gives it for a gone posting, even
// though what it means for THIS queue is "needs new data", not "the platform lost it".
func (rn *run) process(ctx context.Context, c Claimed) outbox.Outcome {
	answers, err := rn.answers.Answers(ctx, c.UserID)
	if err != nil {
		return rn.fail(ctx, c, fmt.Errorf("assemble answers for user %d: %w", c.UserID, err))
	}

	callCtx := ctx
	if rn.opts.CallTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, rn.opts.CallTimeout)
		defer cancel()
	}

	result, err := rn.sidecar.Submit(callCtx, c, answers)
	if err != nil {
		return rn.fail(ctx, c, err)
	}

	switch result.Status {
	case StatusApplied:
		return rn.recordApplied(ctx, c)
	case StatusParked:
		if err := rn.store.Park(ctx, c.QueueID, result.Unmapped, result.Reason); err != nil {
			log.Printf("auto-apply: record parked attempt %d (job %d): %v", c.QueueID, c.JobID, err)
		}
		return outbox.Discarded
	case StatusUnconfirmed:
		return rn.deadLetterImmediately(ctx, c, "submission unconfirmed: neither a confirmation nor a refusal was seen")
	default:
		return rn.fail(ctx, c, fmt.Errorf("sidecar returned unknown status %q", result.Status))
	}
}

// recordApplied handles the one outcome that must never take the ordinary retry path: the
// sidecar has already submitted the application for real, so a failure to record that
// locally cannot be treated like any other transient error. The normal Fail() path bumps a
// retry counter and eventually re-arms the row for reclaim — which here would mean calling
// the sidecar again for a job it already applied to, submitting a second time. Forcing an
// immediate dead-letter (maxAttempts=1) instead means the row never becomes reclaimable
// again; it surfaces as work a human must look at, not as a queue that quietly retries its
// way into a duplicate application.
func (rn *run) recordApplied(ctx context.Context, c Claimed) outbox.Outcome {
	err := rn.store.Submit(ctx, c)
	if err == nil {
		return outbox.Succeeded
	}
	log.Printf("auto-apply: CRITICAL lost the local record of a real submission for queue entry %d (user %d, job %d): %v", c.QueueID, c.UserID, c.JobID, err)
	return rn.deadLetterImmediately(ctx, c, fmt.Sprintf("submitted but failed to record: %v", err))
}

// deadLetterImmediately forces a dead-letter (maxAttempts=1) rather than spending the run's
// configured retry budget — the shared ending for every outcome where retrying normally
// risks a second real submission to the employer: a lost post-submit record
// (recordApplied) and an unconfirmed submission (process's StatusUnconfirmed case) both
// land here. The row never becomes reclaimable again; it surfaces as work a human must look
// at, not as a queue that quietly retries its way into a duplicate application.
func (rn *run) deadLetterImmediately(ctx context.Context, c Claimed, reason string) outbox.Outcome {
	if _, failErr := rn.store.Fail(ctx, c.QueueID, reason, 1); failErr != nil {
		// The write that was supposed to make this row terminal did not happen: it
		// stays claimed, its lease still runs out on its own, and a later run can
		// reclaim it as an ordinary pending attempt — reopening exactly the "might
		// submit twice" risk this whole path exists to close. CRITICAL because this
		// is not a case retrying resolves; only a person can decide whether to block
		// the row by hand before the lease expires.
		log.Printf("auto-apply: CRITICAL could not dead-letter queue entry %d; it stays reclaimable and may submit twice: %v", c.QueueID, failErr)
		return outbox.Failed
	}
	return outbox.DeadLettered
}

func (rn *run) fail(ctx context.Context, c Claimed, err error) outbox.Outcome {
	dead, failErr := rn.store.Fail(ctx, c.QueueID, err.Error(), rn.opts.MaxAttempts)
	if failErr != nil {
		log.Printf("auto-apply: record failure for queue entry %d: %v", c.QueueID, failErr)
	} else if dead {
		log.Printf("auto-apply: queue entry %d (job %d) dead-lettered: %v", c.QueueID, c.JobID, err)
	}
	if failErr == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
}
