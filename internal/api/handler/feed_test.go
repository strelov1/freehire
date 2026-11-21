package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/jevrank"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/feed"
	"github.com/strelov1/freehire/internal/search/search"
)

type feedProfile struct {
	p   userprofile.Profile
	err error
}

func (f feedProfile) Get(context.Context, int64) (userprofile.Profile, error) { return f.p, f.err }

type feedExcluded []int64

func (f feedExcluded) ExcludedJobIDs(context.Context, int64) ([]int64, error) { return f, nil }

type feedPlansFake struct{ err error }

func (f feedPlansFake) RequirePaid(context.Context, int64) (plan.Tier, error) {
	return plan.TierPro, f.err
}

func feedApp(s searcher, prof feedProfile) (*fiber.App, *auth.Issuer) {
	return feedAppPlans(s, prof, feedPlansFake{})
}

func feedAppPlans(s searcher, prof feedProfile, plans feedPlans) (*fiber.App, *auth.Issuer) {
	return feedAppRank(s, prof, plans, nil, nil)
}

func feedAppRank(s searcher, prof feedProfile, plans feedPlans, rank *feed.Reranker, cvs feedCVs) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &feedHandlers{search: s, profiles: prof, excluded: feedExcluded{10}, plans: plans, rank: rank, cvs: cvs}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/feed", auth.RequireAuth(iss, testVersions), h.Feed)
	return app, iss
}

func TestFeed_NoProfileSearchesNothing(t *testing.T) {
	fake := &fakeSearcher{}
	app, iss := feedApp(fake, feedProfile{err: userprofile.ErrNotFound})
	status, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := body["meta"].(map[string]any)["reason"]; got != feedReasonNoProfile {
		t.Errorf("reason = %v", got)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searched the index for a user with no profile: %#v", fake.got)
	}
}

func TestFeed_RanksByProfileAndExplainsEachCard(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 1, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{
		PublicSlug: "go-dev", Title: "Go Dev", Skills: []string{"go", "kubernetes"}, WorkMode: "remote",
	}}}}}
	app, iss := feedApp(fake, feedProfile{p: userprofile.Profile{Skills: []string{"go"}, Seniorities: []string{"senior"}}})
	status, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}
	if len(fake.got.Vector) == 0 || len(fake.got.Sort) != 0 {
		t.Errorf("want a match vector and no sort: %#v", fake.got)
	}
	groups, _ := fake.got.Filter.([][]string)
	if !filterHas(groups, `enrichment.seniority = "senior"`) || !filterHas(groups, "id NOT IN [10]") {
		t.Errorf("profile filter or exclusions missing: %#v", fake.got.Filter)
	}
	m := body["data"].([]any)[0].(map[string]any)["match"].(map[string]any)
	if why := m["why"].([]any); len(why) != 1 || why[0] != "go" {
		t.Errorf("why = %v", why)
	}
	if miss := m["may_not_fit"].([]any); len(miss) != 1 || miss[0] != "kubernetes" {
		t.Errorf("may_not_fit = %v", miss)
	}
	// Salary and seniority are not stated by the job: unknown, never confirmed.
	if unk := m["unknown"].([]any); len(unk) != 2 || unk[0] != "salary" || unk[1] != "seniority" {
		t.Errorf("unknown = %v", unk)
	}
}

func TestFeed_NoSkillsDegradesToDefaultOrder(t *testing.T) {
	fake := &fakeSearcher{}
	app, iss := feedApp(fake, feedProfile{p: userprofile.Profile{Seniorities: []string{"senior"}}})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if got := body["meta"].(map[string]any)["degraded"]; got != "no_match_vector" {
		t.Errorf("degraded = %v", got)
	}
	if fake.got.Vector != nil {
		t.Errorf("vector sent without skills")
	}
}

func TestFeed_PlanGate(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"free":        {plan.ErrPaymentRequired, fiber.StatusPaymentRequired},
		"plan unread": {errors.New("db down"), fiber.StatusServiceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeSearcher{}
			app, iss := feedAppPlans(fake, feedProfile{p: userprofile.Profile{Skills: []string{"go"}}}, feedPlansFake{err: tc.err})
			if status, _ := deckGet(t, app, iss, "/api/v1/me/feed"); status != tc.want {
				t.Fatalf("status = %d, want %d", status, tc.want)
			}
			if fake.got.Limit != 0 {
				t.Errorf("searched for a refused caller: %#v", fake.got)
			}
		})
	}
}

