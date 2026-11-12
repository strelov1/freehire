//go:build integration

// Integration tests for the mentorship routes served through the contract, against a real
// Postgres: the whole path from a mentor submitting a profile, a moderator approving it,
// the mentor opening hours, a seeker booking, to cancelling and the refusals on the way.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestMentorshipContractFlow(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	seedUser := func(email string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email,
		).Scan(&id); err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		return id
	}
	mentorID := seedUser("mentor-contract@example.test")
	seekerID := seedUser("seeker-contract@example.test")
	modID := seedUser("mod-contract@example.test")
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('contractco', 'Contract Co') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	h := newMentorshipHandlers(mentorship.New(
		mentorship.NewQueriesRepository(db.New(pool), pool),
		mentorship.Config{},
	), nil)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookies := map[int64]string{}
	for _, id := range []int64{mentorID, seekerID, modID} {
		cookies[id], _ = iss.Issue(id, testTokenVersion)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	pass := func(c *fiber.Ctx) error { return c.Next() }
	h.register(app.Group("/api/v1"),
		middleware{key: auth.RequireAuth(iss, testVersions), moderator: pass},
		newContract(contractHandlers{mentorshipHandlers: h}))

	// call sends one request as the user (0 = anonymous) and returns status and body.
	call := func(user int64, method, path, body string) (int, map[string]any) {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = bytes.NewReader([]byte(body))
		}
		r := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, rd)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if user != 0 {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookies[user]})
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
	data := func(body map[string]any) map[string]any {
		d, _ := body["data"].(map[string]any)
		return d
	}

	// Submit: 201 with the owner view (status and the configured link), then a second
	// submission is a conflict and an empty body a validation refusal.
	st, body := call(mentorID, "POST", "/me/mentorship/profile", `{
		"company_slug": "contractco", "slug": "contract-mentor", "name": "Contract Mentor",
		"headline": "Staff Engineer", "topics": ["career"], "languages": ["en"],
		"timezone": "UTC", "meeting_url": "https://meet.example.test/contract",
		"session_minutes": 60, "notice_minutes": 0, "horizon_days": 30, "seniority": "senior"
	}`)
	expect(st, 201, "submit", body)
	if d := data(body); d["status"] != "pending" || d["meeting_url"] != "https://meet.example.test/contract" {
		t.Errorf("submit answer = %v", d)
	}
	st, _ = call(mentorID, "POST", "/me/mentorship/profile", `{"slug":"contract-mentor"}`)
	if st < 400 {
		t.Errorf("second submit = %d, want a refusal", st)
	}
	st, body = call(seekerID, "POST", "/me/mentorship/profile", `{}`)
	expect(st, 422, "empty submit", body)

	// Pending: not public yet; the moderation queue lists it with the evidence flag.
	st, body = call(0, "GET", "/mentors/contract-mentor", "")
	expect(st, 404, "pending is not public", body)
	st, body = call(modID, "GET", "/mentorship/profiles", "")
	expect(st, 200, "queue", body)
	queue, _ := body["data"].([]any)
	if len(queue) != 1 {
		t.Fatalf("queue = %v", body)
	}
	row, _ := queue[0].(map[string]any)
	if _, ok := row["has_approved_referral_offer"].(bool); !ok || row["slug"] != "contract-mentor" {
		t.Errorf("queue row = %v", row)
	}
	profileID := strconv.Itoa(int(row["id"].(float64)))

	st, body = call(modID, "POST", "/mentorship/profiles/nope/decide", `{"status":"approved"}`)
	expect(st, 404, "decide bad id", body)
	st, _ = call(modID, "POST", "/mentorship/profiles/"+profileID+"/decide", `{"status":"maybe"}`)
	if st < 400 {
		t.Errorf("decide with an unknown status = %d, want a refusal", st)
	}
	st, body = call(modID, "POST", "/mentorship/profiles/"+profileID+"/decide", `{"status":"approved"}`)
	expect(st, 200, "decide", body)
	if d := data(body); d["status"] != "approved" || d["meeting_url"] != nil {
		t.Errorf("moderator view = %v (status approved, no meeting link)", d)
	}
	st, body = call(modID, "POST", "/mentorship/profiles/"+profileID+"/decide", `{"status":"approved"}`)
	expect(st, 409, "decide twice", body)

	// Public directory: found by filter, an unknown param reported, the card has no link.
	st, body = call(0, "GET", "/mentors?topic=career&seniority=senior&q=Contract&no_reviews=true&limit=5&bogus=1", "")
	expect(st, 200, "directory", body)
	if list, _ := body["data"].([]any); len(list) != 1 {
		t.Errorf("directory = %v", body)
	}
	if meta, _ := body["meta"].(map[string]any); meta["count"] != float64(1) || len(meta["ignored_params"].([]any)) != 1 {
		t.Errorf("directory meta = %v", body["meta"])
	}
	st, body = call(0, "GET", "/mentors/contract-mentor", "")
	expect(st, 200, "profile", body)
	if d := data(body); d["name"] != "Contract Mentor" || d["session_minutes"] != float64(60) {
		t.Errorf("public profile = %v", d)
	} else if _, leaked := d["meeting_url"]; leaked {
		t.Errorf("public profile leaks the meeting link: %v", d)
	}

	// Schedule: replace the week (whole day, every day), a dated rule is refused there,
	// an override needs a date, and an override can be added and removed.
	var rules string
	for day := 0; day < 7; day++ {
		if day > 0 {
			rules += ","
		}
		rules += `{"weekday":` + strconv.Itoa(day) + `,"start":"00:00","end":"24:00"}`
	}
	st, body = call(mentorID, "PUT", "/me/mentorship/availability/weekly", `{"rules":[`+rules+`]}`)
	expect(st, 204, "weekly", body)
	st, body = call(mentorID, "PUT", "/me/mentorship/availability/weekly", `{"rules":[{"date":"2030-01-01","start":"09:00","end":"10:00"}]}`)
	expect(st, 422, "dated rule in weekly", body)
	st, body = call(mentorID, "POST", "/me/mentorship/availability/overrides", `{"start":"09:00","end":"10:00"}`)
	expect(st, 422, "override without a date", body)
	st, body = call(mentorID, "POST", "/me/mentorship/availability/overrides", `{"date":"2030-01-01","start":"9","end":"10:00"}`)
	expect(st, 422, "override with a bad time", body)
	st, body = call(mentorID, "POST", "/me/mentorship/availability/overrides", `{"date":"2030-01-01","start":"10:00","end":"10:00"}`)
	expect(st, 201, "closure override", body)
	if d := data(body); d["closure"] != true || d["date"] != "2030-01-01" {
		t.Errorf("override = %v", d)
	}
	st, body = call(mentorID, "GET", "/me/mentorship/availability", "")
	expect(st, 200, "availability", body)
	rows, _ := body["data"].([]any)
	if len(rows) != 8 || body["meta"].(map[string]any)["count"] != float64(8) {
		t.Fatalf("availability = %v", body)
	}
	var dated string
	for _, r := range rows {
		if m := r.(map[string]any); m["date"] != nil {
			dated = strconv.Itoa(int(m["id"].(float64)))
		}
	}
	st, body = call(mentorID, "DELETE", "/me/mentorship/availability/"+dated, "")
	expect(st, 204, "delete rule", body)
	st, body = call(mentorID, "DELETE", "/me/mentorship/availability/x", "")
	expect(st, 404, "delete bad id", body)

	// Slots: the viewer's zone is echoed; a malformed bound is a 400.
	st, body = call(0, "GET", "/mentors/contract-mentor/slots?timezone=Europe/Berlin", "")
	expect(st, 200, "slots", body)
	slots, _ := body["data"].([]any)
	if len(slots) == 0 || body["meta"].(map[string]any)["timezone"] != "Europe/Berlin" {
		t.Fatalf("slots = %v", body["meta"])
	}
	st, body = call(0, "GET", "/mentors/contract-mentor/slots?from=yesterday", "")
	expect(st, 400, "slots bad from", body)
	st, body = call(0, "GET", "/mentors/nobody/slots", "")
	expect(st, 404, "slots unknown mentor", body)
	first := slots[len(slots)-1].(map[string]any)["starts_at"].(string)

	// Booking: signed-out 401, bad timestamp 422, own session 422, then 201 and a repeat 409.
	st, body = call(0, "POST", "/mentors/contract-mentor/bookings", `{"starts_at":"`+first+`"}`)
	expect(st, 401, "book signed out", body)
	st, body = call(seekerID, "POST", "/mentors/contract-mentor/bookings", `{"starts_at":"soon"}`)
	expect(st, 422, "book bad time", body)
	st, body = call(mentorID, "POST", "/mentors/contract-mentor/bookings", `{"starts_at":"`+first+`"}`)
	expect(st, 422, "book yourself", body)
	st, body = call(seekerID, "POST", "/mentors/contract-mentor/bookings", `{"starts_at":"`+first+`","note":"hi","timezone":"UTC"}`)
	expect(st, 201, "book", body)
	booking := data(body)
	if booking["status"] != "confirmed" || booking["note"] != "hi" || booking["meeting_url"] != "https://meet.example.test/contract" {
		t.Errorf("booking = %v", booking)
	}
	if _, has := booking["seeker_email"]; has {
		t.Errorf("seeker sees their own address back: %v", booking)
	}
	bookingID := booking["id"].(string)
	st, body = call(seekerID, "POST", "/mentors/contract-mentor/bookings", `{"starts_at":"`+first+`"}`)
	expect(st, 409, "book the same hour", body)

	// Sessions: each party sees it, the mentor with the seeker's address; a stranger and a
	// malformed id are a 404.
	st, body = call(seekerID, "GET", "/me/mentorship/sessions?limit=5", "")
	expect(st, 200, "my sessions", body)
	if d := data(body); len(d["upcoming"].([]any)) != 1 || len(d["past"].([]any)) != 0 {
		t.Errorf("my sessions = %v", d)
	}
	st, body = call(mentorID, "GET", "/me/mentorship/bookings", "")
	expect(st, 200, "mentor bookings", body)
	if up := data(body)["upcoming"].([]any); len(up) != 1 || up[0].(map[string]any)["seeker_email"] != "seeker-contract@example.test" {
		t.Errorf("mentor bookings = %v", body)
	}
	st, body = call(mentorID, "GET", "/me/mentorship/sessions/"+bookingID, "")
	expect(st, 200, "mentor reads session", body)
	if data(body)["seeker_email"] != "seeker-contract@example.test" {
		t.Errorf("mentor view lacks the seeker: %v", body)
	}
	st, body = call(seekerID, "GET", "/me/mentorship/sessions/"+bookingID, "")
	expect(st, 200, "seeker reads session", body)
	st, body = call(modID, "GET", "/me/mentorship/sessions/"+bookingID, "")
	expect(st, 404, "stranger reads session", body)
	st, body = call(seekerID, "GET", "/me/mentorship/sessions/not-a-uuid", "")
	expect(st, 404, "bad session id", body)
	if body["error"] != "session not found" {
		t.Errorf("bad id error = %v", body["error"])
	}

	// Review: a session in the future is not reviewable.
	st, body = call(seekerID, "PUT", "/me/mentorship/sessions/"+bookingID+"/review", `{"rating":5,"comment":"great"}`)
	expect(st, 409, "review before it happened", body)

	// Calendar: the booking is carved out as booked in the mentor's month.
	st, body = call(mentorID, "GET", "/me/mentorship/availability/calendar?month="+first[:7], "")
	expect(st, 200, "calendar", body)
	var booked bool
	for _, iv := range body["data"].([]any) {
		booked = booked || iv.(map[string]any)["status"] == "booked"
	}
	if !booked {
		t.Errorf("calendar has no booked interval: %v", body["meta"])
	}
	st, body = call(mentorID, "GET", "/me/mentorship/availability/calendar", "")
	expect(st, 200, "calendar default month", body)

	// Cancel: with a reason, then again (the cancelled one is gone: 404).
	st, body = call(seekerID, "POST", "/me/mentorship/sessions/"+bookingID+"/cancel", `{"reason":"clash"}`)
	expect(st, 200, "cancel", body)
	if data(body)["status"] != "cancelled" {
		t.Errorf("cancelled booking = %v", body)
	}
	st, body = call(seekerID, "POST", "/me/mentorship/sessions/"+bookingID+"/cancel", "")
	expect(st, 404, "cancel twice", body)
	st, body = call(seekerID, "GET", "/me/mentorship/sessions", "")
	expect(st, 200, "sessions after cancel", body)
	if d := data(body); len(d["upcoming"].([]any)) != 0 || len(d["past"].([]any)) != 1 {
		t.Errorf("a cancelled session must be in past: %v", d)
	}

	// The cabinet: read back with the parameters, update, pause, unpause.
	st, body = call(mentorID, "GET", "/me/mentorship/profile", "")
	expect(st, 200, "own profile", body)
	if d := data(body); d["horizon_days"] != float64(30) || d["notice_minutes"] != float64(0) {
		t.Errorf("owner parameters = %v", d)
	}
	st, body = call(mentorID, "PUT", "/me/mentorship/profile", `{
		"company_slug": "contractco", "slug": "contract-mentor", "name": "Contract Mentor",
		"headline": "Principal Engineer", "topics": ["career"], "languages": ["en"],
		"timezone": "UTC", "meeting_url": "https://meet.example.test/contract",
		"session_minutes": 45, "horizon_days": 30
	}`)
	expect(st, 200, "update", body)
	if data(body)["headline"] != "Principal Engineer" {
		t.Errorf("update answer = %v", body)
	}
	st, body = call(mentorID, "POST", "/me/mentorship/profile/pause", `{"paused":true}`)
	expect(st, 200, "pause", body)
	if data(body)["paused"] != true {
		t.Errorf("pause answer = %v", body)
	}
	st, body = call(mentorID, "POST", "/me/mentorship/profile/pause", `{"paused":false}`)
	expect(st, 200, "unpause", body)
	if _, has := data(body)["paused"]; has {
		t.Errorf("a profile that is not paused must omit the flag: %v", body)
	}
	st, body = call(0, "GET", "/me/mentorship/profile", "")
	expect(st, 401, "own profile signed out", body)
}
