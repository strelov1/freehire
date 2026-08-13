//go:build integration

// Integration tests for the notification-center HTTP flow against a real
// Postgres: listing paginates and reports total/unread_count, marking one
// notification read is idempotent and owner-scoped, and mark-all-read reports
// how many it touched. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

func notificationsIntegrationApp(queries *db.Queries, iss *auth.Issuer) *fiber.App {
	h := &authHandlers{queries: queries}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/notifications", auth.RequireAuth(iss, testVersions), h.GetNotifications)
	app.Get("/api/v1/me/notifications/:id", auth.RequireAuth(iss, testVersions), h.GetNotification)
	app.Post("/api/v1/me/notifications/:id/read", auth.RequireAuth(iss, testVersions), h.MarkNotificationRead)
	app.Post("/api/v1/me/notifications/read-all", auth.RequireAuth(iss, testVersions), h.MarkAllNotificationsRead)
	return app
}

func seedNotificationsUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedNotificationRows inserts n unread notifications for the user, oldest
// first insert order (so the newest-first list query returns them reversed).
func seedNotificationRows(t *testing.T, pool *pgxpool.Pool, userID int64, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := 0; i < n; i++ {
		var id int64
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO user_notifications (user_id, kind, title, body, public_slug)
			 VALUES ($1, 'reminder', 'Reminder', 'body', $2) RETURNING id`,
			userID, fmt.Sprintf("job-%d", i)).Scan(&id); err != nil {
			t.Fatalf("seed notification %d: %v", i, err)
		}
		ids[i] = id
	}
	return ids
}

func cookieGetReq(iss *auth.Issuer, userID int64, path string) *http.Request {
	cookie, _ := iss.Issue(userID, testTokenVersion)
	r := httptest.NewRequest(fiber.MethodGet, path, nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	return r
}

func cookiePostReq(iss *auth.Issuer, userID int64, path string) *http.Request {
	cookie, _ := iss.Issue(userID, testTokenVersion)
	r := httptest.NewRequest(fiber.MethodPost, path, nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	return r
}

func TestNotificationsEndToEnd_ListAndCounts(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	userID := seedNotificationsUser(t, pool, "notif-list@example.test")
	ids := seedNotificationRows(t, pool, userID, 3)
	// Mark the oldest (first inserted) read directly, so unread_count should be 2.
	if _, err := pool.Exec(context.Background(),
		`UPDATE user_notifications SET read_at = now() WHERE id = $1`, ids[0]); err != nil {
		t.Fatalf("mark seed read: %v", err)
	}

	app := notificationsIntegrationApp(queries, iss)
	resp, err := app.Test(cookieGetReq(iss, userID, "/api/v1/me/notifications"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("list status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Data []notificationResponse `json:"data"`
		Meta struct {
			Total       int64 `json:"total"`
			UnreadCount int64 `json:"unread_count"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Data) != 3 {
		t.Errorf("data len = %d, want 3", len(out.Data))
	}
	if out.Meta.Total != 3 {
		t.Errorf("meta.total = %d, want 3", out.Meta.Total)
	}
	if out.Meta.UnreadCount != 2 {
		t.Errorf("meta.unread_count = %d, want 2", out.Meta.UnreadCount)
	}
	// Newest-first: the last-seeded row (ids[2]) should be first in the page.
	if out.Data[0].ID != ids[2] {
		t.Errorf("data[0].ID = %d, want newest (%d)", out.Data[0].ID, ids[2])
	}
}

