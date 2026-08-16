//go:build integration

// Integration tests for the real coalescing between the two entry points that can each decide
// to run the three-stage fit chain for the same (user, job): the cold-start autopilot's
// invisible ensureCachedAnalysis and the tailoring workspace's visible StreamMatchAnalysis.
// See match_analysis_coordinator.go and the tailor cold-start animation design.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

// blockingFitModel blocks the FIRST GenerateContent call (Stage 1) until release is closed, so
// a test can deterministically observe "a chain is under way" (started) and hold it there while
// a concurrent caller for the same (user, job) races in. Every call after the first proceeds
// immediately, cycling the canned stage responses exactly as fitModel does.
type blockingFitModel struct {
	resp    []string
	n       int
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingFitModel() *blockingFitModel {
	return &blockingFitModel{
		resp:    []string{fitStage1, fitStage2, fitStage3},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (m *blockingFitModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	m.once.Do(func() {
		close(m.started)
		<-m.release
	})
	r := m.resp[m.n%len(m.resp)]
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r}}}, nil
}
func (*blockingFitModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// seedCoordinatorUser creates a user with a stored, structured CV and a banked career — what
// the fit chain requires to produce an analysis — with an email distinguished by suffix so a
// test seeding more than one user doesn't collide.
func seedCoordinatorUser(t *testing.T, pool *pgxpool.Pool, queries *db.Queries, suffix string) (int64, *resume.Store) {
	t.Helper()
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, "coalesce-"+suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	if _, err := store.Put(ctx, userID, "text/plain", []byte("5y Go at Acme.")); err != nil {
		t.Fatalf("seed CV: %v", err)
	}
	if err := store.SetStructured(ctx, userID, resumeextract.Structured{Summary: "5y Go at Acme.", Skills: []string{"Go"}}, "test-model", resumeUploadedAt); err != nil {
		t.Fatalf("seed structured: %v", err)
	}
	seedBankedCareer(t, queries, userID)
	return userID, store
}

// seedCoordinatorJob seeds one job, slugged and hashed from prefix+i. Callers that only need
// one job pass i=0; TestMatchAnalysisCoordinatorSecondStreamNeverSeesAnalysisUnavailable seeds
// a fresh one per loop iteration — reusing a single (user, job) across iterations would let a
// later iteration's GetUserJobAnalysis hit an earlier iteration's row before the coordinator
// ever ran, hiding the very race that test exists to catch.
func seedCoordinatorJob(t *testing.T, pool *pgxpool.Pool, prefix string, i int) int64 {
	t.Helper()
	var jobID int64
	ext := fmt.Sprintf("%s-%d", prefix, i)
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, description, company_slug, public_slug, skills, content_hash)
		 VALUES ('test', $1, 'http://e.test', 'Senior Go Engineer', 'Build backends.', 'acme', $2, ARRAY['go'], $1)
		 RETURNING id`, ext, coordinatorJobSlug(prefix, i)).Scan(&jobID); err != nil {
		t.Fatalf("seed job %d: %v", i, err)
	}
	return jobID
}

func coordinatorJobSlug(prefix string, i int) string {
	return fmt.Sprintf("%s-race-job-%d", prefix, i)
}

// newCoordinatorHandlers builds the matchHandlers a coordinator test drives requests against —
// model bound to a fresh Analyzer, everything else the fixed fixture StreamMatchAnalysis and
// ensureCachedAnalysis both need.
func newCoordinatorHandlers(pool *pgxpool.Pool, queries *db.Queries, store *resume.Store, model llms.Model) *matchHandlers {
	return &matchHandlers{
		queries:            queries,
		userProfile:        userprofile.New(ownedProfile()),
		resume:             store,
		matchAnalysis:      matchanalysis.NewAnalyzer(llm.NewWithModel(model)),
		matchAnalysisCache: queries,
		credits:            credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3}),
	}
}

// registerEnsureRoute mounts a test-only route standing in for the cold-start autopilot's
// invisible pre-run: it calls the exact same unexported method PostAssistantAutopilot calls,
// against the same matchHandlers and (user, job) a stream request in the same test races.
func registerEnsureRoute(app *fiber.App, h *matchHandlers, userID int64, job db.Job) {
	app.Post("/test/ensure", func(c *fiber.Ctx) error {
		h.ensureCachedAnalysis(c, userID, job)
		return c.SendStatus(fiber.StatusOK)
	})
}

// asyncCall is one in-flight test request: done closes when it completes, and body (only
// meaningful for a stream request — startEnsure leaves it empty) holds its SSE response body.
type asyncCall struct {
	done chan struct{}
	body string
}

// startStream issues a GET to path with the caller's auth cookie in a goroutine, returning
// immediately so the test can interleave a second concurrent call before this one completes.
func startStream(t *testing.T, app *fiber.App, path, token string) *asyncCall {
	t.Helper()
	call := &asyncCall{done: make(chan struct{})}
	go func() {
		defer close(call.done)
		req := httptest.NewRequest(fiber.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Errorf("stream request %s: %v", path, err)
			return
		}
		defer resp.Body.Close()
		call.body = readSSEBody(t, resp.Body)
	}()
	return call
}

// startEnsure issues the test-only POST /test/ensure route in a goroutine — the coordinator
// side of a cold-start autopilot run, with no body worth capturing.
func startEnsure(t *testing.T, app *fiber.App) *asyncCall {
	t.Helper()
	call := &asyncCall{done: make(chan struct{})}
	go func() {
		defer close(call.done)
		resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/test/ensure", nil), 5000)
		if err != nil {
			t.Errorf("ensure request: %v", err)
			return
		}
		resp.Body.Close()
	}()
	return call
}

// waitDone blocks until call finishes or timeout elapses, failing the test in the latter case.
func waitDone(t *testing.T, call *asyncCall, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-call.done:
	case <-time.After(timeout):
		t.Fatal(msg)
	}
}

// waitStillRunning is waitDone's inverse: fails if call finishes before timeout elapses — used
// to assert a follower is genuinely blocked on the leader rather than having raced ahead.
func waitStillRunning(t *testing.T, call *asyncCall, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-call.done:
		t.Fatal(msg)
	case <-time.After(timeout):
	}
}

func TestMatchAnalysisCoordinatorEnsureCachedAnalysisLeadsStreamFollows(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	userID, store := seedCoordinatorUser(t, pool, queries, "a")
	jobID := seedCoordinatorJob(t, pool, "a", 0)
	job, err := queries.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("read job: %v", err)
	}

	model := newBlockingFitModel()
	h := newCoordinatorHandlers(pool, queries, store, model)

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	registerEnsureRoute(app, h, userID, job)
	app.Get("/api/v1/jobs/:slug/fit/stream", auth.RequireAuth(iss, testVersions), h.StreamMatchAnalysis)

	ensure := startEnsure(t, app)
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("ensureCachedAnalysis never started its compute")
	}

	stream := startStream(t, app, "/api/v1/jobs/"+coordinatorJobSlug("a", 0)+"/fit/stream", token)
	waitStillRunning(t, stream, 200*time.Millisecond, "the follower stream returned before the leader was released")

	close(model.release)

	waitDone(t, ensure, 2*time.Second, "ensureCachedAnalysis never finished")
	waitDone(t, stream, 2*time.Second, "the follower stream never finished")

	if model.n != 3 {
		t.Errorf("fit model called %d times, want 3 (one chain, not two — no double spend)", model.n)
	}

	names := sseEvents(t, stream.body)
	if len(names) == 0 || names[0] != "meta" {
		t.Fatalf("follower's first event = %v, want meta; all=%v", names, names)
	}
	if names[len(names)-1] != "final" {
		t.Errorf("follower's last event = %q, want final; all=%v", names[len(names)-1], names)
	}
	var stageDone, stageStart int
	for _, n := range names {
		if n == "stage_done" {
			stageDone++
		}
		if n == "stage_start" {
			stageStart++
		}
	}
	if stageDone != 3 {
		t.Errorf("follower stage_done count = %d, want 3; all=%v", stageDone, names)
	}
	if stageStart != 0 {
		t.Errorf("follower must never see stage_start (it never watched progress); all=%v", names)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1 AND job_id=$2`, userID, jobID).Scan(&rows); err != nil {
		t.Fatalf("count cache rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("cache rows = %d, want 1", rows)
	}

	// ensureCachedAnalysis (the leader here) never touches credits — but the stream (the
	// follower) is a genuinely paying request for a never-analysed job, and must still cost
	// its own credit even though it didn't run the chain itself: h.debitMatch is idempotent
	// per (user, feature, job), so this is one row, not zero (a free ride for a paying
	// caller) or two (double-billed).
	var debits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='debit' AND feature='match'`, userID).Scan(&debits); err != nil {
		t.Fatalf("count match debits: %v", err)
	}
	if debits != 1 {
		t.Errorf("match debits = %d, want 1 — the follower is a genuinely new paying request and must still be charged once, not given a free ride because ensureCachedAnalysis led", debits)
	}
}

// TestMatchAnalysisCoordinatorStreamLeadsEnsureCachedAnalysisFollowsAndLeaderStillBills is the
// mirror of the test above with roles reversed: the paying stream leads, the free
// ensureCachedAnalysis pre-run follows. The leader must bill exactly as it would with no
// follower at all — an unmetered follower joining must never suppress a real charge (that
// was the actual bug: a leader used to skip billing whenever ANY follower joined, which let a
// second browser tab or a reload silently dodge the credit a new job's analysis costs).
func TestMatchAnalysisCoordinatorStreamLeadsEnsureCachedAnalysisFollowsAndLeaderStillBills(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	userID, store := seedCoordinatorUser(t, pool, queries, "b")
	jobID := seedCoordinatorJob(t, pool, "b", 0)
	job, err := queries.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("read job: %v", err)
	}

	model := newBlockingFitModel()
	h := newCoordinatorHandlers(pool, queries, store, model)

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	registerEnsureRoute(app, h, userID, job)
	app.Get("/api/v1/jobs/:slug/fit/stream", auth.RequireAuth(iss, testVersions), h.StreamMatchAnalysis)

	stream := startStream(t, app, "/api/v1/jobs/"+coordinatorJobSlug("b", 0)+"/fit/stream", token)

	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the visible stream never started its compute")
	}

	ensure := startEnsure(t, app)
	waitStillRunning(t, ensure, 200*time.Millisecond, "ensureCachedAnalysis (the follower) returned before the leader was released")

	close(model.release)

	waitDone(t, stream, 2*time.Second, "the leader stream never finished")
	waitDone(t, ensure, 2*time.Second, "ensureCachedAnalysis never finished")

	if model.n != 3 {
		t.Errorf("fit model called %d times, want 3 (one chain, not two — no double spend)", model.n)
	}

	names := sseEvents(t, stream.body)
	joined := strings.Join(names, ",")
	for _, want := range []string{"stage_start", "requirements", "dimensions", "final"} {
		if !strings.Contains(joined, want) {
			t.Errorf("leader stream missing %q event — it must show real progress, not a synthesized burst; got %v", want, names)
		}
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1 AND job_id=$2`, userID, jobID).Scan(&rows); err != nil {
		t.Fatalf("count cache rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("cache rows = %d, want 1", rows)
	}

	var debits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='debit' AND feature='match'`, userID).Scan(&debits); err != nil {
		t.Fatalf("count match debits: %v", err)
	}
	if debits != 1 {
		t.Errorf("match debits = %d, want 1 — the leader is a genuinely new paying request and an unmetered follower joining must not suppress its charge", debits)
	}
}

// TestMatchAnalysisCoordinatorSecondStreamNeverSeesAnalysisUnavailable is a regression test
// for a real race caught live: a leader used to call done() (releasing any follower)
// immediately after AnalyzeStream returned, BEFORE its own h.cacheAnalysis write landed. A
// follower unblocks the instant done() runs and reads the cache right away
// (followMatchAnalysis) — so a follower that won that race read an empty cache and degraded
// straight to "analysis unavailable", even though the leader was about to succeed.
//
// ensureCachedAnalysis's own follower branch can't catch this (it never reads the cache — see
// the two tests above), so this drives TWO concurrent StreamMatchAnalysis calls instead: the
// second becomes a follower via followMatchAnalysis, which does read the cache and is exactly
// where the live bug surfaced. Repeated: the race window was narrow (a channel close vs. a
// local DB round trip), so a single pass is not guaranteed to reproduce it.
func TestMatchAnalysisCoordinatorSecondStreamNeverSeesAnalysisUnavailable(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	userID, store := seedCoordinatorUser(t, pool, queries, "c")
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID, testTokenVersion)

	const iterations = 15
	for i := 0; i < iterations; i++ {
		jobID := seedCoordinatorJob(t, pool, "cc", i)
		path := "/api/v1/jobs/" + coordinatorJobSlug("cc", i) + "/fit/stream"

		model := newBlockingFitModel()
		h := newCoordinatorHandlers(pool, queries, store, model)
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		app.Get("/api/v1/jobs/:slug/fit/stream", auth.RequireAuth(iss, testVersions), h.StreamMatchAnalysis)

		leader := startStream(t, app, path, token)
		select {
		case <-model.started:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: leader never started its compute", i)
		}

		follower := startStream(t, app, path, token)
		// Give the follower a moment to actually reach the coordinator and register before
		// releasing — otherwise it might not join as a follower at all on a slow scheduler.
		time.Sleep(30 * time.Millisecond)
		close(model.release)

		waitDone(t, leader, 2*time.Second, fmt.Sprintf("iteration %d: leader stream never finished", i))
		waitDone(t, follower, 2*time.Second, fmt.Sprintf("iteration %d: follower stream never finished", i))

		followerNames := sseEvents(t, follower.body)
		if len(followerNames) == 0 || followerNames[len(followerNames)-1] != "final" {
			t.Fatalf("iteration %d: follower's stream = %v (leader's = %v) — must end in final, never \"analysis unavailable\"",
				i, followerNames, sseEvents(t, leader.body))
		}
		if strings.Contains(follower.body, "analysis unavailable") {
			t.Fatalf("iteration %d: follower saw \"analysis unavailable\" — done() raced ahead of the cache write; body=%q", i, follower.body)
		}

		var rows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1 AND job_id=$2`, userID, jobID).Scan(&rows); err != nil {
			t.Fatalf("iteration %d: count cache rows: %v", i, err)
		}
		if rows != 1 {
			t.Errorf("iteration %d: cache rows = %d, want 1", i, rows)
		}
	}
}

