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

// Queueable is the auto_apply_available facet plus an explicit, exact extra. The facet is
// jobview.AutoApplyProviders, which atsapply's own test holds equal to its executor sets
// (fillProviders ∪ browserUseProviders) — so this is the one place the queueable set is
// named, and a provider that becomes submittable is queueable with no edit here.
//
// The exact set is pinned rather than only "the facet is covered": a second
// queueable-but-unsubmittable provider spends a daily tailoring turn on attempts that cannot
// end in a submission, which is a product decision to make on purpose, not one to drift into.
func TestQueueable_IsTheFacetPlusRecruitee(t *testing.T) {
	for provider := range jobview.AutoApplyProviders {
		if !Queueable(provider) {
			t.Errorf("the auto-apply facet advertises %q, which cannot be queued", provider)
		}
	}
	for source, want := range map[string]bool{
		"greenhouse": true, "lever": true, "ashby": true, "workable": true, "recruitee": true,
		"smartrecruiters": false, "workday": false, "": false,
	} {
		if got := Queueable(source); got != want {
			t.Errorf("Queueable(%q) = %v, want %v", source, got, want)
		}
	}

	var extra []string
	for provider := range queueableUnadvertised {
		if jobview.AutoApplyProviders[provider] {
			t.Errorf("%q is listed in queueableUnadvertised but the facet advertises it; drop the stale entry", provider)
		}
		extra = append(extra, provider)
	}
	slices.Sort(extra)
	if want := []string{"recruitee"}; !slices.Equal(extra, want) {
		t.Errorf("queueable but not advertised = %v, want exactly %v", extra, want)
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
