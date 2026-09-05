package autoapply

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/platform/outbox"
)

// PreviewStore is the persistence RunPreviews needs: a second claim predicate over the same
// queue table Store already covers, plus the write paths a preview attempt can end in.
// Deliberately not folded into Store — Run's own callers and fakes have no reason to grow
// methods they never call. The one production implementor (cmd/auto-apply's dbStore)
// satisfies both interfaces structurally, the same way it always has for Store alone.
type PreviewStore interface {
	// ClaimForPreview leases up to batch entries that have a tailored CV and no resolved
	// preview yet.
	ClaimForPreview(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// SetPreview persists the resolved answer preview for one entry.
	SetPreview(ctx context.Context, queueID int64, preview ResolvedPreview) error
	// Park records an attempt whose form could not be previewed at all (a captcha-gated
	// provider, an unscannable page) — the same write Store.Park already makes for the
	// structurally identical outcome during a real submission: a park predicts the
	// identical outcome the real submission would hit either way, so sharing blocked_at is
	// correct, not merely convenient.
	Park(ctx context.Context, queueID int64, unmapped []UnmappedField, reason string) error
	// FailPreview records a transient failure for one entry's preview resolution, on its
	// own attempts/failed_at budget — deliberately NOT named Fail and deliberately not
	// Store.Fail: the two passes used to share Store.Fail's own attempts/failed_at columns,
	// which let a transient preview-resolution error (a flaky schema fetch, a browser
	// launch hiccup) spend down the SAME retry budget the real ATS submission depends on,
	// and could dead-letter a row before a submission was ever attempted. See migration
	// 0140 and RecordAutoApplyPreviewFailure.
	FailPreview(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// PreviewStats is what a preview run did.
type PreviewStats struct {
	Resolved int
	// Parked counts attempts whose form could not be previewed at all — not a fault, the
	// same reasoning RunStats.Blocked already documents for the submit pass.
	Parked       int
	Failed       int
	DeadLettered int
}

// Degraded reports whether a preview run deserves an alert, mirroring RunStats.Degraded's
// own reasoning: a parked attempt is the system correctly declining to guess, never a fault.
func (s PreviewStats) Degraded() bool {
	return s.DeadLettered > 0 || (s.Resolved == 0 && s.Parked == 0 && s.Failed > 0)
}

// RunPreviews claims entries that have a tailored CV and no resolved preview yet, resolves
// each one's answer preview through sidecar, and persists it — the second, independent pass
// cmd/auto-apply's own run makes alongside Run, in the same process, reusing the browser
// dependency only that worker has. See openspec/changes/auto-apply-review-tracking/design.md
// for why this runs here rather than in cmd/auto-apply-orchestrate.
func RunPreviews(ctx context.Context, s PreviewStore, answers AnswerSource, sidecar PreviewSidecar, opts RunOptions) (PreviewStats, error) {
	rn := &previewRun{store: s, answers: answers, sidecar: sidecar, opts: opts}
	result, err := outbox.RunPool(ctx, &previewClaimer{store: s}, outbox.RunOptions{
		BatchSize:    opts.BatchSize,
		LeaseSeconds: opts.LeaseSeconds,
		Concurrency:  opts.Concurrency,
		MaxPerRun:    opts.MaxPerRun,
	}, rn.process)
	stats := PreviewStats{
		Resolved:     result.Succeeded,
		Parked:       result.Discarded,
		Failed:       result.Failed,
		DeadLettered: result.DeadLettered,
	}
	if err != nil {
		return stats, fmt.Errorf("claim auto-apply preview attempts: %w", err)
	}
	return stats, nil
}

// previewClaimer mirrors cancelAwareClaimer: guards every Claim call after the first against
// a cancelled ctx, so a cancelled run stops at the next wave boundary.
type previewClaimer struct {
	store PreviewStore
	calls int
}

func (c *previewClaimer) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	if c.calls > 0 && ctx.Err() != nil {
		return nil, nil
	}
	c.calls++
	return c.store.ClaimForPreview(ctx, batch, leaseSeconds)
}

type previewRun struct {
	store   PreviewStore
	answers AnswerSource
	sidecar PreviewSidecar
	opts    RunOptions
}

func (rn *previewRun) process(ctx context.Context, c Claimed) outbox.Outcome {
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

	result, err := rn.sidecar.Preview(callCtx, c, answers)
	if err != nil {
		return rn.fail(ctx, c, err)
	}
	if result.Parked {
		// A form-level park (no specific field named) — the same "reason only, no
		// unmapped list" shape Client.Submit's own captcha/unscannable outcomes use.
		if err := rn.store.Park(ctx, c.QueueID, nil, result.Reason); err != nil {
			log.Printf("auto-apply: record parked preview attempt %d (job %d): %v", c.QueueID, c.JobID, err)
		}
		return outbox.Discarded
	}
	if err := rn.store.SetPreview(ctx, c.QueueID, result.Preview); err != nil {
		return rn.fail(ctx, c, fmt.Errorf("persist resolved preview: %w", err))
	}
	return outbox.Succeeded
}

func (rn *previewRun) fail(ctx context.Context, c Claimed, err error) outbox.Outcome {
	dead, failErr := rn.store.FailPreview(ctx, c.QueueID, err.Error(), rn.opts.MaxAttempts)
	if failErr != nil {
		log.Printf("auto-apply: record preview failure for queue entry %d: %v", c.QueueID, failErr)
	} else if dead {
		log.Printf("auto-apply: queue entry %d (job %d) dead-lettered resolving its preview: %v", c.QueueID, c.JobID, err)
	}
	if failErr == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
}
