package fitanalysis_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	userID int64 = 42
	jobID  int64 = 7
)

// fakeStore is the analysis cache in memory. row==nil stands for "never analysed", which the
// real store reports as pgx.ErrNoRows.
type fakeStore struct {
	row      *db.GetUserJobAnalysisRow
	readErr  error
	upserted []db.UpsertUserJobAnalysisParams
	list     []db.ListUserJobAnalysesRow
}

func (f *fakeStore) GetUserJobAnalysis(context.Context, db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error) {
	if f.readErr != nil {
		return db.GetUserJobAnalysisRow{}, f.readErr
	}
	if f.row == nil {
		return db.GetUserJobAnalysisRow{}, pgx.ErrNoRows
	}
	return *f.row, nil
}

func (f *fakeStore) UpsertUserJobAnalysis(_ context.Context, arg db.UpsertUserJobAnalysisParams) error {
	f.upserted = append(f.upserted, arg)
	return nil
}

func (f *fakeStore) ListUserJobAnalyses(context.Context, int64) ([]db.ListUserJobAnalysesRow, error) {
	return f.list, nil
}

// fakeMeter is a ledger that actually holds a balance, because the rule under test is that the
// DEBIT is the gate: a fake that only recorded calls could not show a second reservation being
// refused by the first one having landed.
type fakeMeter struct {
	// remaining is how much of today's allowance is left. limit is what the day started
	// with, so the fake can report a standing the way the real meter does — used against a
	// limit — while the assertions below keep reading the quantity a reader cares about.
	remaining int
	limit     int
	balErr    error
	debitErr  error
	debits    []string
	releases  []string
	// charged is the ledger's idempotency by (feature, ref): a second consumption for a
	// reference already charged takes nothing more.
	charged map[string]bool
}

// newMeter builds a meter with `remaining` of today's allowance left.
func newMeter(remaining int) *fakeMeter {
	return &fakeMeter{remaining: remaining, limit: remaining, charged: map[string]bool{}}
}

func (m *fakeMeter) used() int { return m.limit - m.remaining }

func (m *fakeMeter) Standing(context.Context, int64, plan.Feature) (plan.Standing, error) {
	if m.balErr != nil {
		return plan.Standing{}, m.balErr
	}
	return plan.Standing{
		Tier: plan.TierFree, Feature: plan.FeatureFit,
		Used: m.used(), Limit: m.limit,
	}, nil
}

func (m *fakeMeter) Consume(_ context.Context, _ int64, f plan.Feature, ref string) (plan.Decision, error) {
	d := plan.Decision{Tier: plan.TierFree, Feature: f, Used: m.used(), Limit: m.limit}
	if m.debitErr != nil {
		return d, m.debitErr
	}
	if m.charged[ref] {
		d.Allowed = true
		return d, nil // idempotent: this reference is already paid for
	}
	if m.remaining <= 0 {
		return d, plan.ErrRefused
	}
	m.remaining--
	m.charged[ref] = true
	m.debits = append(m.debits, string(f)+":"+ref)
	d.Allowed, d.Charge, d.Used = true, 1, m.used()
	return d, nil
}

func (m *fakeMeter) Release(ctx context.Context, _ int64, f plan.Feature, ref string) error {
	// A real ledger opens a transaction, so a cancelled context means the release never
	// happens. Modelling that is the only way a test can show the cleanup outliving the
	// cancellation that triggered it.
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.charged[ref] {
		delete(m.charged, ref)
		m.remaining++
		m.releases = append(m.releases, string(f)+":"+ref)
	}
	return nil
}

