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
	bookingStart := time.Now().UTC().AddDate(0, 0, 3).Truncate(time.Hour)
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

	month := bookingStart.Format("2006-01")

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
}
