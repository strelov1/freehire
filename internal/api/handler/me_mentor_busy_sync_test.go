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

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// mentorBusySyncConnectApp mounts the consent start behind RequireAuth. No database: the
// handler mints state, sets a cookie and redirects, and never reaches the store.
func mentorBusySyncConnectApp(t *testing.T) (*fiber.App, string) {
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
	app.Get("/me/mentor-busy-sync/connect", auth.RequireAuth(iss, testVersions), h.MentorBusySyncConnect)
	return app, token
}

func TestMentorBusySyncConnect_RequiresAuth(t *testing.T) {
	app, _ := mentorBusySyncConnectApp(t)
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me/mentor-busy-sync/connect", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// The consent sends the mentor to Google for calendar.readonly alone (Google has no
// narrower scope for free/busy), carrying its own state cookie distinct from every other
// Google flow's — including the candidate's read-only calendar flow, which shares the
// same scope but must not be able to complete this one or vice versa.
func TestMentorBusySyncConnect_SendsToGoogleForCalendarReadonlyAlone(t *testing.T) {
	app, token := mentorBusySyncConnectApp(t)
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me/mentor-busy-sync/connect", nil)
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
		t.Errorf("the consent did not ask for calendar.readonly: %q", location)
	}
	if strings.Contains(location, "gmail.readonly") {
		t.Errorf("the busy-sync consent also asked for mail: %q", location)
	}
	if strings.Contains(location, "calendar.events") {
		t.Errorf("the busy-sync consent also asked for the write scope: %q", location)
	}
	var names []string
	for _, ck := range resp.Cookies() {
		names = append(names, ck.Name)
	}
	if !slices.Contains(names, "hire_mentor_busy_state") {
		t.Errorf("cookies %v, want the busy-sync flow's own state cookie", names)
	}
	if slices.Contains(names, "hire_calendar_state") || slices.Contains(names, "hire_gmail_state") || slices.Contains(names, "hire_mentor_calendar_state") {
		t.Errorf("the busy-sync consent set another flow's state cookie: %v", names)
	}
}
