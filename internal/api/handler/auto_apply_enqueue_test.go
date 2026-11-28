package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// Every refusal the enqueue use case answers with maps to the status and sentence the route
// always answered — and, because toolError runs the same function, to the sentence and
// error code an agent gets from auto_apply_start.
func TestAutoApplyEnqueueError_MapsEveryRefusal(t *testing.T) {
	for _, tc := range []struct {
		reason     autoapply.RefusalReason
		detail     string
		wantStatus int
		wantMsg    string
		wantReason string
	}{
		{autoapply.RefusalUnsupportedSource, "", fiber.StatusBadRequest, "auto-apply cannot queue an attempt against this posting's ATS", "ats not supported for auto-apply"},
		{autoapply.RefusalNoCVBuilder, "", fiber.StatusServiceUnavailable, "the assistant is not available", ""},
		{autoapply.RefusalNoResume, "", fiber.StatusConflict, "add a résumé before using auto-apply", "no resume for auto-apply"},
		{autoapply.RefusalAlreadyApplied, "", fiber.StatusConflict, "already applied to this job", "already applied"},
		{autoapply.RefusalEligibilityMismatch, "no visa sponsorship", fiber.StatusConflict, "this job's requirements do not match your profile: no visa sponsorship", ""},
		{autoapply.RefusalAlreadyDeclined, "", fiber.StatusConflict, "this auto-apply attempt was already declined", ""},
		{autoapply.RefusalSubmitUncertain, "", fiber.StatusConflict, autoApplyUncertainMessage, ""},
		{autoapply.RefusalStuck, "", fiber.StatusConflict, "this auto-apply attempt stopped — retry it rather than starting a new one", ""},
		{autoapply.RefusalNoPlanMeter, "", fiber.StatusServiceUnavailable, "auto-apply is not available", ""},
	} {
		t.Run(string(tc.reason), func(t *testing.T) {
			err := autoApplyEnqueueError(&autoapply.Refusal{Reason: tc.reason, Detail: tc.detail})
			var fe *fiber.Error
			if !errors.As(err, &fe) {
				t.Fatalf("not a *fiber.Error: %v", err)
			}
			if fe.Code != tc.wantStatus || fe.Message != tc.wantMsg {
				t.Errorf("= %d %q, want %d %q", fe.Code, fe.Message, tc.wantStatus, tc.wantMsg)
			}
			if got := assistant.LogReason(err); got != tc.wantReason {
				t.Errorf("log reason = %q, want %q", got, tc.wantReason)
			}
			// A routine refusal is never a fault worth reporting.
			if _, _, report := classify(err); report {
				t.Error("a refusal is reported as a fault")
			}
		})
	}
}

// An unknown reason is not guessed into a status: it falls through as the use case's own
// error, which renders as the generic 500.
func TestAutoApplyEnqueueError_UnknownReasonFallsThrough(t *testing.T) {
	in := &autoapply.Refusal{Reason: "something-new"}
	if got := autoApplyEnqueueError(in); !errors.Is(got, in) {
		t.Errorf("an unknown reason = %v, want the error itself", got)
	}
}

// Everything that is not a typed refusal is handed back untouched: the 404 of an unknown
// slug, and a charge fault whose wrapped cause must still reach the exception log.
func TestAutoApplyEnqueueError_PassesOtherErrorsThrough(t *testing.T) {
	if got := autoApplyEnqueueError(pgx.ErrNoRows); !errors.Is(got, pgx.ErrNoRows) {
		t.Errorf("pgx.ErrNoRows = %v, want it unchanged", got)
	}

	// The charge fault, as the use case returns it.
	fault := fmt.Errorf("charge auto-apply for job %d: %w", 42, errors.New("failed to acquire connection from pool"))
	if got := autoApplyEnqueueError(fault); !errors.Is(got, fault) {
		t.Errorf("a fault = %v, want it unchanged", got)
	}
	if _, _, report := classify(fault); !report {
		t.Error("the charge fault is not reported")
	}
}

// The spent allowance is not the plain refusal's path: the use case's own type carries the
// Decision, and it must stay out of autoApplyEnqueueError so the 402 body can render it.
func TestAutoApplyEnqueueError_LeavesTheAllowanceRefusalAlone(t *testing.T) {
	in := &autoapply.AllowanceRefusal{Decision: plan.Decision{Feature: plan.FeatureAutoApply}}
	if got := autoApplyEnqueueError(in); !errors.Is(got, in) {
		t.Errorf("AllowanceRefusal = %v, want it unchanged", got)
	}
}

