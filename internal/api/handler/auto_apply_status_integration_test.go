//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/identity/auth"
)

func decodeAutoApplyStatus(t *testing.T, resp *http.Response) *string {
	t.Helper()
	defer resp.Body.Close()
	var out struct {
		Data struct {
			AutoApplyStatus *string `json:"auto_apply_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Data.AutoApplyStatus
}

func TestGetJob_AutoApplyStatusOverlay(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "status@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "status-job")

	get := func(slug, cookie string) *http.Response { return getJobRequest(t, app, slug, cookie) }

	beforeResp := get("status-job", cookie)
	defer beforeResp.Body.Close()
	if got := decodeAutoApplyStatus(t, beforeResp); got != nil {
		t.Fatalf("status before any attempt = %v, want nil", got)
	}
	anonResp := get("status-job", "")
	defer anonResp.Body.Close()
	if got := decodeAutoApplyStatus(t, anonResp); got != nil {
		t.Fatalf("status for an anonymous caller = %v, want nil", got)
	}

	enqueueResp := enqueueRequest(t, app, "status-job", cookie)
	defer enqueueResp.Body.Close()
	if enqueueResp.StatusCode != fiber.StatusOK {
		t.Fatalf("enqueue status = %d, want 200", enqueueResp.StatusCode)
	}
	queuedResp := get("status-job", cookie)
	defer queuedResp.Body.Close()
	if got := decodeAutoApplyStatus(t, queuedResp); got == nil || *got != "queued" {
		t.Fatalf("status after enqueue = %v, want \"queued\"", got)
	}

	var queueID int64
	if err := pool.QueryRow(context.Background(),
		"SELECT id FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&queueID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE auto_apply_queue SET review_decision = 'declined', reviewed_at = now(), blocked_at = now(), last_error = 'x' WHERE id = $1`,
		queueID); err != nil {
		t.Fatalf("mark declined: %v", err)
	}
	declinedResp := get("status-job", cookie)
	defer declinedResp.Body.Close()
	if got := decodeAutoApplyStatus(t, declinedResp); got == nil || *got != "declined" {
		t.Fatalf("status after decline = %v, want \"declined\"", got)
	}
}

// TestGetJob_AutoApplyStatusOverlay_NonGreenhouseSource guards the overlay for every source
// auto-apply can now queue against, not only Greenhouse — the enqueue gate
// (autoApplyEnqueueSources) and this read's own gate must name the same set, or a
// successfully queued Ashby/Workable/Lever/Recruitee attempt would read back as no attempt
// at all and the button would look idle forever.
func TestGetJob_AutoApplyStatusOverlay_NonGreenhouseSource(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "ashby-status@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "ashby", "ashby-status-job")

	enqueueResp := enqueueRequest(t, app, "ashby-status-job", cookie)
	defer enqueueResp.Body.Close()
	if enqueueResp.StatusCode != fiber.StatusOK {
		t.Fatalf("enqueue status = %d, want 200", enqueueResp.StatusCode)
	}

	resp := getJobRequest(t, app, "ashby-status-job", cookie)
	defer resp.Body.Close()
	if got := decodeAutoApplyStatus(t, resp); got == nil || *got != "queued" {
		t.Fatalf("status after enqueue for an Ashby posting = %v, want \"queued\"", got)
	}
}

// TestGetJob_AutoApplyStatusOverlay_FailedEntry guards a gap a code review found:
// GetAutoApplyQueueEntryForJob used to select only id and review_decision, so a
// dead-lettered (failed_at) or parked (blocked_at) submission — both of which leave
// review_decision at 'approved', since only an approved entry is ever claimed — was
// indistinguishable from a healthy entry still in flight. The candidate would see
// "queued" forever with no way to learn the attempt died.
func TestGetJob_AutoApplyStatusOverlay_FailedEntry(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "failed@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	jobID := seedEnqueueJob(t, pool, "greenhouse", "failed-job")

	var queueID int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id, review_decision, reviewed_at, failed_at, last_error, attempts)
		 VALUES ($1, $2, 'approved', now(), now(), 'boom', 3) RETURNING id`,
		userID, jobID).Scan(&queueID); err != nil {
		t.Fatalf("seed dead-lettered entry: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/jobs/failed-job", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie}) //nolint:gosec // a request cookie: the Set-Cookie attributes do not apply
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := decodeAutoApplyStatus(t, resp); got == nil || *got != autoApplyStatusFailed {
		t.Fatalf("status for a dead-lettered entry = %v, want %q", got, autoApplyStatusFailed)
	}

	repeat := enqueueRequest(t, app, "failed-job", cookie)
	defer repeat.Body.Close()
	if repeat.StatusCode != fiber.StatusConflict {
		t.Fatalf("repeat enqueue status = %d, want 409 — a dead-lettered attempt must not report a false queued success", repeat.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auto_apply_queue rows = %d, want 1 (no second row)", count)
	}
}
