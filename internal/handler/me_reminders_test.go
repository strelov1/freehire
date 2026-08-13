package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/reminder"
)

// stubReminderRepo is a DB-free reminder.Repository that records scheduling calls
// and serves canned settings, so the handler orchestration can be asserted without
// a database.
type stubReminderRepo struct {
	settings reminder.Settings
	upserts  int
	cancels  int
}

func (r *stubReminderRepo) GetSettings(context.Context, int64) (reminder.Settings, error) {
	return r.settings, nil
}
func (r *stubReminderRepo) UpsertSettings(_ context.Context, _ int64, s reminder.Settings) (reminder.Settings, error) {
	r.settings = s
	return s, nil
}
func (r *stubReminderRepo) UpsertReminder(context.Context, int64, int64, time.Time, []string) error {
	r.upserts++
	return nil
}
func (r *stubReminderRepo) CancelReminder(context.Context, int64, int64) error {
	r.cancels++
	return nil
}

// remindersApp mounts the save/apply/unsave + notification-settings routes on a
// handler whose tracking and reminder services are backed by DB-free stubs. The
// returned repo lets tests assert the orchestration (a save schedules, an
// apply/unsave cancels).
func remindersApp(settings reminder.Settings) (*fiber.App, *auth.Issuer, *stubReminderRepo) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	repo := &stubReminderRepo{settings: settings}
	h := &trackingHandlers{
		tracking: jobtracking.New(stubTrackingRepo{}),
		reminder: reminder.New(repo),
	}
	app := fiber.New()
	gate := auth.RequireAuth(iss, testVersions)
	app.Post("/jobs/:slug/apply", gate, h.MarkApplied)
	app.Post("/jobs/:slug/save", gate, h.SaveJob)
	app.Delete("/jobs/:slug/save", gate, h.UnsaveJob)
	app.Get("/me/notification-settings", gate, h.GetNotificationSettings)
	app.Put("/me/notification-settings", gate, h.UpdateNotificationSettings)
	return app, iss, repo
}

func do(t *testing.T, app *fiber.App, method, path, token, body string) *http.Response {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestNotificationSettings_RequiresAuth(t *testing.T) {
	app, _, _ := remindersApp(reminder.Settings{})
	getResp := do(t, app, fiber.MethodGet, "/me/notification-settings", "", "")
	defer getResp.Body.Close()
	if got := getResp.StatusCode; got != fiber.StatusUnauthorized {
		t.Errorf("GET status = %d, want 401", got)
	}
	putResp := do(t, app, fiber.MethodPut, "/me/notification-settings", "", `{"enabled":false}`)
	defer putResp.Body.Close()
	if got := putResp.StatusCode; got != fiber.StatusUnauthorized {
		t.Errorf("PUT status = %d, want 401", got)
	}
}

func TestSaveJob_SchedulesReminderWhenEnabled(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: true, Channels: []string{"email"}})
	token, _ := iss.Issue(7, testTokenVersion)
	saveResp := do(t, app, fiber.MethodPost, "/jobs/go-dev/save", token, "")
	defer saveResp.Body.Close()
	if got := saveResp.StatusCode; got != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	if repo.upserts != 1 {
		t.Errorf("want 1 scheduled reminder, got %d", repo.upserts)
	}
}

func TestSaveJob_DisabledRuleSchedulesNothing(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: false})
	token, _ := iss.Issue(7, testTokenVersion)
	resp := do(t, app, fiber.MethodPost, "/jobs/go-dev/save", token, "")
	defer resp.Body.Close()
	if repo.upserts != 0 || repo.cancels != 0 {
		t.Errorf("disabled-rule save must be a no-op: upserts=%d cancels=%d", repo.upserts, repo.cancels)
	}
}

func TestApplyAndUnsave_CancelReminder(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: true, Channels: []string{"email"}})
	token, _ := iss.Issue(7, testTokenVersion)
	applyResp := do(t, app, fiber.MethodPost, "/jobs/go-dev/apply", token, "")
	defer applyResp.Body.Close()
	unsaveResp := do(t, app, fiber.MethodDelete, "/jobs/go-dev/save", token, "")
	defer unsaveResp.Body.Close()
	if repo.cancels != 2 {
		t.Errorf("apply + unsave must each cancel, got %d cancels", repo.cancels)
	}
}

func TestUpdateNotificationSettings_RejectsEnabledWithoutChannels(t *testing.T) {
	app, iss, _ := remindersApp(reminder.Settings{})
	token, _ := iss.Issue(7, testTokenVersion)
	resp := do(t, app, fiber.MethodPut, "/me/notification-settings", token, `{"enabled":true,"channels":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestNotificationSettingsResponse_Shape(t *testing.T) {
	raw, err := json.Marshal(toNotificationSettingsResponse(reminder.Settings{Enabled: true}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := fields["user_id"]; leaked {
		t.Error("notificationSettingsResponse must not include user_id")
	}
	if got := string(fields["channels"]); got != "[]" {
		t.Errorf("channels = %s, want [] for a nil slice", got)
	}
	for _, want := range []string{"enabled", "channels"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("response missing %q", want)
		}
	}
}
