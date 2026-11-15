//go:build integration

package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// gatedFitModel is a fitModel that holds its FIRST call until released, so a test can inspect
// the world while the cold-start chain is provably mid-flight.
type gatedFitModel struct {
	fitModel
	started     chan struct{}
	release     chan struct{}
	holdOnce    sync.Once
	releaseOnce sync.Once
}

func newGatedFitModel(t *testing.T) *gatedFitModel {
	m := &gatedFitModel{
		fitModel: fitModel{resp: []string{fitStage1, fitStage2, fitStage3}},
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	t.Cleanup(m.letGo)
	return m
}

// letGo releases the model however many times it is called, so a test may release it
// explicitly and still be cleaned up after.
func (m *gatedFitModel) letGo() { m.releaseOnce.Do(func() { close(m.release) }) }

func (m *gatedFitModel) GenerateContent(ctx context.Context, msgs []llms.MessageContent, opts ...llms.CallOption) (*llms.ContentResponse, error) {
	m.holdOnce.Do(func() {
		close(m.started)
		<-m.release
	})
	return m.fitModel.GenerateContent(ctx, msgs, opts...)
}

// TestAutopilotAnswersBeforeTheColdStartAnalysis: the response must open BEFORE the cold-start
// fit analysis runs, not after it.
//
// The pre-run used to happen in the handler, ahead of streamSSE, so on a vacancy with nothing
// cached the response stayed silent for as long as the three-stage chain took — measured at
// 2m6s on prod, 2026-08-21, against nginx's 60s default. The proxy cut five runs that day and
// the browser reported "could not start the run" for runs that were running fine and went on
// to finish. Nothing downstream can tell those two apart, so the fix has to be here: open the
// stream first, wait inside it, where the heartbeat is already ticking.
func TestAutopilotAnswersBeforeTheColdStartAnalysis(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	fitM := newGatedFitModel(t)
	h, app := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-ttfb@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, _ := seedTailoringSession(t, pool, h, userID)

	addr := serveOnSocket(t, app)
	conn := dialAutopilotTurn(t, addr, sessionID, cookie)
	defer func() { _ = conn.Close() }()

	select {
	case <-fitM.started:
	case <-time.After(15 * time.Second):
		t.Fatal("the cold-start analysis never started")
	}

	// The chain is held mid-flight. The response line must already be on the wire: what a
	// proxy counts is silence from the upstream, and this is the silence that cost the runs.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("nothing answered while the cold-start analysis is still running (%v) — the "+
			"stream opens only after the chain, which is exactly what a proxy times out", err)
	}
	if !strings.HasPrefix(status, "HTTP/1.1 200") {
		t.Fatalf("response line %q, want HTTP/1.1 200", strings.TrimSpace(status))
	}
	fitM.letGo()
}

// TestAutopilotRefreshesAnalysisAfterEveryRun: the fit analysis is no longer a frozen
// snapshot of the base profile (see docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md).
// An autopilot run must recompute and overwrite the cached (user, job) row once it ends,
// even when a (now-stale) analysis was already cached and the run itself made zero edits.
func TestAutopilotRefreshesAnalysisAfterEveryRun(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	fitM := fullFitChain()
	h, app := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-refresh@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	var jobID int64
	if err := pool.QueryRow(context.Background(), `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	// A stale analysis under a model id the fake never produces, so a later match on the
	// LIVE model id (empty string — llm.NewWithModel sets none) can only mean the row was
	// actually overwritten, not left alone.
	if err := queries.UpsertUserJobAnalysis(context.Background(), db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: []byte(`{"verdict":"Good Fit","overall_score":70}`), Model: "stale-model",
	}); err != nil {
		t.Fatalf("seed cached analysis: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if fitM.n != 3 {
		t.Errorf("fit model called %d times, want exactly 3 (one full chain run) — a cached analysis must still be refreshed once, not skipped and not recomputed twice", fitM.n)
	}
	row, err := queries.GetUserJobAnalysis(context.Background(), db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatalf("read cached analysis: %v", err)
	}
	if row.Model == "stale-model" {
		t.Error("cached analysis still carries the pre-run model stamp — the row was not overwritten")
	}
}

// dialAutopilotTurn opens an autopilot run on a raw socket, the same way dialTurn opens an
// ordinary message — the autopilot endpoint reads no body, so none is sent.
func dialAutopilotTurn(t *testing.T, addr, sessionID, cookie string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	request := fmt.Sprintf("POST /api/v1/assistant/sessions/%s/autopilot HTTP/1.1\r\n"+
		"Host: 127.0.0.1\r\nCookie: %s=%s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
		sessionID, auth.CookieName, cookie)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	return conn
}

// dialAutopilotTurnInBackground opens an autopilot run and keeps reading it, exactly like
// startTurnInBackground does for an ordinary message. The returned channel closes when the
// stream ends.
func dialAutopilotTurnInBackground(t *testing.T, addr, sessionID, cookie string) <-chan struct{} {
	t.Helper()
	conn := dialAutopilotTurn(t, addr, sessionID, cookie)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	return done
}

// TestAutopilotRefreshSurvivesACancelledRun: the guaranteed post-run refresh (see
// TestAutopilotRefreshesAnalysisAfterEveryRun) must not be defeated by CancelAssistantTurn —
// cancelling shares the run's own ctx, and the refresh runs on context.WithoutCancel(ctx)
// specifically so a candidate who stops a run mid-flight still gets a current fit analysis,
// not the code silently skipping it because the ctx it inherited was already dead.
func TestAutopilotRefreshSurvivesACancelledRun(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	turnM := newDisconnectModel(t)
	fitM := fullFitChain()
	h, app := newAutopilotHarness(t, pool, iss, turnM, fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-cancel-refresh@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	var jobID int64
	if err := pool.QueryRow(context.Background(), `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	// Seeded so the pre-run ensure step is a no-op (cache hit): every fitM call below can
	// only come from the post-run refresh, isolating exactly the thing this test checks.
	if err := queries.UpsertUserJobAnalysis(context.Background(), db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: []byte(`{"verdict":"Good Fit","overall_score":70}`), Model: "stale-model",
	}); err != nil {
		t.Fatalf("seed cached analysis: %v", err)
	}

	addr := serveOnSocket(t, app)
	streamed := dialAutopilotTurnInBackground(t, addr, sessionID, cookie)

	select {
	case <-turnM.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never started")
	}

	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+sessionID+"/cancel", cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("cancel: status %d, want 204", resp.StatusCode)
	}
	turnM.letGo()
	<-streamed

	// The refresh runs after runner.Run returns, inside the same detached goroutine as the
	// turn — poll briefly rather than assuming it has already landed the instant the stream
	// closed.
	deadline := time.Now().Add(5 * time.Second)
	for fitM.n != 3 {
		if time.Now().After(deadline) {
			t.Fatalf("fit model called %d times after 5s, want 3 — the post-run refresh must survive a cancelled run", fitM.n)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := queries.GetUserJobAnalysis(context.Background(),
		db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID}); err != nil {
		t.Errorf("analysis not cached after a cancelled run: %v", err)
	}
}
