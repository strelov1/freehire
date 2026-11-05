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
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret)

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
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret)

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
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret)

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
	app, _ := newAutoApplyTailorAppWithOrchestratorSecret(pool, iss, &turnModel{}, testOrchestratorSecret)

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
