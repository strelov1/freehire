//go:build integration

// Integration tests for PostAutoApplyReview's best-effort publish of
// auto-apply/review.decided (openspec/changes/auto-apply-inngest-orchestration): a
// recorded decision publishes the event, and a publish failure never changes the
// endpoint's own response.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// fakeEventPublisher records every publish call and, when err is set, fails every one of
// them — standing in for a real Inngest event API without a network call anywhere near
// these tests.
type fakeEventPublisher struct {
	mu    sync.Mutex
	calls []struct {
		queueID  int64
		decision string
	}
	// submitCalls records PublishSubmit calls separately — auto-apply-submit-trigger's
	// own tests only ever care about the count, not a decision string.
	submitCalls []int64
	err         error
}

func (f *fakeEventPublisher) PublishSubmit(_ context.Context, queueID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitCalls = append(f.submitCalls, queueID)
	return f.err
}

func (f *fakeEventPublisher) PublishReviewDecided(_ context.Context, queueID int64, decision string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		queueID  int64
		decision string
	}{queueID, decision})
	return f.err
}

// Calls returns the total number of publish calls of either kind.
func (f *fakeEventPublisher) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls) + len(f.submitCalls)
}

// waitForPublishCalls polls until events has recorded `want` calls or fails the test.
// publishSubmit/publishReviewDecided (auto_apply_review_publish.go) fire the real publish
// from a goroutine detached from the request — deliberately, so a slow event API cannot
// hold a candidate-facing response open — so a test observing the call count can no longer
// assume it landed by the time app.Test returns.
func waitForPublishCalls(t *testing.T, events *fakeEventPublisher, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := events.Calls(); got >= want {
			if got != want {
				t.Fatalf("publish calls = %d, want %d", got, want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("publish calls = %d, want %d (timed out waiting)", events.Calls(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPostAutoApplyReview_PublishesReviewDecidedEvent(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h, _ := newAutoApplyTailorAppFull(pool, iss, &turnModel{}, plan.DefaultConfig(), "", nil)
	events := &fakeEventPublisher{}
	h.events = events

	userID, cookie := autoApplyTailorUser(t, pool, iss, "publish@example.test")
	job := insertAutoApplyJob(t, pool, "publish-event")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	waitForPublishCalls(t, events, 1)
	if events.calls[0].queueID != queueID || events.calls[0].decision != "approved" {
		t.Errorf("published (queueID=%d, decision=%q), want (queueID=%d, decision=\"approved\")",
			events.calls[0].queueID, events.calls[0].decision, queueID)
	}
}

func TestPostAutoApplyReview_PublishFailureDoesNotChangeTheResponse(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h, _ := newAutoApplyTailorAppFull(pool, iss, &turnModel{}, plan.DefaultConfig(), "", nil)
	events := &fakeEventPublisher{err: errors.New("inngest event api unreachable")}
	h.events = events

	userID, cookie := autoApplyTailorUser(t, pool, iss, "publishfail@example.test")
	job := insertAutoApplyJob(t, pool, "publish-fail")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "declined"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a publish failure must not surface as a request failure", resp.StatusCode)
	}

	var decision string
	if err := pool.QueryRow(context.Background(),
		"SELECT review_decision FROM auto_apply_queue WHERE id = $1", queueID).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if decision != "declined" {
		t.Errorf("review_decision = %q, want \"declined\" — the decision must still be recorded", decision)
	}
}
