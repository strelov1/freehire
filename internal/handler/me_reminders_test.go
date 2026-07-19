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
	settings   reminder.Settings
	upserts    int
	cancels    int
	reschedule int
}

func (r *stubReminderRepo) JobIDBySlug(context.Context, string) (int64, error) { return 1, nil }
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
func (r *stubReminderRepo) RescheduleReminder(context.Context, int64, int64, time.Time) error {
	r.reschedule++
	return nil
}

// remindersApp mounts the save/apply/unsave + reminder routes on an API whose
// tracking and reminder services are backed by DB-free stubs. The returned repo
// lets tests assert the orchestration (a save schedules, an apply/unsave cancels).
func remindersApp(settings reminder.Settings) (*fiber.App, *auth.Issuer, *stubReminderRepo) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	repo := &stubReminderRepo{settings: settings}
	h := &API{
		issuer:   iss,
		tracking: jobtracking.New(stubTrackingRepo{}),
		reminder: reminder.New(repo),
	}
	app := fiber.New()
	gate := auth.RequireAuth(iss)
	app.Post("/jobs/:slug/apply", gate, h.MarkApplied)
	app.Post("/jobs/:slug/save", gate, h.SaveJob)
	app.Delete("/jobs/:slug/save", gate, h.UnsaveJob)
	app.Patch("/jobs/:slug/reminder", gate, h.RescheduleReminder)
	app.Delete("/jobs/:slug/reminder", gate, h.CancelJobReminder)
	app.Get("/me/reminder-settings", gate, h.GetReminderSettings)
	app.Put("/me/reminder-settings", gate, h.UpdateReminderSettings)
	return app, iss, repo
}

func do(t *testing.T, app *fiber.App, method, path, token, body string) *http.Response {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
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

func TestReminderSettings_RequiresAuth(t *testing.T) {
	app, _, _ := remindersApp(reminder.Settings{})
	if got := do(t, app, fiber.MethodGet, "/me/reminder-settings", "", "").StatusCode; got != fiber.StatusUnauthorized {
		t.Errorf("GET status = %d, want 401", got)
	}
	if got := do(t, app, fiber.MethodPut, "/me/reminder-settings", "", `{"enabled":false}`).StatusCode; got != fiber.StatusUnauthorized {
		t.Errorf("PUT status = %d, want 401", got)
	}
}

func TestSaveJob_SchedulesReminderFromDefault(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"email"}})
	token, _ := iss.Issue(7)
	if got := do(t, app, fiber.MethodPost, "/jobs/go-dev/save", token, "").StatusCode; got != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	if repo.upserts != 1 {
		t.Errorf("want 1 scheduled reminder, got %d", repo.upserts)
	}
}

func TestSaveJob_OptOutOverrideCancels(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"email"}})
	token, _ := iss.Issue(7)
	do(t, app, fiber.MethodPost, "/jobs/go-dev/save", token, `{"reminder":{"disabled":true}}`)
	if repo.upserts != 0 {
		t.Errorf("opt-out must not schedule, got %d upserts", repo.upserts)
	}
	if repo.cancels != 1 {
		t.Errorf("opt-out must cancel, got %d cancels", repo.cancels)
	}
}

func TestSaveJob_DisabledRuleSchedulesNothing(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: false})
	token, _ := iss.Issue(7)
	do(t, app, fiber.MethodPost, "/jobs/go-dev/save", token, "")
	if repo.upserts != 0 || repo.cancels != 0 {
		t.Errorf("off-by-default save must be a no-op: upserts=%d cancels=%d", repo.upserts, repo.cancels)
	}
}

func TestApplyAndUnsave_CancelReminder(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"email"}})
	token, _ := iss.Issue(7)
	do(t, app, fiber.MethodPost, "/jobs/go-dev/apply", token, "")
	do(t, app, fiber.MethodDelete, "/jobs/go-dev/save", token, "")
	if repo.cancels != 2 {
		t.Errorf("apply + unsave must each cancel, got %d cancels", repo.cancels)
	}
}

func TestRescheduleReminder_UpdatesFireAt(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{})
	token, _ := iss.Issue(7)
	resp := do(t, app, fiber.MethodPatch, "/jobs/go-dev/reminder", token, `{"delay_days":7}`)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if repo.reschedule != 1 {
		t.Errorf("want 1 reschedule, got %d", repo.reschedule)
	}
}

func TestRescheduleReminder_RejectsBadDelay(t *testing.T) {
	app, iss, _ := remindersApp(reminder.Settings{})
	token, _ := iss.Issue(7)
	resp := do(t, app, fiber.MethodPatch, "/jobs/go-dev/reminder", token, `{"delay_days":9999}`)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCancelJobReminder_Cancels(t *testing.T) {
	app, iss, repo := remindersApp(reminder.Settings{})
	token, _ := iss.Issue(7)
	resp := do(t, app, fiber.MethodDelete, "/jobs/go-dev/reminder", token, "")
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if repo.cancels != 1 {
		t.Errorf("want 1 cancel, got %d", repo.cancels)
	}
}

func TestUpdateReminderSettings_RejectsEnabledWithoutChannels(t *testing.T) {
	app, iss, _ := remindersApp(reminder.Settings{})
	token, _ := iss.Issue(7)
	resp := do(t, app, fiber.MethodPut, "/me/reminder-settings", token, `{"enabled":true,"default_delay_days":3,"channels":[]}`)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestReminderSettingsResponse_Shape(t *testing.T) {
	raw, err := json.Marshal(toReminderSettingsResponse(reminder.Settings{Enabled: true, DefaultDelayDays: 3}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := fields["user_id"]; leaked {
		t.Error("reminderSettingsResponse must not include user_id")
	}
	if got := string(fields["channels"]); got != "[]" {
		t.Errorf("channels = %s, want [] for a nil slice", got)
	}
	for _, want := range []string{"enabled", "default_delay_days", "channels"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("response missing %q", want)
		}
	}
}
