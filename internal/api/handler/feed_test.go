package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (f feedExcluded) FeedExcludedJobIDs(context.Context, int64, time.Time) ([]int64, error) {
	return f, nil
}

// feedStateFake is the watermark store: seen is the stored watermark (nil: first visit),
// hw the drain high-water (zero: nothing pending).
type feedStateFake struct {
	seen     *time.Time
	hw       time.Time
	advanced []time.Time
}

func (f *feedStateFake) GetOrInitFeedSeen(_ context.Context, _ int64, _ string, initAt time.Time) (time.Time, error) {
	if f.seen == nil {
		f.seen = &initAt
	}
	return *f.seen, nil
}

func (f *feedStateFake) AdvanceFeedSeen(_ context.Context, _ int64, _ string, at time.Time) error {
	f.advanced = append(f.advanced, at)
	return nil
}

func (f *feedStateFake) SearchDrainHighWater(context.Context) (time.Time, bool, error) {
	return f.hw, !f.hw.IsZero(), nil
}

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
	h := &feedHandlers{search: s, profiles: prof, excluded: feedExcluded{10}, plans: plans, rank: rank, cvs: cvs, state: &feedStateFake{}, cursors: feed.NewSigner([]byte("cursor-key"))}
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

type feedExcludedAt struct{ before []time.Time }

func (f *feedExcludedAt) FeedExcludedJobIDs(_ context.Context, _ int64, before time.Time) ([]int64, error) {
	f.before = append(f.before, before)
	return []int64{10}, nil
}

var feedUpdated = time.Unix(1_700_000_000, 0)

func feedPagingApp(s searcher, ex *feedExcludedAt, updated time.Time) (*fiber.App, *auth.Issuer) {
	return feedStateApp(s, ex, updated, &feedStateFake{})
}

func feedStateApp(s searcher, ex *feedExcludedAt, updated time.Time, st *feedStateFake) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	prof := feedProfile{p: userprofile.Profile{Skills: []string{"go"}, UpdatedAt: &updated}}
	h := &feedHandlers{search: s, profiles: prof, excluded: ex, plans: feedPlansFake{}, state: st, cursors: feed.NewSigner([]byte("cursor-key"))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/feed", auth.RequireAuth(iss, testVersions), h.Feed)
	app.Post("/api/v1/me/feed/seen", auth.RequireAuth(iss, testVersions), h.Seen)
	return app, iss
}

func nextCursorOf(body map[string]any) string {
	s, _ := body["meta"].(map[string]any)["next_cursor"].(string)
	return s
}

func TestFeed_CursorPagesOverAFixedSnapshot(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 3, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}, {ID: 2, Job: jobview.Job{PublicSlug: "b"}}}}}
	ex := &feedExcludedAt{}
	app, iss := feedPagingApp(fake, ex, feedUpdated)

	// offset is ignored: page 1 starts at 0 and hands out a cursor.
	asked := time.Now()
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=2&offset=20")
	cur := nextCursorOf(body)
	if cur == "" || fake.got.Offset != 0 || fake.got.Limit != 2 {
		t.Fatalf("page 1: cursor=%q params=%#v", cur, fake.got)
	}
	acted := ex.before[0]
	if !acted.Equal(acted.Truncate(time.Second)) {
		t.Errorf("action cutoff %v is not whole seconds", acted)
	}
	// Rounded up: an action earlier in the request's second is before the cutoff.
	if !acted.After(asked) {
		t.Errorf("action cutoff %v not after the request at %v", acted, asked)
	}
	// Jobs stop at the last fully elapsed second: one created later in this second may be
	// inserted after the read.
	snap := asked.Unix() - 1
	if !filterHas(fake.got.Filter.([][]string), fmt.Sprintf("created_ts <= %d", snap)) {
		t.Errorf("page 1 not bounded by the snapshot %d: %#v", snap, fake.got.Filter)
	}

	// An action between pages changes nothing: page 2 reuses the snapshot and the next window.
	fake.res = search.SearchResult{Total: 3, Hits: []search.JobDocument{{ID: 3, Job: jobview.Job{PublicSlug: "c"}}}}
	status, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=2&cursor="+cur)
	if status != fiber.StatusOK || fake.got.Offset != 2 {
		t.Fatalf("page 2: status=%d params=%#v", status, fake.got)
	}
	if !ex.before[1].Equal(acted) || !filterHas(fake.got.Filter.([][]string), fmt.Sprintf("created_ts <= %d", snap)) {
		t.Errorf("page 2 snapshot moved: %v vs %v", ex.before[1], acted)
	}
	if nextCursorOf(body) != "" {
		t.Errorf("last (short) page still has a cursor")
	}
}

