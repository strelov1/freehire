package fitanalysis_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// cannedModel answers the chain's three stages in order, which is what makes Run's SUCCESS
// branch reachable without a model — the branch where the analysis exists and the only
// thing that can still go wrong is the cache write.
type cannedModel struct{ resp []string }

func (m *cannedModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	r := m.resp[0]
	m.resp = m.resp[1:]
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r}}}, nil
}

func (*cannedModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

const (
	fitStage1JSON = `{"requirements":[{"text":"Go","priority":"required","status":"covered","evidence":"5y at Acme"}]}`
	fitStage2JSON = `{"title_alignment":{"score":80},"experience_relevance":{"score":70},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"location_fit":{"score":60},"strengths":["Strong Go"],"gaps":[],"recommendation":"Apply."}`
	fitStage3JSON = `{"title_alignment":{"score":80},"experience_relevance":{"score":70},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"location_fit":{"score":60},"strengths":["Strong Go"],"gaps":[],"recommendation":"Apply."}`
)

// producingRequest is a request whose chain really returns an analysis.
func producingRequest(claim *fitanalysis.Claim) fitanalysis.Request {
	analyzer := matchanalysis.NewAnalyzer(llm.NewWithModel(
		&cannedModel{resp: []string{fitStage1JSON, fitStage2JSON, fitStage3JSON}}))
	return fitanalysis.Request{
		UserID:   userID,
		Job:      db.Job{ID: jobID},
		Analyzer: analyzer,
		Claim:    claim,
		Input: matchanalysis.Input{
			JobTitle:       "Senior Go Engineer",
			JobDescription: "Build backends in Go.",
			StructuredResume: resumeextract.Professional{
				Summary:    "Backend engineer, 5y Go at Acme.",
				Experience: []resumeextract.Experience{{Company: "Acme", Title: "Backend Engineer"}},
				Skills:     []string{"Go"},
			},
			Match:    jobmatch.JobMatch{Matched: []string{"go"}, CoveragePercent: 100},
			Language: "en",
		},
	}
}

// A follower reads only the cache, so what the leader publishes has to be whether the CACHE
// WRITE landed — not merely whether the chain produced something. One flag answered both,
// so a leader whose write failed told its followers to trust a cache holding an older row
// or nothing at all, which Follow's whole discipline exists to prevent.
func TestRunPublishesTheCacheWriteFailureToFollowers(t *testing.T) {
	ctx := context.Background()
	// A STALE row is what makes the bug visible rather than merely latent: with an empty
	// cache a wrongly-trusting follower finds nothing and reports unavailable anyway. The
	// live shape is a pair analysed before, whose older analysis is exactly what a failed
	// write leaves behind for the follower to serve as this run's result.
	store := &fakeStore{row: cachedRow(t, 40), upsertErr: errors.New("statement timeout")}
	svc := newService(store, nil)

	leader := svc.Claim(userID, jobID)
	if !leader.IsLeader() {
		t.Fatal("the first claimant must lead")
	}
	follower := svc.Claim(userID, jobID)
	if follower.IsLeader() {
		t.Fatal("the second claimant must follow")
	}

	analysis, err := svc.Run(ctx, producingRequest(leader), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The caller who ran the chain still gets what it computed — the write is best-effort
	// towards them, and they are already holding the result.
	if analysis == nil {
		t.Fatal("Run returned no analysis, so the case under test was never reached")
	}

	followed, err := svc.Follow(ctx, fitanalysis.Request{UserID: userID, Job: db.Job{ID: jobID}, Claim: follower})
	if !errors.Is(err, fitanalysis.ErrUnavailable) {
		t.Errorf("Follow = %v, want ErrUnavailable: the cache holds nothing this run wrote", err)
	}
	if followed != nil {
		t.Errorf("Follow served a score of %d, which is the STALE row dressed up as this run's result",
			followed.OverallScore)
	}
}

// The counterpart, and the reason the flag had to be SPLIT rather than moved: the caller
// that received the analysis got what it paid for. Refunding on a failed cache write would
// void a charge the returned result earned — Follow's own comment names that case.
func TestRunKeepsTheChargeWhenOnlyTheCacheWriteFailed(t *testing.T) {
	ctx := context.Background()
	meter := newMeter(10)
	store := &fakeStore{upsertErr: errors.New("statement timeout")}
	svc := newService(store, meter)

	reserved, err := svc.Reserve(ctx, userID, jobID)
	if err != nil || !reserved {
		t.Fatalf("Reserve = %v, %v; want a reservation", reserved, err)
	}

	req := producingRequest(nil)
	req.Reserved = reserved
	if _, err := svc.Run(ctx, req, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(meter.releases) != 0 {
		t.Errorf("releases = %v, want none — the candidate received the analysis they paid for", meter.releases)
	}
	if meter.remaining != 9 {
		t.Errorf("remaining = %d, want 9 — the charge stands", meter.remaining)
	}
}

// Cache is best-effort towards its caller and always was; what changed is that it reports
// the failure at all, because the follower it strands cannot read the log.
func TestCacheReportsAFailedWrite(t *testing.T) {
	svc := newService(&fakeStore{upsertErr: errors.New("statement timeout")}, nil)

	err := svc.Cache(context.Background(), userID, db.Job{ID: jobID}, nil, "en",
		&matchanalysis.Analysis{OverallScore: 80, Verdict: "Strong Fit"})

	if err == nil {
		t.Error("Cache reported success on a write that failed")
	}
}
