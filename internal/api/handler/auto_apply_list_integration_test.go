//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestGetAutoApplyList(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	issuer := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, issuer, &turnModel{})
	owner, cookie := autoApplyTailorUser(t, pool, issuer, "list-owner@example.test")
	other, _ := autoApplyTailorUser(t, pool, issuer, "list-other@example.test")

	read := func(cookieValue string, wantCode int) []autoApplyListItem {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me/auto-apply", nil)
		if cookieValue != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookieValue})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != wantCode {
			t.Fatalf("status = %d, want %d", resp.StatusCode, wantCode)
		}
		if wantCode != http.StatusOK {
			return nil
		}
		var body struct {
			Data struct {
				Attempts []autoApplyListItem `json:"attempts"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Data.Attempts == nil {
			t.Fatal("attempts must be an array, not null")
		}
		return body.Data.Attempts
	}
	read("", http.StatusUnauthorized)
	if got := read(cookie, http.StatusOK); len(got) != 0 {
		t.Fatalf("empty list = %+v", got)
	}
	ownJob := insertAutoApplyJob(t, pool, "list-owned")
	ownQueue := insertAutoApplyQueueRow(t, pool, owner, ownJob)
	otherJob := insertAutoApplyJob(t, pool, "list-foreign")
	insertAutoApplyQueueRow(t, pool, other, otherJob)
	// Internal diagnostics must not be exposed by the listing.
	if _, err := pool.Exec(context.Background(), "UPDATE auto_apply_queue SET blocked_at=now(), last_error='private diagnostic' WHERE id=$1", ownQueue); err != nil {
		t.Fatal(err)
	}
	submittedJob := insertAutoApplyJob(t, pool, "list-submitted")
	staleJob := insertAutoApplyJob(t, pool, "list-old-submitted")
	if _, err := pool.Exec(context.Background(), `INSERT INTO auto_apply_events(user_id,job_id,queue_id,kind,created_at) VALUES ($1,$2,999,'submitted',now()), ($1,$3,998,'submitted',now()-interval '31 days')`, owner, submittedJob, staleJob); err != nil {
		t.Fatal(err)
	}
	got := read(cookie, http.StatusOK)
	if len(got) != 2 {
		t.Fatalf("attempts = %+v, want own live attempt and recent submission", got)
	}
	if got[0].JobSlug != "list-submitted" || got[0].Status != "submitted" {
		t.Errorf("newest attempt = %+v", got[0])
	}
	if got[1].JobSlug != "list-owned" || got[1].Status != "blocked" {
		t.Errorf("live attempt = %+v", got[1])
	}
}

func TestGetAutoApplyListIncludesAttemptsBeyondMCPWindow(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	issuer := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAutoApplyTailorApp(pool, issuer, &turnModel{})
	user, cookie := autoApplyTailorUser(t, pool, issuer, "complete-list@example.test")
	for i := range 205 {
		job := insertAutoApplyJob(t, pool, fmt.Sprintf("list-window-%d", i))
		insertAutoApplyQueueRow(t, pool, user, job)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me/auto-apply", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Data autoApplyListResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || len(body.Data.Attempts) != 205 {
		t.Fatalf("HTTP status=%d attempts=%d, want 200 and all 205", resp.StatusCode, len(body.Data.Attempts))
	}
	bounded := newMCPToolCaller(t, h, user).ok("auto_apply_list", map[string]any{})
	if got := len(bounded["attempts"].([]any)); got != 200 {
		t.Fatalf("MCP attempts=%d, want bounded 200", got)
	}
}
