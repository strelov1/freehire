package autoapply

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Enqueuer creates one auto-apply attempt for a candidate and a job: the use case behind the
// job page's route and the MCP auto_apply_start tool, so the two doors cannot disagree about
// its gates. Its ports are the stores it reads and writes; the transports own every
// user-facing sentence and status (they map the typed errors below).
type Enqueuer struct {
	// FindJob resolves the job behind a slug or link. An unknown one is pgx.ErrNoRows.
	FindJob func(ctx context.Context, slug string) (db.Job, error)
	// Queue holds the queue and application reads and the insert. *db.Queries satisfies it.
	Queue EnqueueStore
	// BaseCV reports whether the candidate has a base CV; Seeder says whether a résumé can
	// seed one. Either nil means the CV builder is not wired: start refuses as RefusalNoCVBuilder.
	BaseCV func(ctx context.Context, userID int64) (bool, error)
	Seeder cv.Seeder
	// Blockers evaluates the candidate's hard constraints against a job, the same
	// deterministic evaluation the match analysis uses. Nil, or an empty answer, means no
	// blocker: absence of evidence is never evidence of ineligibility.
	Blockers func(ctx context.Context, userID int64, job db.Job) []hardconstraint.Blocker
	// Plans meters the attempt. Nil means auto-apply is not available (RefusalNoPlanMeter).
	Plans Allowance
	// Tracker puts the job on the candidate's board. Nil skips it.
	Tracker Tracker
	// PublishSubmit starts the orchestrator run for the new entry, best-effort. Nil publishes nothing.
	PublishSubmit func(ctx context.Context, queueID int64)
}

// EnqueueStore is the queue persistence Enqueuer needs. *db.Queries satisfies it.
type EnqueueStore interface {
	GetUserJobApplied(ctx context.Context, arg db.GetUserJobAppliedParams) (bool, error)
	EnqueueAutoApply(ctx context.Context, arg db.EnqueueAutoApplyParams) (int64, error)
	GetAutoApplyQueueEntryForJob(ctx context.Context, arg db.GetAutoApplyQueueEntryForJobParams) (db.GetAutoApplyQueueEntryForJobRow, error)
	GetApplicationStage(ctx context.Context, arg db.GetApplicationStageParams) (pgtype.Text, error)
}

// Allowance is the plan meter's charge. *plan.Store satisfies it.
type Allowance interface {
	Consume(ctx context.Context, userID int64, feature plan.Feature, ref string) (plan.Decision, error)
}

// Tracker records a stage change on the candidate's board. *jobtracking.Service satisfies it.
// A port rather than the service: jobtracking reads this package's review info, so importing
// it back would be a cycle.
type Tracker interface {
	Track(ctx context.Context, userID int64, slug string, stage, notes *string, source string) error
}

// Start creates one auto-apply attempt for userID and the job behind slug, or reports why it
// may not.
//
// Gates, in order: the job must be on a queueable ATS, the caller must have a base CV or a
// résumé to seed one, the pair must not already be applied to, and allowance must be left
// today. The allowance is LAST so every other refusal costs nothing. A repeat for a pair with
// a live, undecided entry succeeds without a second row; a repeat for a pair the candidate
// declined is refused permanently.
//
// An unknown slug comes back as pgx.ErrNoRows.
func (e *Enqueuer) Start(ctx context.Context, userID int64, slug string) error {
	job, err := e.FindJob(ctx, slug)
	if err != nil {
		return err
	}
	if !Queueable(job.Source) {
		return &Refusal{Reason: RefusalUnsupportedSource}
	}

	if e.BaseCV == nil || e.Seeder == nil {
		return &Refusal{Reason: RefusalNoCVBuilder}
	}
	// The base CV need not exist yet: cvStore.Tailor seeds one from the extracted résumé, so
	// this asks Tailor's own question. Requiring the cvs row refused candidates who uploaded
	// a résumé but never opened the CV builder.
	hasBase, err := e.BaseCV(ctx, userID)
	if err != nil {
		return err
	}
	if !hasBase {
		if _, hasResume, err := e.Seeder.Structured(ctx, userID); err != nil {
			return err
		} else if !hasResume {
			return &Refusal{Reason: RefusalNoResume}
		}
	}

	// cmd/auto-apply/store.go's Submit deletes the queue row in the same transaction it marks
	// the job applied, so ON CONFLICT cannot catch a re-click after a real submission; this check does.
	if applied, err := e.Queue.GetUserJobApplied(ctx, db.GetUserJobAppliedParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	}); err != nil {
		return err
	} else if applied {
		return &Refusal{Reason: RefusalAlreadyApplied}
	}

	// Eligibility gate: refuse (shadow mode: only log) a pair the candidate's known
	// work-authorization or location evidence conflicts with. Runs before the charge.
	if b, mismatched := e.eligibilityBlocker(ctx, userID, job); mismatched {
		enforced := EligibilityEnforce()
		// Logged in both modes, tagged by category, so the false-positive rate stays observable.
		verb := "would refuse"
		if enforced {
			verb = "refused"
		}
		log.Printf("auto-apply: eligibility gate %s user %d job %d (%s): %s", verb, userID, job.ID, b.Category, b.Reason)
		if enforced {
			return &Refusal{Reason: RefusalEligibilityMismatch, Detail: b.Reason}
		}
	}

	if err := e.charge(ctx, userID, job.ID); err != nil {
		return err
	}

	e.ensureTracked(ctx, userID, job)

	id, err := e.Queue.EnqueueAutoApply(ctx, db.EnqueueAutoApplyParams{UserID: userID, JobID: job.ID})
	switch {
	case err == nil:
		// Only the request whose INSERT won may publish: an idempotent replay publishing
		// too would let the orchestrator see the same auto-apply/submit more than once.
		if e.PublishSubmit != nil {
			e.PublishSubmit(ctx, id)
		}
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		existing, ferr := e.Queue.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{
			UserID: userID, JobID: job.ID,
		})
		if ferr != nil {
			return ferr
		}
		switch EntryStatusOf(existing.ReviewDecision, existing.FailedAt, existing.BlockedAt) {
		case EntryDeclined:
			return &Refusal{Reason: RefusalAlreadyDeclined}
		case EntryFailed:
			if existing.SubmitUncertainAt.Valid {
				return &Refusal{Reason: RefusalSubmitUncertain}
			}
			// cmd/auto-apply gave up on this entry; "queued" would hide that it is stuck. It is retryable.
			return &Refusal{Reason: RefusalStuck}
		default:
			// A live, undecided entry exists (page reload, second tab): idempotent success, not a fault.
			return nil
		}
	default:
		return err
	}
}

