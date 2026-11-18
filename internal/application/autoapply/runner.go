// Package autoapply drains the queued submission attempts in auto_apply_queue: claim a
// wave, assemble each candidate's known answers, ask the browser-automation sidecar to submit
// the application, and record what happened. It is deliberately incurious about ATS
// specifics: the sidecar owns DOM structure, react-select verification, upload confirmation
// and submission text markers. What populates the queue is out of scope; Run only claims what
// is already there.
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
	// Provider is the ATS this posting came from (jobs.source), the sidecar's routing key; same
	// vocabulary as internal/applyform's Provider.
	Provider string
	// ExternalID is the board-namespaced posting id (sources.NamespaceExternalID: "board:id"),
	// which internal/applyform's schema fetchers reuse.
	ExternalID string
	// JobURL is the posting's own address, the page the sidecar opens to scan and fill the
	// application form.
	JobURL string
	// TailoredCVID is the candidate's approved tailored CV for this vacancy. Store.Claim only
	// returns entries that carry one (an approved review is part of the claim predicate), so it
	// is never zero here; internal/api/atsapply renders it to the PDF for the résumé upload.
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
	// StatusUnconfirmed means the sidecar could not tell whether the employer accepted the
	// application: neither a confirmation nor an explicit refusal appeared. A submission may
	// have gone through (e.g. an Enter-triggered submit, see internal/api/atsapply/fill.go's
	// formStillPresent guard), so it is NOT retried: process dead-letters it immediately, like a
	// lost post-submit record, to keep "never twice".
	StatusUnconfirmed SubmitStatus = "unconfirmed"
	// StatusCaptchaRefused means the board said it could not verify the submission, so no
	// application was created. It is the ONE post-submit outcome safe to retry: every other
	// uncertainty may mean the employer already has the application.
	StatusCaptchaRefused SubmitStatus = "captcha_refused"
)

// captchaMaxAttempts is the retry budget for StatusCaptchaRefused, far more generous than
// the run's ordinary MaxAttempts. An invisible captcha cannot be configured away: nothing
// about the browser setup moved the odds, so success depends on how many times we ask, and
// each ask is free because no application was created. 20 asks against a 5-minute timer is
// about an hour and a half; bounded so a closed posting is eventually reported as given up.
const captchaMaxAttempts = 20

// SidecarResult is what the sidecar returned for one attempt.
type SidecarResult struct {
	Status SubmitStatus
	// Unmapped is set on StatusParked: which required questions stopped the attempt.
	Unmapped []UnmappedField
	// Reason is a short summary of why the attempt parked, independent of the per-field
	// detail in Unmapped (e.g. "captcha required").
	Reason string
	// RetryBudget is how many times THIS sidecar thinks its outcome is worth repeating,
	// overriding the runner's default. Zero means no opinion (the free browser path keeps
	// captchaMaxAttempts); the cloud path names a smaller number because each ask is billed.
	RetryBudget int
}

// SidecarClient submits one application attempt through the browser-automation sidecar. The
// real implementation is internal/atsapply, driving a headless browser in-process; tests use
// a fake. Every ATS-specific decision is made on the other side of this call; this package
// only interprets the verdict.
//
// Submit takes the whole Claimed, like internal/applyform.Fetcher.Fetch, because what a
// submission needs differs by provider (Greenhouse and Ashby key on ExternalID, not JobURL).
type SidecarClient interface {
	Submit(ctx context.Context, c Claimed, answers map[string]string) (SidecarResult, error)
}

// AnswerSource supplies the candidate's known answers for one attempt: identity and
// work-authorization facts. A question with no entry always parks rather than being guessed.
type AnswerSource interface {
	Answers(ctx context.Context, userID int64) (map[string]string, error)
}

