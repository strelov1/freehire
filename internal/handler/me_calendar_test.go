package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/gmailsync"
)

// calendarConnectApp mounts the consent start behind RequireAuth. No database: the
// handler mints state, sets a cookie and redirects, and never reaches the store.
func calendarConnectApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &inboxHandlers{
		gmailConnector: gmailsync.NewConnector("client-id", "secret", "https://freehire.me"),
		frontendOrigin: "https://freehire.me",
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/calendar/connect", auth.RequireAuth(iss, testVersions), h.CalendarConnect)
	return app, token
}

func TestCalendarConnect_RequiresAuth(t *testing.T) {
	app, _ := calendarConnectApp(t)
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me/calendar/connect", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// The consent sends the candidate to Google for the calendar and nothing else, carrying
// its own state cookie so a mail consent in flight cannot complete this one.
func TestCalendarConnect_SendsToGoogleForTheCalendarAlone(t *testing.T) {
	app, token := calendarConnectApp(t)
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me/calendar/connect", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "accounts.google.com") {
		t.Errorf("redirected to %q, want Google's consent screen", location)
	}
	if !strings.Contains(location, "calendar.readonly") {
		t.Errorf("the consent did not ask for the calendar: %q", location)
	}
	if strings.Contains(location, "gmail.readonly") {
		t.Errorf("the calendar consent also asked for mail: %q", location)
	}
	// Its own cookie, not the mail flow's.
	var names []string
	for _, ck := range resp.Cookies() {
		names = append(names, ck.Name)
	}
	if !slices.Contains(names, "hire_calendar_state") {
		t.Errorf("cookies %v, want the calendar's own state cookie", names)
	}
	if slices.Contains(names, "hire_gmail_state") {
		t.Errorf("the calendar consent set the mail flow's state cookie: %v", names)
	}
}