func TestNotificationsEndToEnd_MarkRead(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	aliceID := seedNotificationsUser(t, pool, "notif-alice@example.test")
	bobID := seedNotificationsUser(t, pool, "notif-bob@example.test")
	ids := seedNotificationRows(t, pool, aliceID, 1)

	app := notificationsIntegrationApp(queries, iss)

	t.Run("owner marks it read", func(t *testing.T) {
		resp, err := app.Test(cookiePostReq(iss, aliceID, fmt.Sprintf("/api/v1/me/notifications/%d/read", ids[0])))
		if err != nil {
			t.Fatalf("mark read: %v", err)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}
		var readAt any
		if err := pool.QueryRow(context.Background(),
			`SELECT read_at FROM user_notifications WHERE id = $1`, ids[0]).Scan(&readAt); err != nil {
			t.Fatalf("query: %v", err)
		}
		if readAt == nil {
			t.Error("read_at not set after marking read")
		}
	})

	t.Run("repeating the call is a no-op, still 204", func(t *testing.T) {
		var before time.Time
		if err := pool.QueryRow(context.Background(),
			`SELECT read_at FROM user_notifications WHERE id = $1`, ids[0]).Scan(&before); err != nil {
			t.Fatalf("query: %v", err)
		}
		resp, err := app.Test(cookiePostReq(iss, aliceID, fmt.Sprintf("/api/v1/me/notifications/%d/read", ids[0])))
		if err != nil {
			t.Fatalf("mark read again: %v", err)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}
		var after time.Time
		if err := pool.QueryRow(context.Background(),
			`SELECT read_at FROM user_notifications WHERE id = $1`, ids[0]).Scan(&after); err != nil {
			t.Fatalf("query: %v", err)
		}
		if !before.Equal(after) {
			t.Errorf("read_at changed on repeat call: %v -> %v, want unchanged", before, after)
		}
	})

	t.Run("another user's notification 404s", func(t *testing.T) {
		resp, err := app.Test(cookiePostReq(iss, bobID, fmt.Sprintf("/api/v1/me/notifications/%d/read", ids[0])))
		if err != nil {
			t.Fatalf("mark read as bob: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404 (bob does not own alice's notification)", resp.StatusCode)
		}
	})

	t.Run("nonexistent id 404s", func(t *testing.T) {
		resp, err := app.Test(cookiePostReq(iss, aliceID, "/api/v1/me/notifications/999999999/read"))
		if err != nil {
			t.Fatalf("mark read nonexistent: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}

func TestNotificationsEndToEnd_GetOne(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	aliceID := seedNotificationsUser(t, pool, "notif-get-alice@example.test")
	bobID := seedNotificationsUser(t, pool, "notif-get-bob@example.test")

	var digestID int64
	const jobsJSON = `[{"title":"Backend Engineer","company":"Acme","slug":"acme-backend-engineer"},{"title":"SRE","company":"Acme","slug":"acme-sre"}]`
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO user_notifications (user_id, kind, title, body, jobs)
		 VALUES ($1, 'subscription_digest', 'freehire', '2 new jobs for "Backend"', $2::jsonb)
		 RETURNING id`, aliceID, jobsJSON).Scan(&digestID); err != nil {
		t.Fatalf("seed digest notification: %v", err)
	}

	app := notificationsIntegrationApp(queries, iss)

	t.Run("owner reads it with the jobs snapshot", func(t *testing.T) {
		resp, err := app.Test(cookieGetReq(iss, aliceID, fmt.Sprintf("/api/v1/me/notifications/%d", digestID)))
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var out struct {
			Data notificationResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.ID != digestID {
			t.Errorf("id = %d, want %d", out.Data.ID, digestID)
		}
		var jobs []struct {
			Title   string `json:"title"`
			Company string `json:"company"`
			Slug    string `json:"slug"`
		}
		if err := json.Unmarshal(out.Data.Jobs, &jobs); err != nil {
			t.Fatalf("jobs did not unmarshal: %v (raw: %s)", err, out.Data.Jobs)
		}
		if len(jobs) != 2 || jobs[0].Slug != "acme-backend-engineer" || jobs[1].Slug != "acme-sre" {
			t.Errorf("jobs = %+v, want 2 entries in seeded order", jobs)
		}
	})

	t.Run("another user's notification 404s", func(t *testing.T) {
		resp, err := app.Test(cookieGetReq(iss, bobID, fmt.Sprintf("/api/v1/me/notifications/%d", digestID)))
		if err != nil {
			t.Fatalf("get as bob: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404 (bob does not own alice's notification)", resp.StatusCode)
		}
	})

	t.Run("nonexistent id 404s", func(t *testing.T) {
		resp, err := app.Test(cookieGetReq(iss, aliceID, "/api/v1/me/notifications/999999999"))
		if err != nil {
			t.Fatalf("get nonexistent: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}

func TestNotificationsEndToEnd_MarkAllRead(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	userID := seedNotificationsUser(t, pool, "notif-markall@example.test")
	seedNotificationRows(t, pool, userID, 3)

	app := notificationsIntegrationApp(queries, iss)
	resp, err := app.Test(cookiePostReq(iss, userID, "/api/v1/me/notifications/read-all"))
	if err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Data struct {
			Marked int64 `json:"marked"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Marked != 3 {
		t.Errorf("marked = %d, want 3", out.Data.Marked)
	}

	var unread int64
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM user_notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&unread); err != nil {
		t.Fatalf("count unread: %v", err)
	}
	if unread != 0 {
		t.Errorf("unread remaining = %d, want 0", unread)
	}
}
