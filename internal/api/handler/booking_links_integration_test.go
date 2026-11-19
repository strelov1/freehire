//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

type paidUsers map[int64]bool

func (p paidUsers) RequirePaid(_ context.Context, userID int64) (plan.Tier, error) {
	if !p[userID] {
		return "", plan.ErrPaymentRequired
	}
	return "", nil
}

// The whole booking-link path through the contract: turning it on (paid owner only), the
// anonymous slot list, a booking, and the refusals a recruiter can meet.
func TestBookingLinkContractFlow(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	seed := func(email string) (user, app int64) {
		t.Helper()
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&user); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO applications (user_id, company_slug, role_title, applied_at) VALUES ($1, 'acme', 'Engineer', now()) RETURNING id`,
			user).Scan(&app); err != nil {
			t.Fatal(err)
		}
		return user, app
	}
	owner, ownerApp := seed("booking-owner@example.test")
	free, freeApp := seed("booking-free@example.test")

	h := newBookingLinkHandlers(bookinglink.New(
		bookinglink.NewQueriesRepository(db.New(pool), pool), paidUsers{owner: true}, []byte("k"),
	), "https://freehire.test/")
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookies := map[int64]string{}
	for _, id := range []int64{owner, free} {
		cookies[id], _ = iss.Issue(id, testTokenVersion)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)},
		newContract(contractHandlers{bookingLinkHandlers: h}))

	call := func(user int64, method, path, body string, header ...string) (int, map[string]any) {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = bytes.NewReader([]byte(body))
		}
		r := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, rd)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		if user != 0 {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookies[user], Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		}
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
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
	idPath := func(id int64) string { return "/me/applications/" + strconv.FormatInt(id, 10) + "/booking-link" }
	tz := `{"timezone":"Europe/Berlin"}`

	st, body := call(free, "POST", idPath(freeApp), tz)
	expect(st, 402, "free account", body)
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM booking_links`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("a refused enable wrote %d rows (%v)", rows, err)
	}
	st, body = call(owner, "POST", idPath(freeApp), tz)
	expect(st, 404, "someone else's application", body)
	st, body = call(0, "POST", idPath(ownerApp), tz)
	expect(st, 401, "anonymous enable", body)

	st, body = call(owner, "POST", idPath(ownerApp), tz)
	expect(st, 201, "enable", body)
	url, _ := body["data"].(map[string]any)["url"].(string)
	token, ok := strings.CutPrefix(url, "https://freehire.test/booking/")
	if !ok || len(token) < 22 {
		t.Fatalf("url = %q", url)
	}
	st, body = call(owner, "POST", idPath(ownerApp), tz)
	expect(st, 201, "enable again", body)
	if again := body["data"].(map[string]any)["url"]; again != url {
		t.Fatalf("second enable url = %v, want %v", again, url)
	}

	st, body = call(0, "GET", "/booking/not-a-token/slots", "")
	expect(st, 404, "bad token", body)
	st, body = call(0, "GET", "/booking/"+token+"/slots?tz=Europe/Berlin", "")
	expect(st, 200, "slots", body)
	if body["meta"].(map[string]any)["timezone"] != "Europe/Berlin" {
		t.Fatalf("meta = %v", body["meta"])
	}
	list, _ := body["data"].([]any)
	if len(list) == 0 {
		t.Fatal("no slots offered")
	}
	first := list[0].(map[string]any)["starts_at"].(string)
	for _, forbidden := range []string{"booking-owner@example.test", `"application_id"`} {
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("the public slot list leaks %s", forbidden)
		}
	}

	book := func(start, name, email string, header ...string) (int, map[string]any) {
		return call(0, "POST", "/booking/"+token+"/bookings",
			`{"starts_at":"`+start+`","name":"`+name+`","email":"`+email+`"}`, header...)
	}
	st, body = book(first, "", "r@acme.com")
	expect(st, 400, "empty name", body)
	st, body = book(first, "Rec", "nope")
	expect(st, 400, "bad email", body)
	st, body = book(first, "Rec", "r@acme.com", "Content-Encoding", "gzip")
	expect(st, 415, "encoded body", body)
	at, _ := time.Parse(time.RFC3339, first)
	st, body = book(at.Add(7*time.Minute).Format(time.RFC3339), "Rec", "r@acme.com")
	expect(st, 409, "off-grid start", body)

	st, body = book(first, "Rec", "r@acme.com")
	expect(st, 201, "book", body)
	st, body = book(first, "Other", "o@acme.com")
	expect(st, 409, "double book", body)
	if body["code"] != "slot_unavailable" {
		t.Fatalf("double book code = %v", body["code"])
	}
}