// EntryStatus is the three-value status of one live queue entry as both the job page's
// status overlay and Start's conflict path read it (see EntryStatusOf). Coarser than Status: every entry still
// working towards a decision is EntryQueued, whatever stage it is at.
type EntryStatus string

const (
	// EntryQueued: the entry is live and undecided.
	EntryQueued EntryStatus = "queued"
	// EntryFailed: cmd/auto-apply gave up on the entry — dead-lettered (failed_at) or parked for
	// missing form data (blocked_at). Both leave review_decision 'approved', so without those
	// columns a stuck attempt would read as queued forever.
	EntryFailed EntryStatus = "failed"
	// EntryDeclined: the candidate declined the tailored CV.
	EntryDeclined EntryStatus = "declined"
)

// EntryStatusOf derives the entry's status from the columns GetAutoApplyQueueEntryForJob reads.
// Declined is checked first: DeclineAutoApplyReview also sets blocked_at, so checking
// blocked first would report a decline as a failure.
func EntryStatusOf(reviewDecision pgtype.Text, failedAt, blockedAt pgtype.Timestamptz) EntryStatus {
	if reviewDecision.Valid && reviewDecision.String == string(EntryDeclined) {
		return EntryDeclined
	}
	if failedAt.Valid || blockedAt.Valid {
		return EntryFailed
	}
	return EntryQueued
}

// Refusal is Start's answer when it may not create an attempt: a state of the candidate, the
// job or the account, never a fault. Reason names which, so each transport maps it to its own
// status and sentence. Detail carries the eligibility blocker's own reason and is empty for
// every other reason.
type Refusal struct {
	Reason RefusalReason
	Detail string
}

func (r *Refusal) Error() string { return "autoapply: start refused: " + string(r.Reason) }

// RefusalReason is why Start refused.
type RefusalReason string

const (
	// RefusalUnsupportedSource: the posting is not on an ATS auto-apply can queue against.
	RefusalUnsupportedSource RefusalReason = "unsupported_source"
	// RefusalNoCVBuilder: this deployment has no CV builder, so nothing can tailor the attempt.
	RefusalNoCVBuilder RefusalReason = "no_cv_builder"
	// RefusalNoResume: the candidate has neither a base CV nor a résumé to seed one.
	RefusalNoResume RefusalReason = "no_resume"
	// RefusalAlreadyApplied: the candidate already applied to this job.
	RefusalAlreadyApplied RefusalReason = "already_applied"
	// RefusalEligibilityMismatch: the candidate's known work authorization or location
	// conflicts with the job's requirements (enforce mode only; see EligibilityEnforce).
	RefusalEligibilityMismatch RefusalReason = "eligibility_mismatch"
	// RefusalAlreadyDeclined: the pair's existing attempt was declined, which is permanent.
	RefusalAlreadyDeclined RefusalReason = "already_declined"
	// RefusalSubmitUncertain: the existing attempt stopped and may have reached the employer.
	RefusalSubmitUncertain RefusalReason = "submit_uncertain"
	// RefusalStuck: the existing attempt stopped; it is retried, never started anew.
	RefusalStuck RefusalReason = "stuck"
	// RefusalNoPlanMeter: this deployment cannot charge the attempt, so it does not start one.
	RefusalNoPlanMeter RefusalReason = "no_plan_meter"
)

