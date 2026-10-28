//go:build integration

// Integration tests for the mentor's own resolved-calendar HTTP flow against a real
// Postgres: a malformed month is refused, a signed-out caller is refused, a caller with
// no mentor profile is refused, and an authenticated mentor's booking shows up as booked.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestGetMyCalendarHTTPFlow(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var mentorUserID, otherUserID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('mentor-cal-http@example.test', true) RETURNING id`,
	).Scan(&mentorUserID); err != nil {
		t.Fatalf("seed mentor user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('no-profile-cal-http@example.test', true) RETURNING id`,
	).Scan(&otherUserID); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('calhttpco', 'Cal HTTP Co') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	queries := db.New(pool)
	mentor, err := queries.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID: mentorUserID, CompanySlug: pgtype.Text{String: "calhttpco", Valid: true}, Slug: "cal-http-mentor",
		DisplayName: "Cal H.", Headline: "Engineering Manager", Bio: "",
		Topics: []string{"career"}, Languages: []string{"en"},
		Timezone: "Europe/Berlin", SessionDurationMin: 60,
		MinNoticeMin: 0, HorizonDays: 60,
		MeetingUrl: "https://meet.example.test/cal-http",
	})
	if err != nil {
		t.Fatalf("CreateMentorProfile: %v", err)
	}

	// No availability rule is seeded: a confirmed booking is carved out as booked
	// regardless of the mentor's stated hours, which is exactly what this test checks.
	// Pinned to the month boundary right after the autumn clock change, next year: 23:00 UTC on
	// 31 October is already 00:00 on 1 November in Berlin (CET), so the booking belongs to the
	// mentor's November. A date taken from the wall clock made this test pass or fail by the
	// day it ran.
	bookingStart := time.Date(time.Now().Year()+1, time.October, 31, 23, 0, 0, 0, time.UTC)
	seekerID := otherUserID
	if _, err := queries.CreateMentorBooking(ctx, db.CreateMentorBookingParams{
		MentorID: mentor.ID, SeekerUserID: seekerID,
		StartsAt:       pgtype.Timestamptz{Time: bookingStart, Valid: true},
		EndsAt:         pgtype.Timestamptz{Time: bookingStart.Add(time.Hour), Valid: true},
		SeekerTimezone: "Europe/Berlin", MeetingUrl: "https://meet.example.test/cal-http",
	}); err != nil {
		t.Fatalf("CreateMentorBooking: %v", err)
	}

	h := newMentorshipHandlers(mentorship.New(
		mentorship.NewQueriesRepository(queries, pool),
		mentorship.Config{},
	), nil)

	iss := auth.NewIssuer("test-secret", time.Hour)
	mentorCookie, _ := iss.Issue(mentorUserID, testTokenVersion)
	otherCookie, _ := iss.Issue(otherUserID, testTokenVersion)
	cookieAuth := auth.RequireAuth(iss, testVersions)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/mentorship/availability/calendar", cookieAuth, h.GetMyCalendar)

	// The calendar's month is the mentor's own, so it is read in the mentor's zone.
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	month := bookingStart.In(berlin).Format("2006-01")

	// Signed out is refused.
	signedOutResp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet,
		"/api/v1/me/mentorship/availability/calendar?month="+month, nil))
	if err != nil {
		t.Fatalf("signed-out request: %v", err)
	}
	defer signedOutResp.Body.Close()
	if signedOutResp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("signed-out status = %d, want 401", signedOutResp.StatusCode)
	}

	// A caller with no mentor profile at all is refused.
	noProfileReq := httptest.NewRequestWithContext(ctx, fiber.MethodGet,
		"/api/v1/me/mentorship/availability/calendar?month="+month, nil)
	noProfileReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: otherCookie})
	noProfileResp, err := app.Test(noProfileReq)
	if err != nil {
		t.Fatalf("no-profile request: %v", err)
	}
	defer noProfileResp.Body.Close()
	if noProfileResp.StatusCode != fiber.StatusNotFound {
		t.Errorf("no-profile status = %d, want 404", noProfileResp.StatusCode)
	}

	// A malformed month is refused.
	badMonthReq := httptest.NewRequestWithContext(ctx, fiber.MethodGet,
		"/api/v1/me/mentorship/availability/calendar?month=not-a-month", nil)
	badMonthReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mentorCookie})
	badMonthResp, err := app.Test(badMonthReq)
	if err != nil {
		t.Fatalf("bad-month request: %v", err)
	}
	defer badMonthResp.Body.Close()
	if badMonthResp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Errorf("bad-month status = %d, want 422", badMonthResp.StatusCode)
	}

	// The mentor's own booking shows up as booked.
	okReq := httptest.NewRequestWithContext(ctx, fiber.MethodGet,
		"/api/v1/me/mentorship/availability/calendar?month="+month, nil)
	okReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mentorCookie})
	okResp, err := app.Test(okReq)
	if err != nil {
		t.Fatalf("ok request: %v", err)
	}
	defer okResp.Body.Close()
	body, _ := io.ReadAll(okResp.Body)
	if okResp.StatusCode != fiber.StatusOK {
		t.Fatalf("ok status = %d, body = %s", okResp.StatusCode, body)
	}

	var parsed struct {
		Data []struct {
			StartsAt time.Time `json:"starts_at"`
			EndsAt   time.Time `json:"ends_at"`
			Status   string    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, body)
	}

	found := false
	for _, iv := range parsed.Data {
		if iv.Status == "booked" && iv.StartsAt.Equal(bookingStart) {
			found = true
		}
	}
	if !found {
		t.Errorf("no booked interval starting at %v in %+v", bookingStart, parsed.Data)
	}

	// The month before ends exactly where the mentor's November begins — 23:00 UTC, since
	// Berlin is back on CET by then — and does not carry the booking.
	prevMonth := bookingStart.In(berlin).AddDate(0, -1, 0).Format("2006-01")
	prevReq := httptest.NewRequestWithContext(ctx, fiber.MethodGet,
		"/api/v1/me/mentorship/availability/calendar?month="+prevMonth, nil)
	prevReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mentorCookie})
	prevResp, err := app.Test(prevReq)
	if err != nil {
		t.Fatalf("previous-month request: %v", err)
	}
	defer prevResp.Body.Close()
	prevBody, _ := io.ReadAll(prevResp.Body)
	var prev struct {
		Data []struct {
			EndsAt time.Time `json:"ends_at"`
			Status string    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(prevBody, &prev); err != nil || len(prev.Data) == 0 {
		t.Fatalf("previous month: %v, body = %s", err, prevBody)
	}
	if end := prev.Data[len(prev.Data)-1].EndsAt; !end.Equal(bookingStart) {
		t.Errorf("previous month ends at %v, want %v (midnight in Berlin after the clock change)", end, bookingStart)
	}
	for _, iv := range prev.Data {
		if iv.Status == "booked" {
			t.Errorf("the previous month carries the next month's booking: %+v", prev.Data)
		}
	}
}
