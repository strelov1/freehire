package fitanalysis_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/credits"
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

// fakeMeter records what the service asked the ledger for, so a test can assert on the charge
// that was NOT made — which is most of what matters here.
type fakeMeter struct {
	remaining int
	cost      int
	balErr    error
	debits    []string
}

func (m *fakeMeter) Balance(context.Context, int64) (credits.Balance, error) {
	if m.balErr != nil {
		return credits.Balance{}, m.balErr
	}
	return credits.Balance{Remaining: m.remaining}, nil
}

func (m *fakeMeter) Cost(credits.Feature) int { return m.cost }

func (m *fakeMeter) Debit(_ context.Context, _ int64, f credits.Feature, ref string) (credits.Balance, error) {
	m.debits = append(m.debits, string(f)+":"+ref)
	return credits.Balance{Remaining: m.remaining - m.cost}, nil
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

// TestAuthorize is the credit rule, which used to be reachable only through a *fiber.Ctx.
func TestAuthorize(t *testing.T) {
	t.Run("a first analysis of a job is chargeable", func(t *testing.T) {
		svc := newService(&fakeStore{}, &fakeMeter{remaining: 10, cost: 1})
		chargeable, err := svc.Authorize(context.Background(), userID, jobID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !chargeable {
			t.Error("a never-analysed job must be chargeable")
		}
	})

	t.Run("a recompute is free and never gated", func(t *testing.T) {
		// No points at all: a recompute must still be allowed, which is what keeps an
		// analysis cached before credits shipped re-runnable.
		svc := newService(&fakeStore{row: cachedRow(t, 80)}, &fakeMeter{remaining: 0, cost: 1})
		chargeable, err := svc.Authorize(context.Background(), userID, jobID)
		if err != nil {
			t.Fatalf("a recompute must not be refused: %v", err)
		}
		if chargeable {
			t.Error("a recompute must not be chargeable")
		}
	})

	t.Run("a first analysis with too few points is refused, with the balance", func(t *testing.T) {
		svc := newService(&fakeStore{}, &fakeMeter{remaining: 0, cost: 1})
		_, err := svc.Authorize(context.Background(), userID, jobID)
		var refused *fitanalysis.InsufficientCreditsError
		if !errors.As(err, &refused) {
			t.Fatalf("err = %v, want *InsufficientCreditsError", err)
		}
		if refused.Balance.Remaining != 0 {
			t.Errorf("refusal carries remaining = %d, want 0 — the caller renders it", refused.Balance.Remaining)
		}
	})

	t.Run("exactly enough points is allowed", func(t *testing.T) {
		svc := newService(&fakeStore{}, &fakeMeter{remaining: 1, cost: 1})
		if _, err := svc.Authorize(context.Background(), userID, jobID); err != nil {
			t.Fatalf("a balance equal to the cost must be affordable: %v", err)
		}
	})

	t.Run("an unreadable balance never refuses", func(t *testing.T) {
		// Best-effort: the atomic Debit is the real ceiling, so a transient read failure
		// must not 402 a caller who can in fact afford the run.
		svc := newService(&fakeStore{}, &fakeMeter{balErr: errors.New("db down"), cost: 1})
		chargeable, err := svc.Authorize(context.Background(), userID, jobID)
		if err != nil {
			t.Fatalf("a balance read failure must not refuse: %v", err)
		}
		if !chargeable {
			t.Error("the run is still a first analysis, so still chargeable")
		}
	})

	t.Run("no meter wired means no gate", func(t *testing.T) {
		svc := newService(&fakeStore{}, nil)
		chargeable, err := svc.Authorize(context.Background(), userID, jobID)
		if err != nil || !chargeable {
			t.Fatalf("chargeable=%v err=%v, want true/nil with no ledger", chargeable, err)
		}
	})

	t.Run("a store failure is an error, not a free pass", func(t *testing.T) {
		svc := newService(&fakeStore{readErr: errors.New("db down")}, &fakeMeter{remaining: 10, cost: 1})
		if _, err := svc.Authorize(context.Background(), userID, jobID); err == nil {
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

func TestFollow(t *testing.T) {
	ctx := context.Background()

	// followerOf claims for the leader, then again for a follower, and releases the leader
	// with the given outcome — the shape the streaming caller is in when it loses the race.
	followerOf := func(svc *fitanalysis.Service, leaderSucceeded bool) *fitanalysis.Claim {
		leader := svc.Claim(userID, jobID)
		follower := svc.Claim(userID, jobID)
		leader.Release(leaderSucceeded)
		return follower
	}

	t.Run("a failed leader is reported, never papered over with the older row", func(t *testing.T) {
		// The row is present and readable: only run.succeeded says the leader failed, and
		// serving this row would dress a stale analysis up as the live result.
		store := &fakeStore{row: cachedRow(t, 80)}
		meter := &fakeMeter{remaining: 10, cost: 1}
		svc := newService(store, meter)

		_, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Chargeable: true,
			Claim: followerOf(svc, false),
		})
		if !errors.Is(err, fitanalysis.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if len(meter.debits) != 0 {
			t.Errorf("debits = %v, want none — nothing was served", meter.debits)
		}
	})

	t.Run("a successful leader's result is replayed and this caller still pays", func(t *testing.T) {
		// The leader here stands in for the autopilot's free pre-run: it never charges, so
		// the follower's own genuinely-new analysis must still be billed once.
		store := &fakeStore{row: cachedRow(t, 80)}
		meter := &fakeMeter{remaining: 10, cost: 1}
		svc := newService(store, meter)

		analysis, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Chargeable: true,
			Claim: followerOf(svc, true),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis == nil || analysis.OverallScore != 80 {
			t.Fatalf("analysis = %+v, want the leader's cached one", analysis)
		}
		if len(meter.debits) != 1 || meter.debits[0] != "match:7" {
			t.Errorf("debits = %v, want exactly [match:7]", meter.debits)
		}
	})

	t.Run("a recompute follower pays nothing", func(t *testing.T) {
		meter := &fakeMeter{remaining: 10, cost: 1}
		svc := newService(&fakeStore{row: cachedRow(t, 80)}, meter)

		if _, err := svc.Follow(ctx, fitanalysis.Request{
			UserID: userID, Job: db.Job{ID: jobID}, Chargeable: false,
			Claim: followerOf(svc, true),
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(meter.debits) != 0 {
			t.Errorf("debits = %v, want none — this caller owed nothing", meter.debits)
		}
	})

	t.Run("a leader that succeeded but left nothing readable is unavailable", func(t *testing.T) {
		svc := newService(&fakeStore{}, &fakeMeter{remaining: 10, cost: 1})
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
	meter := &fakeMeter{remaining: 10, cost: 1}
	svc := newService(store, meter)

	leader := svc.Claim(userID, jobID)
	follower := svc.Claim(userID, jobID)

	// Chargeable is deliberately true: Ensure must ignore it, never charge, and never be
	// made metered by a caller that filled the field in.
	svc.Ensure(context.Background(), fitanalysis.Request{
		UserID: userID, Job: db.Job{ID: jobID}, Chargeable: true, Claim: leader,
	})

	if len(store.upserted) != 0 {
		t.Errorf("upserts = %d, want 0 — an already-cached analysis is not recomputed", len(store.upserted))
	}
	if len(meter.debits) != 0 {
		t.Errorf("debits = %v, want none — the cold-start pre-run is unmetered by design", meter.debits)
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

	got := fitanalysis.ProjectTailoring(a, job)

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