// blockingFailingFitModel blocks the first call until release, then always fails — used to
// deterministically make a leader's RECOMPUTE attempt fail while a follower is already
// waiting on it.
type blockingFailingFitModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingFailingFitModel() *blockingFailingFitModel {
	return &blockingFailingFitModel{started: make(chan struct{}), release: make(chan struct{})}
}

func (m *blockingFailingFitModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	m.once.Do(func() {
		close(m.started)
		<-m.release
	})
	return nil, errors.New("upstream llm exploded")
}
func (*blockingFailingFitModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// TestMatchAnalysisCoordinatorFollowerNeverServesStaleAnalysisAfterFailedRecompute is a
// regression test for a real gap: followMatchAnalysis used to trust whatever was in the cache
// unconditionally, with no way to tell "the leader just computed this successfully" apart from
// "the leader's recompute failed and this is what a PREVIOUS successful run left behind". A
// follower joining a failed recompute would replay that stale row as a normal `final` event —
// masking a failed recompute as a successful fresh one. run.succeeded is what closes this: a
// follower must see stream_error whenever the leader's attempt failed, never the old analysis.
func TestMatchAnalysisCoordinatorFollowerNeverServesStaleAnalysisAfterFailedRecompute(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	userID, store := seedCoordinatorUser(t, pool, queries, "d")
	jobID := seedCoordinatorJob(t, pool, "d", 0)

	// Seed a stale cached row directly, as if a prior successful analysis already ran — a
	// distinctive sentinel score so the test can tell "stale row leaked through" apart from
	// "a real (if canned) analysis was served".
	staleAnalysis := matchanalysis.Analysis{OverallScore: 999, Verdict: "Stale Sentinel"}
	blob, err := json.Marshal(staleAnalysis)
	if err != nil {
		t.Fatalf("marshal stale analysis: %v", err)
	}
	if err := queries.UpsertUserJobAnalysis(ctx, db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: blob, Model: "stale-model", Language: "en",
	}); err != nil {
		t.Fatalf("seed stale analysis: %v", err)
	}

	model := newBlockingFailingFitModel()
	h := newCoordinatorHandlers(pool, queries, store, model)

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug/fit/stream", auth.RequireAuth(iss, testVersions), h.StreamMatchAnalysis)
	path := "/api/v1/jobs/" + coordinatorJobSlug("d", 0) + "/fit/stream"

	leader := startStream(t, app, path, token)
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the leader's recompute never started")
	}

	follower := startStream(t, app, path, token)
	waitStillRunning(t, follower, 200*time.Millisecond, "the follower returned before the leader's failed recompute was released")

	close(model.release)

	waitDone(t, leader, 2*time.Second, "the leader stream never finished")
	waitDone(t, follower, 2*time.Second, "the follower stream never finished")

	leaderNames := sseEvents(t, leader.body)
	if leaderNames[len(leaderNames)-1] != "stream_error" {
		t.Errorf("leader's last event = %q, want stream_error (the recompute failed); all=%v", leaderNames[len(leaderNames)-1], leaderNames)
	}

	followerNames := sseEvents(t, follower.body)
	if followerNames[len(followerNames)-1] != "stream_error" {
		t.Errorf("follower's last event = %q, want stream_error — it must report the leader's failure, not replay the stale cached row; all=%v", followerNames[len(followerNames)-1], followerNames)
	}
	if strings.Contains(follower.body, "999") || strings.Contains(follower.body, "Stale Sentinel") {
		t.Errorf("follower's body contains the stale sentinel analysis — a failed recompute was masked as success; body=%q", follower.body)
	}

	// The stale row must survive untouched — a failed recompute must not overwrite good data
	// with nothing, and must not have been silently replaced either.
	row, err := queries.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatalf("re-read cache row: %v", err)
	}
	if !strings.Contains(string(row.Analysis), "999") {
		t.Errorf("cache row changed after a failed recompute; got %s", row.Analysis)
	}
}

// readSSEBody reads a full (bounded) SSE response body as a string.
func readSSEBody(t *testing.T, r io.Reader) string {
	t.Helper()
	var b strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteString("\n")
	}
	return b.String()
}
