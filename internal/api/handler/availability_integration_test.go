//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/availability"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The shared availability routes: weekly hours open to everyone, booking settings paid only,
// bad input 422 with nothing written, and a rule deletable by its owner alone.
func TestAvailabilityContract(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	seed := func(email string) (id int64) {
		t.Helper()
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	paid, free := seed("avail-paid@example.test"), seed("avail-free@example.test")

	h := &availabilityHandlers{store: availability.New(db.New(pool)), plans: paidUsers{paid: true}}
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{key: auth.RequireAuth(iss, testVersions)},
		newContract(contractHandlers{availabilityHandlers: h}))

	call := func(user int64, method, path, body string) (int, map[string]any) {
		t.Helper()
		r := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, bytes.NewReader([]byte(body)))
		r.Header.Set("Content-Type", "application/json")
		tok, _ := iss.Issue(user, testTokenVersion)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		resp, err := app.Test(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}
	expect := func(got, want int, what string, body map[string]any) {
		t.Helper()
		if got != want {
			t.Fatalf("%s: status = %d, want %d, body = %v", what, got, want, body)
		}
	}

	st, body := call(free, "GET", "/me/availability", "")
	expect(st, 200, "empty read", body)
	if d := body["data"].(map[string]any); d["saved"] != false || d["timezone"] != nil {
		t.Fatalf("a fresh user must read as unsaved: %v", d)
	}

	st, body = call(free, "PUT", "/me/availability/weekly", `{"rules":[{"weekday":1,"start":"09:00","end":"12:00"}]}`)
	expect(st, 204, "weekly write, free tier", body)
	st, body = call(free, "POST", "/me/availability/overrides", `{"date":"2030-01-07","start":"10:00","end":"10:00"}`)
	expect(st, 201, "override", body)
	ruleID := int64(body["data"].(map[string]any)["id"].(float64))
	st, body = call(free, "PUT", "/me/availability/weekly", `{"rules":[{"weekday":9,"start":"09:00","end":"12:00"}]}`)
	expect(st, 422, "bad weekday", body)

	st, body = call(paid, "DELETE", "/me/availability/"+itoa(ruleID), "")
	expect(st, 404, "someone else's rule", body)
	st, body = call(free, "DELETE", "/me/availability/"+itoa(ruleID), "")
	expect(st, 204, "own rule", body)

	good := `{"timezone":"Europe/Berlin","duration_min":60,"notice_hours":48}`
	st, body = call(free, "PUT", "/me/availability/booking", good)
	expect(st, 402, "free tier booking settings", body)
	for _, bad := range []string{
		`{"timezone":"Europe/Berlin","duration_min":20,"notice_hours":48}`,
		`{"timezone":"Europe/Berlin","duration_min":30,"notice_hours":169}`,
		`{"timezone":"Europe/Berlin","duration_min":30,"notice_hours":-1}`,
		`{"timezone":"Local","duration_min":30,"notice_hours":1}`,
	} {
		st, body = call(paid, "PUT", "/me/availability/booking", bad)
		expect(st, 422, bad, body)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_availability_settings`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("refused writes left %d settings rows (%v)", rows, err)
	}
	st, body = call(paid, "PUT", "/me/availability/booking", good)
	expect(st, 200, "paid booking settings", body)
	st, body = call(paid, "GET", "/me/availability", "")
	expect(st, 200, "read back", body)
	d := body["data"].(map[string]any)
	b := d["booking"].(map[string]any)
	if d["saved"] != true || d["timezone"] != "Europe/Berlin" || b["duration_min"] != float64(60) || b["notice_hours"] != float64(48) {
		t.Fatalf("read back %v", d)
	}
}