// The MCP auto_apply_start tool runs the use case's answer through the same mapping, so an
// agent is refused with the same status class (what the tool's metrics count a refusal by)
// and the same sentence the route answers. Composed here exactly as autoApplySlugTool does.
func TestAutoApplyStartTool_KeepsTheRefusalKindAndText(t *testing.T) {
	d := plan.Decision{Feature: plan.FeatureAutoApply, Used: 3, Limit: 3, Enforced: true}
	for _, tc := range []struct {
		name       string
		err        error
		wantCode   string
		wantText   string
		wantReason string
	}{
		{
			name: "unsupported ATS", err: &autoapply.Refusal{Reason: autoapply.RefusalUnsupportedSource},
			wantCode: "invalid_request", wantText: "auto-apply cannot queue an attempt against this posting's ATS",
			wantReason: "ats not supported for auto-apply",
		},
		{
			name: "no résumé", err: &autoapply.Refusal{Reason: autoapply.RefusalNoResume},
			wantCode: "conflict", wantText: "add a résumé before using auto-apply",
			wantReason: "no resume for auto-apply",
		},
		{
			name: "eligibility mismatch", err: &autoapply.Refusal{Reason: autoapply.RefusalEligibilityMismatch, Detail: "on-site in US"},
			wantCode: "conflict", wantText: "this job's requirements do not match your profile: on-site in US",
		},
		{
			name: "already applied", err: &autoapply.Refusal{Reason: autoapply.RefusalAlreadyApplied},
			wantCode: "conflict", wantText: "already applied to this job", wantReason: "already applied",
		},
		{
			name: "declined", err: &autoapply.Refusal{Reason: autoapply.RefusalAlreadyDeclined},
			wantCode: "conflict", wantText: "this auto-apply attempt was already declined",
		},
		{
			name: "allowance spent", err: &autoapply.AllowanceRefusal{Decision: d},
			wantCode: "payment_required", wantText: "You've used today's automatic applications.",
			wantReason: "auto-apply allowance spent",
		},
		{
			name: "no CV builder", err: &autoapply.Refusal{Reason: autoapply.RefusalNoCVBuilder},
			wantCode: "unavailable", wantText: "the assistant is not available",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A known slug with no attempt is the use case's answer, not jobNotFound.
			got := toolError(slugToolError("acme-1", autoApplyEnqueueError(tc.err)))
			if code := observability.ErrorCode(got); code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
			if got.Error() != tc.wantText {
				t.Errorf("text = %q, want %q", got.Error(), tc.wantText)
			}
			if reason := assistant.LogReason(got); reason != tc.wantReason {
				t.Errorf("log reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// A deployment without a plan meter must keep answering "auto-apply is not available" (503).
// The wiring assigns the concrete *plan.Store into an interface field: a nil pointer put there
// is a non-nil interface, which would skip the use case's own nil guard and panic in Consume.
// The same holds for the queue.
func TestAutoApplyEnqueuer_ANilDependencyStaysNil(t *testing.T) {
	h := &assistantHandlers{}
	e := h.autoApplyEnqueuer()
	if e.Plans != nil {
		t.Error("a nil *plan.Store became a non-nil Allowance: the use case's nil guard would be skipped")
	}
	if e.Queue != nil {
		t.Error("a nil *db.Queries became a non-nil EnqueueStore")
	}

	// And the use case refuses as unmetered rather than dereferencing the nil store. The job
	// lookup and CV gates are given so the start reaches the charge.
	e.FindJob = func(context.Context, string) (db.Job, error) { return db.Job{ID: 42, Source: "greenhouse"}, nil }
	e.BaseCV = func(context.Context, int64) (bool, error) { return true, nil }
	e.Seeder = cv.Seeder(hasResume{})
	e.Queue = unmeteredQueue{}
	err := e.Start(context.Background(), 7, "acme-1")
	var refused *autoapply.Refusal
	if !errors.As(err, &refused) || refused.Reason != autoapply.RefusalNoPlanMeter {
		t.Fatalf("a start without a meter = %v, want RefusalNoPlanMeter", err)
	}
	var fe *fiber.Error
	if !errors.As(autoApplyEnqueueError(err), &fe) || fe.Code != fiber.StatusServiceUnavailable {
		t.Errorf("without a meter the route answers %v, want 503", err)
	}
}

// hasResume is a Seeder with no résumé: the base CV is present, so it is never asked.
type hasResume struct{}

func (hasResume) Structured(context.Context, int64) (resumeextract.Structured, bool, error) {
	return resumeextract.Structured{}, false, nil
}

// unmeteredQueue is a queue the start never reaches once it refuses as unmetered.
type unmeteredQueue struct{}

func (unmeteredQueue) GetUserJobApplied(context.Context, db.GetUserJobAppliedParams) (bool, error) {
	return false, nil
}

func (unmeteredQueue) EnqueueAutoApply(context.Context, db.EnqueueAutoApplyParams) (int64, error) {
	panic("enqueued without a meter")
}

func (unmeteredQueue) GetAutoApplyQueueEntryForJob(context.Context, db.GetAutoApplyQueueEntryForJobParams) (db.GetAutoApplyQueueEntryForJobRow, error) {
	panic("queue read without a meter")
}

func (unmeteredQueue) GetApplicationStage(context.Context, db.GetApplicationStageParams) (pgtype.Text, error) {
	panic("queue read without a meter")
}
