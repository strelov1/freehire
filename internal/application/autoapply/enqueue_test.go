package autoapply

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A charge that fails for a reason other than a refusal still blocks the action — an
// unmetered auto-apply submits a real application — and must not be rendered as an
// allowance refusal: that is an answer for the candidate, while this is a fault to report.
func TestCharge_FaultRefusesAndKeepsItsCause(t *testing.T) {
	cause := errors.New("failed to acquire connection from pool")
	e := &Enqueuer{Plans: failingAllowance{err: cause}}

	err := e.charge(context.Background(), 7, 42)
	if err == nil {
		t.Fatal("a failed charge let the auto-apply through")
	}
	var refused *AllowanceRefusal
	if errors.As(err, &refused) {
		t.Fatalf("a fault was rendered as an allowance refusal: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("the cause was dropped: %v", err)
	}
}

// A spent allowance comes back as *AllowanceRefusal carrying the meter's Decision, the one
// thing the 402 body renders.
func TestCharge_SpentAllowanceIsAnAllowanceRefusal(t *testing.T) {
	d := plan.Decision{Feature: plan.FeatureAutoApply, Used: 3, Limit: 3}
	e := &Enqueuer{Plans: refusingAllowance{decision: d}}

	err := e.charge(context.Background(), 7, 42)
	var refused *AllowanceRefusal
	if !errors.As(err, &refused) {
		t.Fatalf("a spent allowance = %v, want *AllowanceRefusal", err)
	}
	if refused.Decision != d {
		t.Errorf("Decision = %+v, want the meter's %+v", refused.Decision, d)
	}
}

// No meter at all is a configured-out deployment, not a fault: a refusal the transport
// words as "not available".
func TestCharge_NoMeterRefusesAsUnmetered(t *testing.T) {
	err := (&Enqueuer{}).charge(context.Background(), 7, 42)
	var refused *Refusal
	if !errors.As(err, &refused) || refused.Reason != RefusalNoPlanMeter {
		t.Fatalf("charge without a meter = %v, want RefusalNoPlanMeter", err)
	}
}

// Without a CV builder nothing can tailor an attempt, so the gate never reaches a read.
func TestStart_WithoutACVBuilderRefusesBeforeAnyRead(t *testing.T) {
	e := &Enqueuer{
		FindJob: func(context.Context, string) (db.Job, error) { return db.Job{ID: 1, Source: "greenhouse"}, nil },
		Queue:   panicStore{},
	}
	err := e.Start(context.Background(), 7, "acme-1")
	var refused *Refusal
	if !errors.As(err, &refused) || refused.Reason != RefusalNoCVBuilder {
		t.Fatalf("start without a CV builder = %v, want RefusalNoCVBuilder", err)
	}
}

// TestQueueable_CoversTheFacetProviders pins the relationship between the two provider
// lists (moved from internal/api/handler with the set). They are deliberately different sets,
// not two copies of one:
//
//   - queueableSources: an attempt may be QUEUED. Every entry here is tailored and reviewed,
//     whatever it can do afterwards.
//   - jobview.AutoApplyProviders: the "auto-apply available" facet, where a fill may be
//     SUBMITTED. atsapply's own test holds it equal to fillProviders ∪ browserUseProviders.
//
// Every provider the facet advertises must be queueable, or the filter offers a posting whose
// button can never start an attempt. The reverse is wider by exactly recruitee: it is queued
// and reviewed, then parks because no submit path drives it.
//
// Without this test a provider added to the facet but not here, or removed from here while
// still advertised, passes every check in both packages.
func TestQueueable_CoversTheFacetProviders(t *testing.T) {
	var unqueueable []string
	for provider := range jobview.AutoApplyProviders {
		if !Queueable(provider) {
			unqueueable = append(unqueueable, provider)
		}
	}
	if len(unqueueable) > 0 {
		slices.Sort(unqueueable)
		t.Errorf("the auto-apply facet advertises providers that cannot be queued: %v", unqueueable)
	}

	var notAdvertised []string
	for provider := range queueableSources {
		if !jobview.AutoApplyProviders[provider] {
			notAdvertised = append(notAdvertised, provider)
		}
	}
	slices.Sort(notAdvertised)
	// An exact set, not a subset: a second queueable-but-unsubmittable provider is a product
	// decision to make on purpose (and to document), not one to drift into.
	if want := []string{"recruitee"}; !slices.Equal(notAdvertised, want) {
		t.Errorf("queueable but absent from the auto-apply facet = %v, want exactly %v", notAdvertised, want)
	}
}

// The overlay's precedence: a decline also sets blocked_at, so it must outrank the
// failure markers, and a parked (blocked) attempt reads as failed, never as still queued.
func TestEntryStatusOf_Precedence(t *testing.T) {
	at := pgtype.Timestamptz{Valid: true}
	declined := pgtype.Text{String: "declined", Valid: true}
	approved := pgtype.Text{String: "approved", Valid: true}
	for _, tc := range []struct {
		name            string
		decision        pgtype.Text
		failed, blocked pgtype.Timestamptz
		want            EntryStatus
	}{
		{name: "fresh", decision: pgtype.Text{}, want: EntryQueued},
		{name: "approved", decision: approved, want: EntryQueued},
		{name: "failed", decision: approved, failed: at, want: EntryFailed},
		{name: "parked", decision: approved, blocked: at, want: EntryFailed},
		{name: "declined", decision: declined, blocked: at, failed: at, want: EntryDeclined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EntryStatusOf(tc.decision, tc.failed, tc.blocked); got != tc.want {
				t.Errorf("EntryStatusOf = %q, want %q", got, tc.want)
			}
		})
	}
}

type failingAllowance struct{ err error }

func (f failingAllowance) Consume(context.Context, int64, plan.Feature, string) (plan.Decision, error) {
	return plan.Decision{}, f.err
}

type refusingAllowance struct{ decision plan.Decision }

func (r refusingAllowance) Consume(context.Context, int64, plan.Feature, string) (plan.Decision, error) {
	return r.decision, plan.ErrRefused
}

// panicStore fails a test that reaches the queue: Start must refuse before any queue read.
type panicStore struct{}

func (panicStore) GetUserJobApplied(context.Context, db.GetUserJobAppliedParams) (bool, error) {
	panic("queue read before the CV-builder gate")
}
func (panicStore) EnqueueAutoApply(context.Context, db.EnqueueAutoApplyParams) (int64, error) {
	panic("queue write")
}
func (panicStore) GetAutoApplyQueueEntryForJob(context.Context, db.GetAutoApplyQueueEntryForJobParams) (db.GetAutoApplyQueueEntryForJobRow, error) {
	panic("queue read")
}
func (panicStore) GetApplicationStage(context.Context, db.GetApplicationStageParams) (pgtype.Text, error) {
	panic("queue read")
}