func TestFeed_NoCursorAtTheTotalOrPastTheWindow(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 1, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
	app, iss := feedPagingApp(fake, &feedExcludedAt{}, feedUpdated)
	if _, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1"); nextCursorOf(body) != "" {
		t.Errorf("cursor at the total")
	}
	h := &feedHandlers{cursors: feed.NewSigner([]byte("k"))}
	if got := h.nextCursor(feed.Cursor{Offset: maxPageWindow - 1}, 0, 1, 1, 1_000_000); got != "" {
		t.Errorf("cursor past maxPageWindow")
	}
}

func TestFeed_ChangedProfileIsSourceChanged(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 9, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
	app, iss := feedPagingApp(fake, &feedExcludedAt{}, feedUpdated)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
	cur := nextCursorOf(body)

	edited, iss2 := feedPagingApp(fake, &feedExcludedAt{}, feedUpdated.Add(time.Second))
	*fake = fakeSearcher{}
	status, body := deckGet(t, edited, iss2, "/api/v1/me/feed?cursor="+cur)
	if status != fiber.StatusOK || body["meta"].(map[string]any)["reason"] != feedReasonSourceChanged || len(body["data"].([]any)) != 0 {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searched after the source changed")
	}
}

func TestFeed_BadCursorIs400(t *testing.T) {
	app, iss := feedPagingApp(&fakeSearcher{}, &feedExcludedAt{}, feedUpdated)
	sg := feed.NewSigner([]byte("cursor-key"))
	now := time.Now()
	good := feed.Cursor{UserID: 7, Source: "profile", SnapshotAt: now.Unix(), Offset: 1, ExpiresAt: now.Add(time.Hour).Unix()}
	tampered := sg.Sign(good)
	flipped := byte('A') // a different first payload character, so the tag no longer matches
	if tampered[0] == 'A' {
		flipped = 'B'
	}
	tampered = string(flipped) + tampered[1:]
	expired, otherUser, otherSource := good, good, good
	expired.ExpiresAt = now.Add(-time.Minute).Unix()
	otherUser.UserID = 8
	otherSource.Source = "saved:1"
	for name, c := range map[string]string{
		"tampered": tampered, "malformed": "nonsense", "expired": sg.Sign(expired),
		"other user": sg.Sign(otherUser), "other source": sg.Sign(otherSource),
	} {
		if status, _ := deckGet(t, app, iss, "/api/v1/me/feed?cursor="+c); status != fiber.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, status)
		}
	}
}

// scriptedSearcher answers by the phase its filter asks for: a created_ts lower bound is the new phase.
type scriptedSearcher struct {
	newHits, restHits []search.JobDocument
	calls             []search.SearchParams
}

func (f *scriptedSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.calls = append(f.calls, p)
	hits := f.restHits
	for _, g := range p.Filter.([][]string) {
		if len(g) == 1 && strings.HasPrefix(g[0], "created_ts >= ") {
			hits = f.newHits
		}
	}
	end := min(p.Offset+p.Limit, len(hits))
	return search.SearchResult{Total: int64(len(hits)), Hits: hits[min(p.Offset, end):end]}, nil
}

func docs(slugs ...string) []search.JobDocument {
	out := make([]search.JobDocument, len(slugs))
	for i, s := range slugs {
		out[i] = search.JobDocument{ID: int64(i + 1), Job: jobview.Job{PublicSlug: s}}
	}
	return out
}

func slugsAndNew(body map[string]any) (slugs []string, isNew []bool) {
	for _, c := range body["data"].([]any) {
		m := c.(map[string]any)
		slugs = append(slugs, m["public_slug"].(string))
		isNew = append(isNew, m["is_new"].(bool))
	}
	return
}

func TestFeed_FirstVisitHasNothingNew(t *testing.T) {
	st := &feedStateFake{}
	fake := &scriptedSearcher{newHits: docs("n1"), restHits: docs("r1", "r2")}
	app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	slugs, isNew := slugsAndNew(body)
	meta := body["meta"].(map[string]any)
	if len(slugs) != 2 || slugs[0] != "r1" || isNew[0] || isNew[1] || meta["phase"] != "rest" || meta["seen_cursor"] != nil {
		t.Errorf("slugs=%v isNew=%v meta=%v", slugs, isNew, meta)
	}
	if st.seen == nil || len(fake.calls) != 1 {
		t.Errorf("watermark not created or new phase searched: %v, %d calls", st.seen, len(fake.calls))
	}
}