func cachedRow(t *testing.T, score int) *db.GetUserJobAnalysisRow {
	t.Helper()
	blob, err := json.Marshal(&matchanalysis.Analysis{OverallScore: score, Verdict: "Strong Fit"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &db.GetUserJobAnalysisRow{Analysis: blob, Model: "model-a", Language: "en"}
}

func newService(store fitanalysis.Store, meter fitanalysis.Meter) *fitanalysis.Service {
	// A nil client makes the analyzer a no-op; none of these cases reaches a compute.
	return fitanalysis.New(store, meter, matchanalysis.NewAnalyzer(nil))
}

// TestReserve is the credit rule, and it is now the GATE: the atomic debit decides, so a check
// that two concurrent runs could both pass no longer exists.
func TestReserve(t *testing.T) {
	ctx := context.Background()

	t.Run("a first analysis of a job takes the credit up front", func(t *testing.T) {
		meter := newMeter(10)
		svc := newService(&fakeStore{}, meter)

		reserved, err := svc.Reserve(ctx, userID, jobID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reserved {
			t.Error("a never-analysed job must reserve")
		}
		if len(meter.debits) != 1 || meter.debits[0] != "match:7" {
			t.Errorf("debits = %v, want the charge taken BEFORE the chain", meter.debits)
		}
		if meter.remaining != 9 {
			t.Errorf("remaining = %d, want 9 — the credit is held, not merely checked", meter.remaining)
		}
	})

	t.Run("a recompute is free and takes nothing", func(t *testing.T) {
		meter := newMeter(0) // no points at all
		svc := newService(&fakeStore{row: cachedRow(t, 80)}, meter)

		reserved, err := svc.Reserve(ctx, userID, jobID)
		if err != nil {
			t.Fatalf("a recompute must not be refused: %v", err)
		}
		if reserved || len(meter.debits) != 0 {
			t.Errorf("reserved=%v debits=%v, want a free recompute", reserved, meter.debits)
		}
	})

	t.Run("too few points is refused, with the balance", func(t *testing.T) {
		meter := newMeter(0)
		svc := newService(&fakeStore{}, meter)

		_, err := svc.Reserve(ctx, userID, jobID)
		var refused *fitanalysis.RefusedError
		if !errors.As(err, &refused) {
			t.Fatalf("err = %v, want *RefusedError", err)
		}
		if refused.Decision.Used != 0 {
			t.Errorf("refusal carries remaining = %d, want 0", refused.Decision.Used)
		}
	})

	// THE RACE. Two never-analysed jobs, one credit. Before the ledger became the gate both
	// passed a balance check, both ran the chain, and the loser's debit failed silently after
	// its analysis had been computed, cached and served.
	t.Run("one credit cannot fund two different jobs", func(t *testing.T) {
		meter := newMeter(1)
		svc := newService(&fakeStore{}, meter)

		if _, err := svc.Reserve(ctx, userID, 100); err != nil {
			t.Fatalf("the first run must be affordable: %v", err)
		}
		_, err := svc.Reserve(ctx, userID, 200)
		var refused *fitanalysis.RefusedError
		if !errors.As(err, &refused) {
			t.Fatalf("second job err = %v, want *RefusedError — one credit funds one job", err)
		}
		if len(meter.debits) != 1 {
			t.Errorf("debits = %v, want exactly one — the second run never got its credit", meter.debits)
		}
	})

	t.Run("two callers for the SAME job collapse into one charge", func(t *testing.T) {
		// Two tabs on one never-analysed job is neither a double charge nor a discount.
		meter := newMeter(10)
		svc := newService(&fakeStore{}, meter)

		for i := range 2 {
			if _, err := svc.Reserve(ctx, userID, jobID); err != nil {
				t.Fatalf("caller %d: %v", i, err)
			}
		}
		if meter.remaining != 9 || len(meter.debits) != 1 {
			t.Errorf("remaining=%d debits=%v, want one charge for one job", meter.remaining, meter.debits)
		}
	})

	t.Run("metering fails open", func(t *testing.T) {
		// Bookkeeping must never be able to refuse a legitimate analysis. An uncharged run is
		// a smaller wrong than a candidate blocked by our accounting.
		meter := newMeter(10)
		meter.debitErr = errors.New("ledger unreachable")
		svc := newService(&fakeStore{}, meter)

		reserved, err := svc.Reserve(ctx, userID, jobID)
		if err != nil {
			t.Fatalf("an unreachable ledger must not refuse: %v", err)
		}
		if reserved {
			t.Error("nothing was reserved, so nothing may be released later")
		}
	})

	t.Run("no meter wired reserves nothing and refuses nothing", func(t *testing.T) {
		svc := newService(&fakeStore{}, nil)
		reserved, err := svc.Reserve(ctx, userID, jobID)
		if err != nil || reserved {
			t.Fatalf("reserved=%v err=%v, want false/nil with no ledger", reserved, err)
		}
	})

	t.Run("a store failure is an error, not a free pass", func(t *testing.T) {
		svc := newService(&fakeStore{readErr: errors.New("db down")}, newMeter(10))
		if _, err := svc.Reserve(ctx, userID, jobID); err == nil {
			t.Error("a cache read failure must surface, not be read as 'never analysed'")
		}
	})
}

func TestCached(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	t.Run("never analysed reads as no analysis, no error", func(t *testing.T) {
		svc := newService(&fakeStore{}, nil)
		analysis, _, err := svc.Cached(context.Background(), userID, jobID)
		if err != nil || analysis != nil {
			t.Fatalf("analysis=%v err=%v, want nil/nil", analysis, err)
		}
	})

	t.Run("a corrupt blob reads as no analysis", func(t *testing.T) {
		// The caller re-offers a compute rather than surfacing a decode error.
		svc := newService(&fakeStore{row: &db.GetUserJobAnalysisRow{Analysis: []byte("{not json")}}, nil)
		analysis, _, err := svc.Cached(context.Background(), userID, jobID)
		if err != nil || analysis != nil {
			t.Fatalf("analysis=%v err=%v, want nil/nil", analysis, err)
		}
	})

	t.Run("a cached analysis comes back with the stamps it was computed under", func(t *testing.T) {
		row := cachedRow(t, 80)
		row.CvUploadedAt = pgtype.Timestamptz{Time: now, Valid: true}
		svc := newService(&fakeStore{row: row}, nil)

		analysis, stored, err := svc.Cached(context.Background(), userID, jobID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis == nil || analysis.OverallScore != 80 {
			t.Fatalf("analysis = %+v, want the cached one scoring 80", analysis)
		}
		live := fitanalysis.Stamps{CVUploadedAt: &now, Model: "model-a", Language: "en"}
		if !live.Fresh(stored) {
			t.Error("stamps read back off the row must judge fresh against the values they were written with")
		}
		if live.Fresh(fitanalysis.Stamps{Model: "model-b"}) {
			t.Error("a different model must not judge fresh")
		}
	})
}

// reserve stands in for the caller having paid before it reached the code under test.
func reserve(t *testing.T, m *fakeMeter, jobID int64) {
	t.Helper()
	if _, err := m.Consume(context.Background(), userID, plan.FeatureFit, strconv.FormatInt(jobID, 10)); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}
}

// followerOf claims for the leader, then again for a follower, and releases the leader with the
// given outcome — the shape a streaming caller is in when it loses the race.
func followerOf(svc *fitanalysis.Service, leaderSucceeded bool) *fitanalysis.Claim {
	leader := svc.Claim(userID, jobID)
	follower := svc.Claim(userID, jobID)
	leader.Release(leaderSucceeded)
	return follower
}

func TestFollow(t *testing.T) {
	ctx := context.Background()

	t.Run("a failed leader is reported, never papered over with the older row", func(t *testing.T) {
		// The row is present and readable: only run.succeeded says the leader failed, and
		// serving this row would dress a stale analysis up as the live result. Reserved is
		// false because a cached analysis is exactly what makes a run free.
		store := &fakeStore{row: cachedRow(t, 80)}
		meter := newMeter(10)
		svc := newService(store, meter)

		_, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Reserved: false,
			Claim: followerOf(svc, false),
		})
		if !errors.Is(err, fitanalysis.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if len(meter.releases) != 0 {
			t.Errorf("releases = %v, want none — this caller reserved nothing", meter.releases)
		}
	})

	t.Run("a successful leader's result is replayed and this caller still pays", func(t *testing.T) {
		// The leader here stands in for the autopilot's free pre-run: it never charges, so
		// the follower's own genuinely-new analysis must still be billed once.
		store := &fakeStore{row: cachedRow(t, 80)}
		meter := newMeter(10)
		svc := newService(store, meter)
		reserve(t, meter, jobID)

		analysis, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
			Claim: followerOf(svc, true),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis == nil || analysis.OverallScore != 80 {
			t.Fatalf("analysis = %+v, want the leader's cached one", analysis)
		}
		// The caller reserved before waiting and the result is real, so the charge stands.
		if len(meter.releases) != 0 {
			t.Errorf("releases = %v, want none — a served analysis is paid for", meter.releases)
		}
	})

	t.Run("a recompute follower pays nothing", func(t *testing.T) {
		meter := newMeter(10)
		svc := newService(&fakeStore{row: cachedRow(t, 80)}, meter)

		if _, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Reserved: false,
			Claim: followerOf(svc, true),
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(meter.releases) != 0 {
			t.Errorf("releases = %v, want none — this caller reserved nothing", meter.releases)
		}
	})

	t.Run("a leader that succeeded but left nothing readable is unavailable", func(t *testing.T) {
		svc := newService(&fakeStore{}, newMeter(10))
		if _, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID},
			Claim: followerOf(svc, true),
		}); !errors.Is(err, fitanalysis.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}

// TestEnsureShortCircuitsOnACachedAnalysis pins the cold-start pre-run's cheapest path: an
// analysis already exists, so nothing is computed and the follower waiting on this leader is
// told the cache is good.
func TestEnsureShortCircuitsOnACachedAnalysis(t *testing.T) {
	store := &fakeStore{row: cachedRow(t, 80)}
	meter := newMeter(10)
	svc := newService(store, meter)

	leader := svc.Claim(userID, jobID)
	follower := svc.Claim(userID, jobID)

	// Reserved is deliberately true: Ensure must ignore it, never charge and never release,
	// and never be made metered by a caller that filled the field in.
	svc.Ensure(context.Background(), fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true, Claim: leader,
	})

	if len(store.upserted) != 0 {
		t.Errorf("upserts = %d, want 0 — an already-cached analysis is not recomputed", len(store.upserted))
	}
	if len(meter.debits) != 0 || len(meter.releases) != 0 {
		t.Errorf("debits=%v releases=%v, want none — the cold-start pre-run is unmetered by design",
			meter.debits, meter.releases)
	}

	// The follower must be released, and told the cache is trustworthy.
	analysis, err := svc.Follow(context.Background(), fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Claim: follower,
	})
	if err != nil || analysis == nil {
		t.Fatalf("follower got analysis=%v err=%v, want the cached analysis", analysis, err)
	}
}