func TestFeed_SignedOutIs401(t *testing.T) {
	app, _ := feedApp(&fakeSearcher{}, feedProfile{})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/me/feed", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

type fakeScorer struct {
	scores map[string]float64
	calls  int
}

func (f *fakeScorer) Rank(_ context.Context, _ string, jobs []jevrank.Job) []jevrank.Result {
	f.calls++
	out := make([]jevrank.Result, len(jobs))
	for i, j := range jobs {
		out[i] = jevrank.Result{Slug: j.Slug, Score: f.scores[j.Slug], OK: true}
	}
	return out
}

type fakeCVs struct {
	ok  bool
	err error
}

func (f fakeCVs) BaseCV(context.Context, int64) (cv.Record, bool, error) {
	return cv.Record{}, f.ok, f.err
}

func rankHits() *fakeSearcher {
	hit := func(slug string) search.JobDocument {
		return search.JobDocument{Job: jobview.Job{PublicSlug: slug, Title: slug}}
	}
	return &fakeSearcher{res: search.SearchResult{Total: 3, Hits: []search.JobDocument{hit("a"), hit("b"), hit("c")}}}
}

func feedRanked(t *testing.T, prof userprofile.Profile, cvs fakeCVs, sc *fakeScorer) (slugs []string, ranking any) {
	t.Helper()
	rr := feed.NewReranker(sc, nil, time.Second, time.Hour)
	app, iss := feedAppRank(rankHits(), feedProfile{p: prof}, feedPlansFake{}, rr, cvs)
	status, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}
	for _, c := range body["data"].([]any) {
		slugs = append(slugs, c.(map[string]any)["public_slug"].(string))
	}
	return slugs, body["meta"].(map[string]any)["ranking"]
}

func TestFeed_RerankByFit(t *testing.T) {
	sc := &fakeScorer{scores: map[string]float64{"a": 0.1, "b": 0.9, "c": 0.5}}
	slugs, ranking := feedRanked(t, userprofile.Profile{Skills: []string{"go"}}, fakeCVs{ok: true}, sc)
	if ranking != "fit" || len(slugs) != 3 || slugs[0] != "b" || slugs[1] != "c" || slugs[2] != "a" {
		t.Errorf("ranking = %v, slugs = %v", ranking, slugs)
	}
}

func TestFeed_RerankFallsBackToSkills(t *testing.T) {
	for name, cvs := range map[string]fakeCVs{"no base cv": {}, "cv read error": {err: errors.New("db down")}} {
		t.Run(name, func(t *testing.T) {
			sc := &fakeScorer{}
			slugs, ranking := feedRanked(t, userprofile.Profile{Skills: []string{"go"}}, cvs, sc)
			if ranking != "skills" || len(slugs) != 3 || slugs[0] != "a" || sc.calls != 0 {
				t.Errorf("ranking = %v, slugs = %v, calls = %d", ranking, slugs, sc.calls)
			}
		})
	}
}

func TestFeed_DegradedRankingIsDateWithoutJev(t *testing.T) {
	sc := &fakeScorer{}
	_, ranking := feedRanked(t, userprofile.Profile{Seniorities: []string{"senior"}}, fakeCVs{ok: true}, sc)
	if ranking != "date" || sc.calls != 0 {
		t.Errorf("ranking = %v, calls = %d", ranking, sc.calls)
	}
}

func TestFeed_NoJevKeyRanksBySkills(t *testing.T) {
	rr := newFeedReranker("", nil)
	if rr != nil {
		t.Fatal("a reranker without a Jev key must be nil")
	}
	app, iss := feedAppRank(rankHits(), feedProfile{p: userprofile.Profile{Skills: []string{"go"}}}, feedPlansFake{}, rr, fakeCVs{ok: true})
	status, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if status != fiber.StatusOK || body["meta"].(map[string]any)["ranking"] != "skills" {
		t.Errorf("status = %d, meta = %v", status, body["meta"])
	}
}