func TestFeed_NewBeforeRestWithBoundaryFillAndSeen(t *testing.T) {
	seen := time.Now().Add(-time.Hour).Truncate(time.Second)
	st := &feedStateFake{seen: &seen}
	fake := &scriptedSearcher{newHits: docs("n1", "n2", "n3"), restHits: docs("r1", "r2", "r3", "r4", "r5")}
	app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, st)

	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=5")
	slugs, isNew := slugsAndNew(body)
	meta := body["meta"].(map[string]any)
	if fmt.Sprint(slugs) != "[n1 n2 n3 r1 r2]" || fmt.Sprint(isNew) != "[true true true false false]" || meta["phase"] != "new" {
		t.Fatalf("slugs=%v isNew=%v meta=%v", slugs, isNew, meta)
	}
	// The window filters: new is after the watermark, rest is at or before it.
	if !filterHas(fake.calls[0].Filter.([][]string), fmt.Sprintf("created_ts >= %d", seen.Unix()+1)) ||
		!filterHas(fake.calls[1].Filter.([][]string), fmt.Sprintf("created_ts <= %d", seen.Unix())) || fake.calls[1].Offset != 0 || fake.calls[1].Limit != 2 {
		t.Errorf("phase filters: %#v", fake.calls)
	}
	sc, _ := meta["seen_cursor"].(string)
	if sc == "" {
		t.Fatal("the page where new ends carries seen_cursor")
	}
	// The next page continues in the rest phase at the rest offset, with no seen_cursor.
	_, body = deckGet(t, app, iss, "/api/v1/me/feed?limit=5&cursor="+nextCursorOf(body))
	slugs, isNew = slugsAndNew(body)
	if fmt.Sprint(slugs) != "[r3 r4 r5]" || isNew[0] || body["meta"].(map[string]any)["seen_cursor"] != nil || nextCursorOf(body) != "" {
		t.Errorf("rest page: slugs=%v meta=%v", slugs, body["meta"])
	}
	// Seen moves the watermark; the page cursor is not a seen cursor.
	if status, _ := deckPost(t, app, iss, "/api/v1/me/feed/seen", `{"cursor":"`+sc+`"}`); status != fiber.StatusNoContent || len(st.advanced) != 1 {
		t.Fatalf("seen: status=%d advanced=%v", status, st.advanced)
	}
	if status, _ := deckPost(t, app, iss, "/api/v1/me/feed/seen", `{"cursor":"nonsense"}`); status != fiber.StatusBadRequest {
		t.Errorf("bad seen cursor: status=%d", status)
	}
}

func TestFeed_ManyNewStaysInNewPhase(t *testing.T) {
	seen := time.Now().Add(-time.Hour).Truncate(time.Second)
	fake := &scriptedSearcher{newHits: docs("n1", "n2", "n3"), restHits: docs("r1")}
	app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, &feedStateFake{seen: &seen})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=2")
	slugs, isNew := slugsAndNew(body)
	if fmt.Sprint(slugs) != "[n1 n2]" || !isNew[1] || nextCursorOf(body) == "" || body["meta"].(map[string]any)["seen_cursor"] != nil {
		t.Fatalf("page 1: slugs=%v meta=%v", slugs, body["meta"])
	}
	_, body = deckGet(t, app, iss, "/api/v1/me/feed?limit=2&cursor="+nextCursorOf(body))
	slugs, _ = slugsAndNew(body)
	if fmt.Sprint(slugs) != "[n3 r1]" || body["meta"].(map[string]any)["seen_cursor"] == nil {
		t.Errorf("page 2: slugs=%v meta=%v", slugs, body["meta"])
	}
}

func TestFeed_SnapshotStaysBelowTheDrainHighWater(t *testing.T) {
	st := &feedStateFake{hw: time.Now().Add(-time.Hour).Truncate(time.Second)}
	ex := &feedExcludedAt{}
	app, iss := feedStateApp(&scriptedSearcher{restHits: docs("r1")}, ex, feedUpdated, st)
	asked := time.Now()
	deckGet(t, app, iss, "/api/v1/me/feed")
	if want := st.hw.Unix() - 1; st.seen.Unix() != want {
		t.Errorf("snapshot = %d, want %d (below the pending job's second)", st.seen.Unix(), want)
	}
	// Actions since the pending job still leave the feed: their cutoff is the session start.
	if !ex.before[0].After(asked) {
		t.Errorf("action cutoff %v follows the drain high-water, want after %v", ex.before[0], asked)
	}
}

