//go:build integration

// Integration tests for the auto-apply orchestrator's shared-secret auth gate
// (openspec/changes/auto-apply-inngest-orchestration): a valid secret resolves the
// entry's owner from the record itself rather than from any authenticated identity, an
// absent/invalid credential is still refused, a human credential is unaffected by the
// secret's own existence, and system-caller requests share one process-wide rate-limit
// budget while human requests do not.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

const testOrchestratorSecret = "orchestrator-secret-value"

func TestPostAutoApplyTailor_OrchestratorSecretResolvesEntryOwner(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret, nil)

	userID, _ := autoApplyTailorUser(t, pool, iss, "orch-owner@example.test")
	job := insertAutoApplyJob(t, pool, "orch-reviewed")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	if _, err := pool.Exec(context.Background(),
		`UPDATE auto_apply_queue SET review_decision = 'declined', reviewed_at = now(), blocked_at = now() WHERE id = $1`,
		queueID); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+testOrchestratorSecret)
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 — the secret should resolve THIS entry's real owner and reach the already-reviewed check", resp.StatusCode)
	}
}

func TestPostAutoApplyTailor_NoCredentialsIsUnauthorizedEvenWithSecretConfigured(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret, nil)

	userID, _ := autoApplyTailorUser(t, pool, iss, "nocred@example.test")
	job := insertAutoApplyJob(t, pool, "nocred")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", nil)
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPostAutoApplyTailor_WrongSecretIsUnauthorized(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret, nil)

	userID, _ := autoApplyTailorUser(t, pool, iss, "wrongsecret@example.test")
	job := insertAutoApplyJob(t, pool, "wrongsecret")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer not-the-real-secret")
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// A human credential must resolve exactly as it always did — via ownership — even when an
// orchestrator secret is configured: the secret is a fallback the gate tries FIRST only
// because it is a cheap string compare, never a replacement for the ownership check.
func TestPostAutoApplyTailor_HumanCredentialStillOwnershipScopedWithSecretConfigured(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret, nil)

	owner, _ := autoApplyTailorUser(t, pool, iss, "human-owner@example.test")
	_, otherCookie := autoApplyTailorUser(t, pool, iss, "human-other@example.test")
	job := insertAutoApplyJob(t, pool, "human-foreign")
	queueID := insertAutoApplyQueueRow(t, pool, owner, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", otherCookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a foreign queue entry — the secret's presence in config must not weaken ownership for a cookie caller", resp.StatusCode)
	}
}

// The process-wide rate limit applies ONLY to system-caller (secret-authenticated)
// requests: oneShotThrottler admits the first use of a key and refuses every later one, so
// a second secret-authenticated call in the same test must trip it.
func TestPostAutoApplyTailor_OrchestratorSecretIsRateLimitedProcessWide(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	th := newOneShotThrottler()
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret, th)

	userID, _ := autoApplyTailorUser(t, pool, iss, "limited@example.test")
	job := insertAutoApplyJob(t, pool, "limited")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	if _, err := pool.Exec(context.Background(),
		`UPDATE auto_apply_queue SET review_decision = 'declined', reviewed_at = now(), blocked_at = now() WHERE id = $1`,
		queueID); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}

	doSecretRequest := func() int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+testOrchestratorSecret)
		resp, err := app.Test(req, 10_000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := doSecretRequest(); got != fiber.StatusConflict {
		t.Fatalf("first request status = %d, want 409 (within budget, reaches the already-reviewed check)", got)
	}
	if got := doSecretRequest(); got != fiber.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429 — the process-wide budget is spent", got)
	}
}

// A human-authenticated request never touches the orchestrator's own rate limit — an
// exhausted secret budget must not leak into the ownership-scoped path.
func TestPostAutoApplyTailor_HumanCredentialUnaffectedByExhaustedOrchestratorLimiter(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	th := newOneShotThrottler()
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret, th)

	// Spend the one-shot orchestrator budget with an unrelated entry.
	spentOwner, _ := autoApplyTailorUser(t, pool, iss, "spent-owner@example.test")
	insertBaseCV(t, pool, spentOwner)
	spentJob := insertAutoApplyJob(t, pool, "spent-job")
	spentQueueID := insertAutoApplyQueueRow(t, pool, spentOwner, spentJob)
	spendReq := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/auto-apply/"+strconv.FormatInt(spentQueueID, 10)+"/tailor", nil)
	spendReq.Header.Set(fiber.HeaderAuthorization, "Bearer "+testOrchestratorSecret)
	spendResp, err := app.Test(spendReq, 10_000)
	if err != nil {
		t.Fatalf("spend request: %v", err)
	}
	spendResp.Body.Close()

	// A cookie-authenticated request for its OWNER's own entry must still be refused only
	// for the reasons ownership-scoped auth already enforces (foreign entry → 404), never
	// for a rate limit that was never applied to it.
	owner, _ := autoApplyTailorUser(t, pool, iss, "human-owner-2@example.test")
	_, otherCookie := autoApplyTailorUser(t, pool, iss, "human-other-2@example.test")
	job := insertAutoApplyJob(t, pool, "human-foreign-2")
	queueID := insertAutoApplyQueueRow(t, pool, owner, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", otherCookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode == fiber.StatusTooManyRequests {
		t.Fatalf("status = 429 — a human caller must never share the orchestrator's own rate-limit budget")
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404 (ownership, unaffected by the spent orchestrator budget)", resp.StatusCode)
	}
}
