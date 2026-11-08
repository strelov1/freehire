//go:build integration

package handler

import (
	"net/http"
	"strings"
	"testing"
)

// mcpAs opens the signed-in MCP server's tools for one of the env's users.
func (e *buddyEnv) mcpAs(user string) *mcpToolCaller {
	e.t.Helper()
	return newMCPToolCaller(e.t, &assistantHandlers{buddies: e.buddies}, e.users[user])
}

// failsWith runs a tool that must fail and checks the sentence it fails with.
func (c *mcpToolCaller) failsWith(name string, args map[string]any, want string) {
	c.t.Helper()
	text, failed := c.call(name, args)
	if !failed || !strings.Contains(text, want) {
		c.t.Fatalf("%s: failed=%v text=%q, want a failure containing %q", name, failed, text, want)
	}
}

func TestBuddyMCPHostAndGuestBookAndCancel(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest", "late", "outsider")
	host, guest, late := env.mcpAs("host"), env.mcpAs("guest"), env.mcpAs("late")

	// Acting before joining is refused as the route refuses it.
	env.mcpAs("outsider").failsWith("buddy_mock_hosts", nil, "join the buddy network first")

	hostHandle, _ := host.ok("buddy_join", nil)["handle"].(string)
	guestStatus := guest.ok("buddy_join", nil)
	late.ok("buddy_join", nil)
	if guestStatus["member"] != true || hostHandle == "" {
		t.Fatalf("join: host %q, guest %v", hostHandle, guestStatus)
	}
	if st := guest.ok("buddy_status", nil); st["handle"] != guestStatus["handle"] {
		t.Fatalf("status = %v", st)
	}

	link := "https://meet.example.test/host-room"
	host.ok("buddy_mock_settings_set", map[string]any{
		"timezone": "UTC", "meeting_url": link, "topics": []string{"coding", "behavioral"}, "paused": false,
	})
	rules := make([]map[string]any, 0, 7)
	for day := 0; day < 7; day++ {
		rules = append(rules, map[string]any{"weekday": day, "start": "00:00", "end": "24:00"})
	}
	host.ok("buddy_mock_schedule_set_weekly", map[string]any{"rules": rules})
	host.failsWith("buddy_mock_schedule_set_weekly", map[string]any{"rules": []map[string]any{
		{"date": "2030-01-07", "start": "10:00", "end": "12:00"},
	}}, "weekly schedule takes weekday rules")
	host.failsWith("buddy_mock_schedule_add_override", map[string]any{"start": "10:00", "end": "12:00"}, "an override needs a date")
	override := host.ok("buddy_mock_schedule_add_override", map[string]any{"date": "2030-01-07", "start": "10:00", "end": "12:00"})
	schedule := host.ok("buddy_mock_schedule_get", nil)
	if schedule["settings"].(map[string]any)["meeting_url"] != link || len(schedule["rules"].([]any)) != 8 {
		t.Fatalf("schedule = %v", schedule)
	}
	host.ok("buddy_mock_schedule_delete", map[string]any{"rule_id": override["id"]})

	hosts := guest.ok("buddy_mock_hosts", map[string]any{"topic": "coding", "timezone": "Asia/Tokyo"})
	listed := hosts["hosts"].([]any)
	if len(listed) == 0 || listed[0].(map[string]any)["handle"] != hostHandle {
		t.Fatalf("hosts = %v", hosts)
	}

	slotsText, failed := guest.call("buddy_mock_slots", map[string]any{"handle": hostHandle, "timezone": "Asia/Tokyo"})
	if failed || strings.Contains(slotsText, link) {
		t.Fatalf("slots failed=%v or carry the meeting link: %s", failed, slotsText)
	}
	slotsOut := guest.ok("buddy_mock_slots", map[string]any{"handle": hostHandle, "timezone": "Asia/Tokyo"})
	first := slotsOut["slots"].([]any)[0].(map[string]any)["starts_at"].(string)

	guest.failsWith("buddy_mock_book", map[string]any{"handle": hostHandle, "starts_at": first}, "starts_at and topic are required")
	mock := guest.ok("buddy_mock_book", map[string]any{"handle": hostHandle, "starts_at": first, "topic": "coding", "timezone": "Asia/Tokyo"})
	if mock["role"] != "guest" || mock["counterpart"] != hostHandle || mock["meeting_url"] != link {
		t.Fatalf("booked = %v", mock)
	}
	late.failsWith("buddy_mock_book", map[string]any{"handle": hostHandle, "starts_at": first, "topic": "coding"}, "that time is no longer available")

	// The website shows the same mock to both participants.
	for _, user := range []string{"host", "guest"} {
		upcoming := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", user, nil)["data"].(map[string]any)["upcoming"].([]any)
		if len(upcoming) != 1 || upcoming[0].(map[string]any)["id"] != mock["id"] {
			t.Fatalf("%s's upcoming on the website = %v", user, upcoming)
		}
	}
	if up := guest.ok("buddy_mock_list", nil)["upcoming"].([]any); len(up) != 1 || up[0].(map[string]any)["id"] != mock["id"] {
		t.Fatalf("guest's upcoming over MCP = %v", up)
	}

	guest.failsWith("buddy_mock_cancel", map[string]any{"id": "not-a-uuid"}, "mock interview not found")
	guest.ok("buddy_mock_cancel", map[string]any{"id": mock["id"]})
	past := guest.ok("buddy_mock_list", nil)["past"].([]any)
	if p := past[0].(map[string]any); p["status"] != "cancelled" || p["meeting_url"] != nil {
		t.Fatalf("guest's past = %v", p)
	}

	// Leaving cancels and tells, as the route does.
	guest.ok("buddy_mock_book", map[string]any{"handle": hostHandle, "starts_at": first, "topic": "coding"})
	guest.ok("buddy_leave", nil)
	if got := env.mockCancelNotices("host"); got != 2 {
		t.Fatalf("host cancellation notices = %d, want 2 (one cancel, one leave)", got)
	}
	if st := guest.ok("buddy_status", nil); st["member"] != false {
		t.Fatalf("status after leave = %v", st)
	}
}