// TestEnsureReleasesItsClaimWhenTheCacheReadFails is the deadlock guard: a leader that gives up
// must still wake its followers, or that (candidate, job) pair is stranded for the process's life.
func TestEnsureReleasesItsClaimWhenTheCacheReadFails(t *testing.T) {
	svc := newService(&fakeStore{readErr: errors.New("db down")}, nil)
	leader := svc.Claim(userID, jobID)
	follower := svc.Claim(userID, jobID)

	svc.Ensure(context.Background(), fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Claim: leader,
	})

	released := make(chan error, 1)
	go func() {
		_, err := svc.Follow(context.Background(), fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Claim: follower,
		})
		released <- err
	}()
	select {
	case err := <-released:
		if !errors.Is(err, fitanalysis.ErrUnavailable) {
			t.Errorf("follower err = %v, want ErrUnavailable — the leader gave up", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the follower was never released by a leader that failed its cache read")
	}
}

// TestRequiredAndOptional pin the two readers of one cached analysis: the tailoring surfaces
// that cannot proceed without it, and the CV-vs-job score that can.
func TestRequiredAndOptional(t *testing.T) {
	ctx := context.Background()

	t.Run("no analysis is ErrNoAnalysis for Required, plain absence for Optional", func(t *testing.T) {
		svc := newService(&fakeStore{}, nil)
		if _, err := svc.Required(ctx, userID, jobID); !errors.Is(err, fitanalysis.ErrNoAnalysis) {
			t.Errorf("Required err = %v, want ErrNoAnalysis", err)
		}
		if a, ok := svc.Optional(ctx, userID, jobID); ok || a != nil {
			t.Errorf("Optional = %v/%v, want nil/false", a, ok)
		}
	})

	t.Run("an unreadable blob reads the same as none", func(t *testing.T) {
		// Both mean "there is nothing to ground on" — a decode failure must not become a
		// 500 on a surface whose honest answer is "run the analysis first".
		store := &fakeStore{row: &db.GetUserJobAnalysisRow{Analysis: []byte("{not json")}}
		svc := newService(store, nil)
		if _, err := svc.Required(ctx, userID, jobID); !errors.Is(err, fitanalysis.ErrNoAnalysis) {
			t.Errorf("Required err = %v, want ErrNoAnalysis", err)
		}
	})

	t.Run("a store failure surfaces on Required and degrades on Optional", func(t *testing.T) {
		svc := newService(&fakeStore{readErr: errors.New("db down")}, nil)
		if _, err := svc.Required(ctx, userID, jobID); err == nil || errors.Is(err, fitanalysis.ErrNoAnalysis) {
			t.Errorf("Required err = %v, want the read failure itself, not ErrNoAnalysis", err)
		}
		if _, ok := svc.Optional(ctx, userID, jobID); ok {
			t.Error("Optional must report no analysis when the read failed")
		}
	})

	t.Run("a service wired to nothing degrades rather than panicking", func(t *testing.T) {
		// Tools run inside an SSE writer's goroutine, where a panic reaches no recover.
		var svc *fitanalysis.Service
		if _, err := svc.Required(ctx, userID, jobID); !errors.Is(err, fitanalysis.ErrNoAnalysis) {
			t.Errorf("Required on a nil service = %v, want ErrNoAnalysis", err)
		}
		if _, ok := svc.Optional(ctx, userID, jobID); ok {
			t.Error("Optional on a nil service must report no analysis")
		}
	})

	t.Run("a cached analysis comes back to both", func(t *testing.T) {
		svc := newService(&fakeStore{row: cachedRow(t, 80)}, nil)
		got, err := svc.Required(ctx, userID, jobID)
		if err != nil || got == nil || got.OverallScore != 80 {
			t.Fatalf("Required = %+v, %v; want the cached analysis", got, err)
		}
		if a, ok := svc.Optional(ctx, userID, jobID); !ok || a.OverallScore != 80 {
			t.Errorf("Optional = %+v/%v, want the same analysis/true", a, ok)
		}
	})
}

// TestProjectTailoring pins the requirement split the honest wall turns on: the agent may
// reframe a missing_have from evidence the candidate already owns, and must ASK about a
// missing_gap. Putting one in the other's list is what would let it write an unbacked claim.
func TestProjectTailoring(t *testing.T) {
	a := &matchanalysis.Analysis{
		Verdict:      "Good Fit",
		OverallScore: 71,
		RequirementMatch: []matchanalysis.Requirement{
			{Text: "Kafka in production", Status: matchanalysis.StatusMissingHave},
			{Text: "Kubernetes operator authoring", Status: matchanalysis.StatusMissingGap},
			{Text: "Go", Status: matchanalysis.StatusCovered},
			{Text: "Terraform", Status: matchanalysis.StatusMissingHave},
		},
	}
	job := db.Job{Title: "Senior Backend", Company: "Acme", PublicSlug: "senior-backend-acme",
		Description: "<p>We need <strong>Kafka</strong> in production.</p>"}

	got := fitanalysis.ProjectTailoring(a, job, nil)

	if len(got.MissingHave) != 2 || got.MissingHave[0].Text != "Kafka in production" {
		t.Errorf("MissingHave = %+v, want the two reframe-able requirements", got.MissingHave)
	}
	if len(got.MissingGap) != 1 || got.MissingGap[0].Text != "Kubernetes operator authoring" {
		t.Errorf("MissingGap = %+v, want only the genuine gap", got.MissingGap)
	}
	// A covered requirement belongs in neither list: there is nothing to reframe and nothing
	// to ask about.
	for _, r := range append(got.MissingHave, got.MissingGap...) {
		if r.Status == matchanalysis.StatusCovered {
			t.Errorf("a covered requirement (%q) leaked into the split", r.Text)
		}
	}
	if strings.Contains(got.Job.Description, "<p>") || strings.Contains(got.Job.Description, "<strong>") {
		t.Errorf("the posting reaches the model as markup: %q", got.Job.Description)
	}
	if got.Job.Slug != "senior-backend-acme" || got.Verdict != "Good Fit" || got.OverallScore != 71 {
		t.Errorf("projection = %+v, want the vacancy and verdict carried through", got)
	}
}

// TestProjectTailoringCarriesThePostingsOwnRequirements pins the second requirement source.
// The split above is what an ANALYSIS decided is missing; this is what the POSTING itself
// asks for, read out of its own markup. The agent needs both: the split tells it what to act
// on, the list tells it what the employer actually wrote — including the requirements the
// analysis found already covered, which are exactly the ones worth keeping prominent in a CV.
func TestProjectTailoringCarriesThePostingsOwnRequirements(t *testing.T) {
	a := &matchanalysis.Analysis{Verdict: "Good Fit"}
	job := db.Job{Title: "Senior Backend", Description: "<p>irrelevant</p>"}
	reqs := []enrich.Requirement{
		{Text: "5 years of Go", Priority: "required"},
		{Text: "Kafka", Priority: "preferred"},
	}

	got := fitanalysis.ProjectTailoring(a, job, reqs)

	if len(got.Job.Requirements) != 2 {
		t.Fatalf("Job.Requirements = %+v, want the posting's own two", got.Job.Requirements)
	}
	if got.Job.Requirements[0].Text != "5 years of Go" || got.Job.Requirements[0].Priority != "required" {
		t.Errorf("Job.Requirements[0] = %+v, want the text and priority carried through", got.Job.Requirements[0])
	}
}

// A posting that states no requirements — or one whose caller could not read them — must
// reach the model with the key ABSENT, not as an empty list dressed up as an answer: the agent
// reads `"requirements": []` as "this employer asked for nothing", which no posting means.
// Asserted through the JSON, because `omitempty` is the whole mechanism and a struct
// comparison would pass with the tag deleted.
func TestProjectTailoringOmitsRequirementsItDoesNotHave(t *testing.T) {
	got := fitanalysis.ProjectTailoring(&matchanalysis.Analysis{}, db.Job{}, nil)

	encoded, err := json.Marshal(got.Job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "requirements") {
		t.Errorf("job encoded as %s, want no requirements key when the posting states none", encoded)
	}

	withOne := fitanalysis.ProjectTailoring(&matchanalysis.Analysis{}, db.Job{},
		[]enrich.Requirement{{Text: "Go", Priority: "required"}})
	encoded, err = json.Marshal(withOne.Job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"requirements"`) {
		t.Errorf("job encoded as %s, want the requirements key when the posting states some", encoded)
	}
}

// TestRunReleasesTheReservationWhenNothingIsProduced is the other half of making the debit the
// gate: taking the credit up front is only honest if a run that produces nothing gives it back.
// Otherwise closing the race would have cost candidates money on every failed analysis.
func TestRunReleasesTheReservationWhenNothingIsProduced(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	meter := newMeter(10)
	svc := newService(store, meter)

	reserved, err := svc.Reserve(ctx, userID, jobID)
	if err != nil || !reserved {
		t.Fatalf("Reserve = %v, %v; want a reservation", reserved, err)
	}
	if meter.remaining != 9 {
		t.Fatalf("remaining = %d, want the credit held", meter.remaining)
	}

	// A nil client makes Analyze answer (nil, nil): the LLM is unconfigured, so nothing is
	// computed and nothing cached.
	analysis, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: reserved,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil)
	if err != nil || analysis != nil {
		t.Fatalf("Run = %v, %v; want nil/nil for an unconfigured LLM", analysis, err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — a failed analysis costs nothing", meter.remaining)
	}
	if len(meter.releases) != 1 {
		t.Errorf("releases = %v, want the reservation given back", meter.releases)
	}
	if len(store.upserted) != 0 {
		t.Errorf("upserts = %d, want 0 — nothing was produced to cache", len(store.upserted))
	}

	// And the ref is chargeable again, so the candidate's retry is charged once, not never.
	again, err := svc.Reserve(ctx, userID, jobID)
	if err != nil || !again {
		t.Fatalf("retry Reserve = %v, %v; a released ref must be chargeable again", again, err)
	}
}

// TestRunKeepsTheReservationWhenItProducesAnAnalysis is the companion: a run that delivers is
// paid for, and the release path must not fire on success.
func TestRunKeepsTheReservationWhenItProducesAnAnalysis(t *testing.T) {
	// Run's success branch needs a real analyzer, which needs a model; the streamed and
	// on-demand success paths are covered end to end by the tagged handler tests. What is
	// checked here is the accounting either side of it: Reserve holds, and nothing releases
	// unless the run says it produced nothing.
	ctx := context.Background()
	meter := newMeter(10)
	svc := newService(&fakeStore{}, meter)

	if _, err := svc.Reserve(ctx, userID, jobID); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if meter.remaining != 9 || len(meter.releases) != 0 {
		t.Fatalf("remaining=%d releases=%v after reserving", meter.remaining, meter.releases)
	}
}

// TestReleaseNeverVoidsACreditAnEarlierRunEarned pins the other half of the rule: a LATER run
// that fails — the autopilot's refresh, a recompute — must not give back the credit an earlier
// run already earned. A credit buys HAVING the analysis, not the attempt that produced it.
func TestReleaseNeverVoidsACreditAnEarlierRunEarned(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	// An analysis exists for the pair, and it was paid for.
	svc := newService(&fakeStore{row: cachedRow(t, 80)}, meter)
	reserve(t, meter, jobID)

	// A recompute runs later and produces nothing. It reaches the release (it ran the chain),
	// and the guard is what stops it giving back the earlier run's credit.
	if _, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID},
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(meter.releases) != 0 {
		t.Errorf("releases = %v, want none — the analysis it was paid for still exists", meter.releases)
	}
	if meter.remaining != 9 {
		t.Errorf("remaining = %d, want 9", meter.remaining)
	}
}

// TestAFreeLeaderReleasesThePayingFollowersCredit is why the release belongs to whoever ran the
// chain rather than to whoever paid. The autopilot's cold-start pre-run leads for free; a
// streaming caller reserved and is following it. When the pre-run produces nothing, the
// follower is told the analysis is unavailable — and the charge standing against that pair is
// theirs, so the leader gives it back on their behalf.
func TestAFreeLeaderReleasesThePayingFollowersCredit(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	svc := newService(&fakeStore{}, meter)

	// The streaming caller reserved before it started following.
	reserve(t, meter, jobID)
	if meter.remaining != 9 {
		t.Fatalf("remaining = %d, want the credit held", meter.remaining)
	}

	leader := svc.Claim(userID, jobID)
	follower := svc.Claim(userID, jobID)

	// The pre-run: nothing cached, a nil client, so it computes nothing. Reserved is false —
	// this half never charges.
	svc.Ensure(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Claim: leader,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	})

	if _, err := svc.Follow(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true, Claim: follower,
	}); !errors.Is(err, fitanalysis.ErrUnavailable) {
		t.Fatalf("follower err = %v, want ErrUnavailable", err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — nobody got an analysis, so nobody pays", meter.remaining)
	}
	if len(meter.releases) != 1 {
		t.Errorf("releases = %v, want exactly one, from the leader", meter.releases)
	}
}

// TestAFollowerNeverReleasesOnItsOwn is the other side: the debit is per (candidate, job) and
// shared, so a follower giving it back could void a charge the leader's result earned. The
// route that matters is a leader whose CACHE WRITE fails — it still returned its analysis to
// whoever paid for it, and leaves no row for the follower to read.
func TestAFollowerNeverReleasesOnItsOwn(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	// No row: the leader "succeeded" but its cache write did not land.
	svc := newService(&fakeStore{}, meter)
	reserve(t, meter, jobID)

	if _, err := svc.Follow(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
		Claim: followerOf(svc, true), // leader reports success
	}); !errors.Is(err, fitanalysis.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}

	if len(meter.releases) != 0 {
		t.Errorf("releases = %v, want none — the leader was served and earned the charge", meter.releases)
	}
	if meter.remaining != 9 {
		t.Errorf("remaining = %d, want 9 — the served analysis stays paid for", meter.remaining)
	}
}

// TestACancelledRunStillGivesTheCreditBack is the disconnect case. The on-demand endpoint runs
// on the request's own context, so a client that walks away mid-analysis cancels it — the chain
// fails with the cancellation, and a release on that same context could not even open its
// transaction. The candidate would stay charged for an analysis they never received, in exactly
// the situation the release exists for.
func TestACancelledRunStillGivesTheCreditBack(t *testing.T) {
	meter := newMeter(10)
	svc := newService(&fakeStore{}, meter)

	if _, err := svc.Reserve(context.Background(), userID, jobID); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if meter.remaining != 9 {
		t.Fatalf("remaining = %d, want the credit held", meter.remaining)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the reader is gone before the run even starts

	if _, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — a run nobody waited for still costs nothing", meter.remaining)
	}
	if len(meter.releases) != 1 {
		t.Errorf("releases = %v, want the reservation given back despite the cancellation", meter.releases)
	}
}

// TestAFailedEarnedCheckStillGivesTheCreditBack pins the direction the cleanup errs in. The
// check that stops a release — "does an analysis already exist for this pair?" — is a database
// read, and it can fail or time out. An unanswered question is not a yes: leaving a candidate
// charged for a run that produced nothing is the worse of the two errors.
func TestAFailedEarnedCheckStillGivesTheCreditBack(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	store := &fakeStore{}
	svc := newService(store, meter)

	if _, err := svc.Reserve(ctx, userID, jobID); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	// From here the cache is unreadable, so the release cannot learn whether anything exists.
	store.readErr = errors.New("db down")

	if _, err := svc.Run(ctx, fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Reserved: true,
		Analyzer: matchanalysis.NewAnalyzer(nil),
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if meter.remaining != 10 {
		t.Errorf("remaining = %d, want 10 — an unanswerable check must not keep the charge", meter.remaining)
	}
}