// AllowanceRefusal is Start's answer when the day's allowance is spent. Its own type because
// it carries the Decision the wire renders, unlike the plain refusals above.
type AllowanceRefusal struct{ Decision plan.Decision }

func (r *AllowanceRefusal) Error() string { return "autoapply: start refused: allowance spent" }

// queueableSources are the ATS providers auto-apply can queue an attempt against.
// Which of them can SUBMIT depends on the deployment's executor and enforcement
// (atsapply.SubmittableProviders); an attempt that cannot still tailors and gets reviewed,
// then parks with a named reason in cmd/auto-apply.
// Widening this list is a product decision: each attempt spends a daily tailoring turn.
var queueableSources = map[string]bool{
	"greenhouse": true,
	"ashby":      true,
	"workable":   true,
	"lever":      true,
	"recruitee":  true,
}

// Queueable reports whether auto-apply may queue an attempt against a posting from source.
func Queueable(source string) bool {
	return queueableSources[source]
}

// EligibilityEnforce gates whether an eligibility mismatch (see Enqueuer.eligibilityBlocker)
// refuses enqueueing or only logs. OFF (shadow-only) by default like PLAN_ENFORCE: a false
// positive blocks a paying candidate from a job they are eligible for.
//
// Reads os.Getenv on every call: memoizing would freeze the first value for the process,
// so t.Setenv could never change it in a later test.
func EligibilityEnforce() bool {
	return os.Getenv("AUTO_APPLY_ELIGIBILITY_ENFORCE") == "1"
}

// eligibilityBlocker reports the first unmet work-authorization or location-and-work-mode
// blocker for the (userID, job) pair. Degrades to "no blocker" when evaluation cannot run
// (no evaluator configured): absence of evidence is never evidence of ineligibility.
func (e *Enqueuer) eligibilityBlocker(ctx context.Context, userID int64, job db.Job) (hardconstraint.Blocker, bool) {
	if e.Blockers == nil {
		return hardconstraint.Blocker{}, false
	}
	return firstEligibilityBlocker(e.Blockers(ctx, userID, job))
}

// firstEligibilityBlocker picks the first unmet work-authorization or location-and-work-mode
// entry among the computed blockers. Other categories (experience, education, ...) bear on
// fit, not on misrepresenting the candidate.
func firstEligibilityBlocker(blockers []hardconstraint.Blocker) (hardconstraint.Blocker, bool) {
	for _, b := range blockers {
		if b.Met {
			continue
		}
		if b.Category == hardconstraint.CategoryWorkAuth || b.Category == hardconstraint.CategoryLocationWorkMode {
			return b, true
		}
	}
	return hardconstraint.Blocker{}, false
}

// charge spends one unit of the caller's daily auto-apply allowance for this posting. A spent
// allowance comes back as *AllowanceRefusal.
//
// The reference is the POSTING's id, so a second request for the same job reads as already
// charged (double click or retry); Consume is idempotent by reference.
//
// Unlike other metered features, a failure to CHARGE does not let the action through: this
// submits a real application to a real employer under the candidate's name.
func (e *Enqueuer) charge(ctx context.Context, userID, jobID int64) error {
	if e.Plans == nil {
		return &Refusal{Reason: RefusalNoPlanMeter}
	}

	ref := "job:" + strconv.FormatInt(jobID, 10)
	d, err := e.Plans.Consume(ctx, userID, plan.FeatureAutoApply, ref)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, plan.ErrRefused):
		return &AllowanceRefusal{Decision: d}
	default:
		// Wrapped, not an unavailable: the fault must reach the exception log.
		return fmt.Errorf("charge auto-apply for job %d: %w", jobID, err)
	}
}

// ensureTracked puts job on the caller's tracker board at stage `preparing` when it is not
// there, so an attempt is visible before it succeeds. Never moves a job that already has a
// stage (a NULL stage or no row is the only trigger).
//
// Best-effort: a failure must never cost the candidate their attempt.
func (e *Enqueuer) ensureTracked(ctx context.Context, userID int64, job db.Job) {
	if e.Tracker == nil {
		return
	}
	current, err := e.Queue.GetApplicationStage(ctx, db.GetApplicationStageParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("auto-apply: reading current stage for user %d job %d: %v", userID, job.ID, err)
		return
	}
	if current.Valid {
		return
	}
	stage := "preparing"
	if err := e.Tracker.Track(ctx, userID, job.PublicSlug, &stage, nil, "auto_apply"); err != nil {
		log.Printf("auto-apply: tracking job %d for user %d: %v", job.ID, userID, err)
	}
}
