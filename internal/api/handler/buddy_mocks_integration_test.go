//go:build integration

package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// openMockHost gives user settings and a whole week of hours over the HTTP contract, and
// returns their meeting link.
func (e *buddyEnv) openMockHost(user string) string {
	e.t.Helper()
	link := "https://meet.example.test/" + user + "-room"
	e.want(http.StatusOK, http.MethodPut, "/api/v1/me/buddies/mocks/settings", user, map[string]any{
		"timezone": "UTC", "meeting_url": link, "topics": []string{"coding", "behavioral"}, "paused": false,
	})
	rules := make([]map[string]any, 0, 7)
	for day := 0; day < 7; day++ {
		rules = append(rules, map[string]any{"weekday": day, "start": "00:00", "end": "24:00"})
	}
	e.raw(http.StatusNoContent, http.MethodPut, "/api/v1/me/buddies/mocks/availability/weekly", user, map[string]any{"rules": rules})
	return link
}

func TestBuddyMockSettingsAreValidated(t *testing.T) {
	env := newBuddyEnv(t, "a")
	env.want(http.StatusUnauthorized, http.MethodGet, "/api/v1/me/buddies/mocks", "", nil)
	env.join("a")

	env.want(http.StatusUnprocessableEntity, http.MethodPut, "/api/v1/me/buddies/mocks/settings", "a", map[string]any{
		"timezone": "Mars/Olympus", "meeting_url": "https://meet.example.test/x", "topics": []string{"coding"},
	})
	env.want(http.StatusUnprocessableEntity, http.MethodPut, "/api/v1/me/buddies/mocks/settings", "a", map[string]any{
		"timezone": "UTC", "meeting_url": "http://meet.example.test/x", "topics": []string{"coding"},
	})
	if out := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks/settings", "a", nil); out["data"] != nil {
		t.Fatalf("refused settings were stored: %v", out)
	}
}

