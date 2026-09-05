//go:build integration

// Integration tests for the job-lists HTTP flow against a real Postgres: create,
// add/remove jobs, share/unshare, the public read by slug, owner-scoping (a
// non-owner's request 404s), and idempotent add/remove. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestJobListsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var ownerID, otherID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('joblists-owner@example.test', true) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('joblists-other@example.test', true) RETURNING id`).Scan(&otherID); err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	const jobSlug = "backend-engineer-acme-t35nijto"
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('test', 'joblist-1', 'http://example.test/1', 'Backend Engineer', 'Acme', $1)`, jobSlug); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	ownerCookie, _ := iss.Issue(ownerID, testTokenVersion)
	otherCookie, _ := iss.Issue(otherID, testTokenVersion)
	queries := db.New(pool)
	h := newJobListHandlers(queries)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	requireAuth := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/lists/:slug", h.GetPublicList)
	app.Get("/api/v1/me/lists", requireAuth, h.ListJobLists)
	app.Get("/api/v1/me/lists/membership", requireAuth, h.ListJobListMembership)
	app.Post("/api/v1/me/lists", requireAuth, h.CreateJobList)
	app.Patch("/api/v1/me/lists/:id", requireAuth, h.UpdateJobList)
	app.Delete("/api/v1/me/lists/:id", requireAuth, h.DeleteJobList)
	app.Post("/api/v1/me/lists/:id/jobs", requireAuth, h.AddJobToList)
	app.Delete("/api/v1/me/lists/:id/jobs/:job_slug", requireAuth, h.RemoveJobFromList)
	app.Post("/api/v1/me/lists/:id/share", requireAuth, h.ShareJobList)
	app.Delete("/api/v1/me/lists/:id/share", requireAuth, h.UnshareJobList)

	cookieReq := func(method, path, cookie string, body []byte) *http.Request {
		var r *http.Request
		if body != nil {
			r = httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequestWithContext(ctx, method, path, nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return r
	}

	do := func(t *testing.T, req *http.Request, wantStatus int) map[string]any {
		t.Helper()
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != wantStatus {
			t.Fatalf("status = %d, want %d (body %s)", resp.StatusCode, wantStatus, body)
		}
		if len(body) == 0 {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("decode body %s: %v", body, err)
		}
		return out
	}

	// 401 without a session cookie.
	do(t, cookieReq(fiber.MethodGet, "/api/v1/me/lists", "", nil), fiber.StatusUnauthorized)

	// Create a list.
	createBody, _ := json.Marshal(map[string]string{"name": "Backend jobs", "description": "shortlist"})
	created := do(t, cookieReq(fiber.MethodPost, "/api/v1/me/lists", ownerCookie, createBody), fiber.StatusCreated)
	data := created["data"].(map[string]any)
	if data["name"] != "Backend jobs" || data["description"] != "shortlist" {
		t.Fatalf("created list = %+v", data)
	}
	if data["public_slug"] != "" {
		t.Fatalf("new list should be private, got public_slug %v", data["public_slug"])
	}
	listID := int64(data["id"].(float64))

	// A duplicate name is a 409.
	do(t, cookieReq(fiber.MethodPost, "/api/v1/me/lists", ownerCookie, createBody), fiber.StatusConflict)

	// A non-owner cannot rename it.
	renameBody, _ := json.Marshal(map[string]string{"name": "Hijacked"})
	do(t, cookieReq(fiber.MethodPatch, fmt.Sprintf("/api/v1/me/lists/%d", listID), otherCookie, renameBody), fiber.StatusNotFound)

	// Add a job to the list.
	addBody, _ := json.Marshal(map[string]string{"job_slug": jobSlug})
	do(t, cookieReq(fiber.MethodPost, fmt.Sprintf("/api/v1/me/lists/%d/jobs", listID), ownerCookie, addBody), fiber.StatusNoContent)
	// Adding it again is idempotent.
	do(t, cookieReq(fiber.MethodPost, fmt.Sprintf("/api/v1/me/lists/%d/jobs", listID), ownerCookie, addBody), fiber.StatusNoContent)

	// An unknown job slug is a 404.
	unknownJobBody, _ := json.Marshal(map[string]string{"job_slug": "does-not-exist"})
	do(t, cookieReq(fiber.MethodPost, fmt.Sprintf("/api/v1/me/lists/%d/jobs", listID), ownerCookie, unknownJobBody), fiber.StatusNotFound)

	// Listing shows the job count.
	listed := do(t, cookieReq(fiber.MethodGet, "/api/v1/me/lists", ownerCookie, nil), fiber.StatusOK)
	rows := listed["data"].([]any)
	if len(rows) != 1 {
		t.Fatalf("list count = %d, want 1", len(rows))
	}
	if rows[0].(map[string]any)["job_count"] != float64(1) {
		t.Fatalf("job_count = %v, want 1", rows[0].(map[string]any)["job_count"])
	}

	// Membership reports this list as containing the job.
	membership := do(t, cookieReq(fiber.MethodGet, "/api/v1/me/lists/membership?job_slug="+jobSlug, ownerCookie, nil), fiber.StatusOK)
	memberRows := membership["data"].([]any)
	if len(memberRows) != 1 || memberRows[0].(map[string]any)["in_list"] != true {
		t.Fatalf("membership = %+v, want one list with in_list=true", memberRows)
	}

	// Renaming a non-empty list still reports its real job_count (not the zero
	// value fromRow would carry if Update's row shape dropped the job-count
	// subquery ListJobLists computes).
	renameBody2, _ := json.Marshal(map[string]string{"name": "Backend jobs — renamed"})
	renamed := do(t, cookieReq(fiber.MethodPatch, fmt.Sprintf("/api/v1/me/lists/%d", listID), ownerCookie, renameBody2), fiber.StatusOK)
	if renamed["data"].(map[string]any)["job_count"] != float64(1) {
		t.Fatalf("job_count after renaming a 1-job list = %v, want 1", renamed["data"].(map[string]any)["job_count"])
	}

	// Share the list and read it back publicly. Its job_count carries through
	// share the same way rename's does.
	shared := do(t, cookieReq(fiber.MethodPost, fmt.Sprintf("/api/v1/me/lists/%d/share", listID), ownerCookie, nil), fiber.StatusOK)
	slug := shared["data"].(map[string]any)["public_slug"].(string)
	if slug == "" || !strings.HasPrefix(slug, "backend-jobs-") {
		t.Fatalf("minted slug = %q, want readable prefix", slug)
	}
	if shared["data"].(map[string]any)["job_count"] != float64(1) {
		t.Fatalf("job_count on the share response = %v, want 1", shared["data"].(map[string]any)["job_count"])
	}

	public := do(t, cookieReq(fiber.MethodGet, "/api/v1/lists/"+slug, "", nil), fiber.StatusOK)
	pdata := public["data"].(map[string]any)
	if pdata["name"] != "Backend jobs — renamed" || pdata["description"] != "shortlist" {
		t.Fatalf("public list = %+v", pdata)
	}
	if _, hasOwner := pdata["user_id"]; hasOwner {
		t.Fatal("public list response must not expose user_id")
	}
	jobs := pdata["jobs"].([]any)
	if len(jobs) != 1 || jobs[0].(map[string]any)["title"] != "Backend Engineer" {
		t.Fatalf("public list jobs = %+v", jobs)
	}

	// Remove the job — verify it is actually gone (job_count back to 0, membership
	// flips to false), not just that the status code says so — then removing it
	// again is idempotent.
	do(t, cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/lists/%d/jobs/%s", listID, jobSlug), ownerCookie, nil), fiber.StatusNoContent)
	afterRemove := do(t, cookieReq(fiber.MethodGet, "/api/v1/me/lists", ownerCookie, nil), fiber.StatusOK)
	if got := afterRemove["data"].([]any)[0].(map[string]any)["job_count"]; got != float64(0) {
		t.Fatalf("job_count after removing the only job = %v, want 0", got)
	}
	afterRemoveMembership := do(t, cookieReq(fiber.MethodGet, "/api/v1/me/lists/membership?job_slug="+jobSlug, ownerCookie, nil), fiber.StatusOK)
	if got := afterRemoveMembership["data"].([]any)[0].(map[string]any)["in_list"]; got != false {
		t.Fatalf("membership in_list after removal = %v, want false", got)
	}
	do(t, cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/lists/%d/jobs/%s", listID, jobSlug), ownerCookie, nil), fiber.StatusNoContent)

	// Unshare, then the public link 404s.
	do(t, cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/lists/%d/share", listID), ownerCookie, nil), fiber.StatusNoContent)
	do(t, cookieReq(fiber.MethodGet, "/api/v1/lists/"+slug, "", nil), fiber.StatusNotFound)

	// A non-owner cannot delete it; the owner can.
	do(t, cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/lists/%d", listID), otherCookie, nil), fiber.StatusNotFound)
	do(t, cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/lists/%d", listID), ownerCookie, nil), fiber.StatusNoContent)
}