func TestFeedSeen_RefusesPageCursorAndOtherUser(t *testing.T) {
	st := &feedStateFake{}
	app, iss := feedStateApp(&scriptedSearcher{restHits: docs("a", "b")}, &feedExcludedAt{}, feedUpdated, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
	if status, _ := deckPost(t, app, iss, "/api/v1/me/feed/seen", `{"cursor":"`+nextCursorOf(body)+`"}`); status != fiber.StatusBadRequest {
		t.Errorf("page cursor on seen: status=%d", status)
	}
	now := time.Now()
	foreign := feed.NewSigner([]byte("cursor-key")).Sign(feed.Cursor{UserID: 8, Source: "profile", Phase: feed.PhaseSeen, SnapshotAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix()})
	if status, _ := deckPost(t, app, iss, "/api/v1/me/feed/seen", `{"cursor":"`+foreign+`"}`); status != fiber.StatusBadRequest || len(st.advanced) != 0 {
		t.Errorf("foreign seen cursor: status=%d advanced=%v", status, st.advanced)
	}
}

func deckPost(t *testing.T, app *fiber.App, iss *auth.Issuer, target, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	token, _ := iss.Issue(7, testTokenVersion)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func TestFeed_NewExactlyFillsThePage(t *testing.T) {
	seen := time.Now().Add(-time.Hour).Truncate(time.Second)
	for name, tc := range map[string]struct {
		rest     []search.JobDocument
		wantNext bool
	}{"rest exists": {docs("r1"), true}, "no rest": {nil, false}} {
		t.Run(name, func(t *testing.T) {
			fake := &scriptedSearcher{newHits: docs("n1", "n2"), restHits: tc.rest}
			app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, &feedStateFake{seen: &seen})
			_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=2")
			slugs, _ := slugsAndNew(body)
			if fmt.Sprint(slugs) != "[n1 n2]" || body["meta"].(map[string]any)["seen_cursor"] == nil {
				t.Fatalf("slugs=%v meta=%v", slugs, body["meta"])
			}
			if got := nextCursorOf(body) != ""; got != tc.wantNext {
				t.Errorf("next_cursor present = %v, want %v", got, tc.wantNext)
			}
			if tc.wantNext {
				_, body = deckGet(t, app, iss, "/api/v1/me/feed?limit=2&cursor="+nextCursorOf(body))
				if slugs, _ := slugsAndNew(body); fmt.Sprint(slugs) != "[r1]" {
					t.Errorf("rest page = %v", slugs)
				}
			}
		})
	}
}

func TestFeed_ChangedProfileOnAnOffsetZeroRestCursorIsSourceChanged(t *testing.T) {
	seen := time.Now().Add(-time.Hour).Truncate(time.Second)
	fake := &scriptedSearcher{newHits: docs("n1", "n2"), restHits: docs("r1")}
	app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, &feedStateFake{seen: &seen})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=2")
	cur := nextCursorOf(body)
	edited, iss2 := feedStateApp(fake, &feedExcludedAt{}, feedUpdated.Add(time.Second), &feedStateFake{seen: &seen})
	fake.calls = nil
	_, body = deckGet(t, edited, iss2, "/api/v1/me/feed?limit=2&cursor="+cur)
	if body["meta"].(map[string]any)["reason"] != feedReasonSourceChanged || len(fake.calls) != 0 {
		t.Errorf("meta=%v calls=%d", body["meta"], len(fake.calls))
	}
}

func TestFeed_CursorWithoutPhaseIs400(t *testing.T) {
	app, iss := feedPagingApp(&fakeSearcher{}, &feedExcludedAt{}, feedUpdated)
	now := time.Now()
	old := feed.NewSigner([]byte("cursor-key")).Sign(feed.Cursor{UserID: 7, Source: "profile", SnapshotAt: now.Unix(), Offset: 20, ExpiresAt: now.Add(time.Hour).Unix()})
	if status, _ := deckGet(t, app, iss, "/api/v1/me/feed?cursor="+old); status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}