// Store is the persistence the runner needs; the real one wraps the generated queries and a
// pool, tests use a fake. The port hides the pool and transactions, not the queue's semantics.
type Store interface {
	// Claim leases up to batch live, unleased attempts.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Submit records a successful application and retires the queue entry atomically, composing
	// jobtracking's application write with the queue delete under one transaction/lock so a
	// double-claim cannot double-write.
	Submit(ctx context.Context, c Claimed) error
	// Park records an attempt no known answer could complete, without retrying it — it
	// needs new data, not another try.
	Park(ctx context.Context, queueID int64, unmapped []UnmappedField, reason string) error
	// Fail records a transient failure for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
	// FailCaptcha counts a captcha refusal on its own counter, not the ordinary attempts
	// budget. Not Fail with a different maxAttempts: the two would share a column, and the
	// generous captcha budget would spend down the one a transient error depends on. Same
	// separation as RecordAutoApplyPreviewFailure.
	FailCaptcha(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
	// DeadLetterUncertain dead-letters an attempt whose submission may already have
	// reached the employer, and marks it so (auto_apply_queue.submit_uncertain_at). The
	// mark is what keeps a candidate's retry from re-arming it: Fail at its last attempt
	// looks identical to an ordinary exhausted budget, which IS safe to retry.
	DeadLetterUncertain(ctx context.Context, queueID int64, reason string) error
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

// Degraded reports whether a run deserves an alert. A parked attempt is the system
// correctly declining to guess, so it never counts. A dead letter (including a submission
// whose local record was lost) or a run that failed everything it touched does.
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

// cancelAwareClaimer mirrors internal/applyform's: it checks for a cancelled ctx before
// every Claim after the first, so a cancelled run stops at the next wave boundary instead of
// burning the backlog's retries.
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

// process resolves one attempt end to end; outbox.RunPool tallies the result. Blocked
// reuses outbox.Discarded ("left the rotation without a retry"); here it means "needs new
// data".
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
	case StatusCaptchaRefused:
		return rn.failCaptcha(ctx, c, result.Reason, result.RetryBudget)
	default:
		return rn.fail(ctx, c, fmt.Errorf("sidecar returned unknown status %q", result.Status))
	}
}

// recordApplied handles the outcome that must never take the ordinary retry path: the
// sidecar already submitted for real, so a failed local record is not a transient error.
// Fail() would re-arm the row and the sidecar would submit a second time; forcing an
// immediate dead-letter (maxAttempts=1) surfaces it for a human instead.
func (rn *run) recordApplied(ctx context.Context, c Claimed) outbox.Outcome {
	err := rn.store.Submit(ctx, c)
	if err == nil {
		return outbox.Succeeded
	}
	log.Printf("auto-apply: CRITICAL lost the local record of a real submission for queue entry %d (user %d, job %d): %v", c.QueueID, c.UserID, c.JobID, err)
	return rn.deadLetterImmediately(ctx, c, fmt.Sprintf("submitted but failed to record: %v", err))
}

// deadLetterImmediately forces a dead-letter rather than spending the retry budget: the
// shared ending where retrying risks a second real submission (a lost post-submit record, an
// unconfirmed submission). The row never becomes reclaimable; a human must look at it.
func (rn *run) deadLetterImmediately(ctx context.Context, c Claimed, reason string) outbox.Outcome {
	if failErr := rn.store.DeadLetterUncertain(ctx, c.QueueID, reason); failErr != nil {
		// The write that should have made this row terminal failed: it stays claimed, its lease
		// runs out, and a later run could reclaim it as an ordinary attempt, reopening the
		// double-submit risk. CRITICAL because retrying cannot fix it; only a person can block the
		// row before the lease expires.
		log.Printf("auto-apply: CRITICAL could not dead-letter queue entry %d; it stays reclaimable and may submit twice: %v", c.QueueID, failErr)
		return outbox.Failed
	}
	return outbox.DeadLettered
}

// failCaptcha records a captcha refusal against its own budget. Otherwise an ordinary
// failure: the entry stays claimable and running out dead-letters it, which tells the
// candidate we stopped asking.
func (rn *run) failCaptcha(ctx context.Context, c Claimed, boardSaid string, budget int) outbox.Outcome {
	reason := "captcha refused the submission, so no application was created"
	if boardSaid != "" {
		reason = fmt.Sprintf("%s: %s", reason, boardSaid)
	}
	// A sidecar that pays for its own asks names how many are worth making; one that does
	// not keeps the measured default.
	if budget <= 0 {
		budget = captchaMaxAttempts
	}
	dead, failErr := rn.store.FailCaptcha(ctx, c.QueueID, reason, budget)
	if failErr != nil {
		log.Printf("auto-apply: record captcha refusal for queue entry %d: %v", c.QueueID, failErr)
	} else if dead {
		log.Printf("auto-apply: queue entry %d (job %d) dead-lettered after %d captcha refusals: %s", c.QueueID, c.JobID, budget, reason)
	}
	if failErr == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
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