func TestBuddyMockBookingOverHTTP(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest", "late")
	hostHandle := env.join("host")
	guestHandle := env.join("guest")
	env.join("late")
	link := env.openMockHost("host")
	slotsPath := "/api/v1/me/buddies/profiles/" + hostHandle + "/mock-slots?timezone=Asia/Tokyo"

	if raw := env.raw(http.StatusOK, http.MethodGet, slotsPath, "guest", nil); strings.Contains(raw, link) {
		t.Fatal("the slot listing carries the meeting link")
	}
	listing := env.want(http.StatusOK, http.MethodGet, slotsPath, "guest", nil)
	if meta := listing["meta"].(map[string]any); meta["timezone"] != "Asia/Tokyo" || len(meta["topics"].([]any)) != 2 {
		t.Fatalf("meta = %v", meta)
	}
	first := listing["data"].([]any)[0].(map[string]any)["starts_at"].(string)
	body := map[string]any{"starts_at": first, "topic": "coding", "timezone": "Asia/Tokyo"}
	bookPath := "/api/v1/me/buddies/profiles/" + hostHandle + "/mocks"

	mock := env.want(http.StatusCreated, http.MethodPost, bookPath, "guest", body)["data"].(map[string]any)
	if mock["role"] != "guest" || mock["counterpart"] != hostHandle || mock["meeting_url"] != link || mock["can_cancel"] != true {
		t.Fatalf("booked = %v", mock)
	}
	env.want(http.StatusConflict, http.MethodPost, bookPath, "late", body)
	env.want(http.StatusUnprocessableEntity, http.MethodPost, bookPath, "late",
		map[string]any{"starts_at": first, "topic": "system_design", "timezone": "UTC"})

	hostRaw := env.raw(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", "host", nil)
	for _, leak := range []string{"guest@example.test", strconv.FormatInt(env.users["guest"], 10)} {
		if strings.Contains(hostRaw, leak) {
			t.Fatalf("the host's list carries %q", leak)
		}
	}
	upcoming := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", "host", nil)["data"].(map[string]any)["upcoming"].([]any)
	if up := upcoming[0].(map[string]any); up["role"] != "host" || up["counterpart"] != guestHandle {
		t.Fatalf("host's upcoming = %v", up)
	}

	env.raw(http.StatusNoContent, http.MethodPost, "/api/v1/me/buddies/mocks/"+mock["id"].(string)+"/cancel", "guest", nil)
	past := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", "guest", nil)["data"].(map[string]any)["past"].([]any)
	if p := past[0].(map[string]any); p["status"] != "cancelled" || p["meeting_url"] != nil {
		t.Fatalf("guest's past = %v", p)
	}
	env.want(http.StatusNotFound, http.MethodPost, "/api/v1/me/buddies/mocks/not-a-uuid/cancel", "guest", nil)
}

func TestBuddyMockRoutesHideAMemberAcrossABlock(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest")
	hostHandle := env.join("host")
	env.join("guest")
	env.openMockHost("host")
	if _, err := env.pool.Exec(context.Background(),
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, env.users["host"], env.users["guest"]); err != nil {
		t.Fatal(err)
	}
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/"+hostHandle+"/mock-slots", "guest", nil)
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/"+hostHandle+"/mock-reviews", "guest", nil)
}

// bookFirstMock books the host's earliest open slot for guest and returns the mock id.
func (e *buddyEnv) bookFirstMock(hostHandle, guest string) string {
	e.t.Helper()
	listing := e.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/profiles/"+hostHandle+"/mock-slots", guest, nil)
	first := listing["data"].([]any)[0].(map[string]any)["starts_at"].(string)
	mock := e.want(http.StatusCreated, http.MethodPost, "/api/v1/me/buddies/profiles/"+hostHandle+"/mocks", guest,
		map[string]any{"starts_at": first, "topic": "coding", "timezone": "UTC"})["data"].(map[string]any)
	return mock["id"].(string)
}

// mockCancelNotices counts the cancellation notices user has in the notification center.
func (e *buddyEnv) mockCancelNotices(user string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM user_notifications WHERE user_id = $1 AND kind = 'buddy_mock_cancelled'`,
		e.users[user]).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestBuddyLeaveTellsTheOtherMockParticipant(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest")
	hostHandle := env.join("host")
	env.join("guest")
	env.openMockHost("host")
	env.bookFirstMock(hostHandle, "guest")

	env.raw(http.StatusNoContent, http.MethodDelete, "/api/v1/me/buddies", "guest", nil)

	if got := env.mockCancelNotices("host"); got != 1 {
		t.Fatalf("host cancellation notices after the guest left = %d, want 1", got)
	}
	if got := env.mockCancelNotices("guest"); got != 0 {
		t.Fatalf("the member who left was told about their own leave: %d", got)
	}
}

func TestBuddyBlockTellsTheOtherMockParticipant(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest")
	hostHandle := env.join("host")
	env.join("guest")
	env.openMockHost("host")
	env.bookFirstMock(hostHandle, "guest")
	conv := env.want(http.StatusCreated, http.MethodPost, "/api/v1/me/buddies/invite", "guest",
		map[string]any{"handle": hostHandle, "body": "see you at the mock"})["data"].(map[string]any)

	env.raw(http.StatusNoContent, http.MethodPost, convPath(conv["id"].(float64), "/block"), "host", nil)

	if got := env.mockCancelNotices("guest"); got != 1 {
		t.Fatalf("guest cancellation notices after the host blocked = %d, want 1", got)
	}
	if got := env.mockCancelNotices("host"); got != 0 {
		t.Fatalf("the blocker was told about their own block: %d", got)
	}
}

func TestBuddyMockHostsOverHTTP(t *testing.T) {
	env := newBuddyEnv(t, "host", "viewer", "outsider")
	hostHandle := env.join("host")
	env.join("viewer")
	link := env.openMockHost("host")
	path := "/api/v1/me/buddies/mocks/hosts?timezone=Asia/Tokyo"

	env.want(http.StatusUnauthorized, http.MethodGet, path, "", nil)
	env.want(http.StatusForbidden, http.MethodGet, path, "outsider", nil)
	env.want(http.StatusUnprocessableEntity, http.MethodGet, path+"&topic=astrology", "viewer", nil)

	if raw := env.raw(http.StatusOK, http.MethodGet, path, "viewer", nil); strings.Contains(raw, link) {
		t.Fatal("the hosts listing carries the meeting link")
	}
	out := env.want(http.StatusOK, http.MethodGet, path, "viewer", nil)
	if meta := out["meta"].(map[string]any); meta["timezone"] != "Asia/Tokyo" || meta["count"] != float64(1) {
		t.Fatalf("meta = %v", meta)
	}
	host := out["data"].([]any)[0].(map[string]any)
	next := host["next_slot"].(map[string]any)
	if host["handle"] != hostHandle || host["card_summary"] == nil || len(host["topics"].([]any)) != 2 ||
		next["utc_offset"] != "+09:00" || !strings.HasSuffix(next["local_start"].(string), "+09:00") {
		t.Fatalf("host = %v", host)
	}
	if got := env.want(http.StatusOK, http.MethodGet, path+"&topic=system_design", "viewer", nil); len(got["data"].([]any)) != 0 {
		t.Fatalf("a topic the host does not offer still lists them: %v", got)
	}
	// The host does not see themselves.
	if got := env.want(http.StatusOK, http.MethodGet, path, "host", nil); len(got["data"].([]any)) != 0 {
		t.Fatalf("the host lists themselves: %v", got)
	}
}
