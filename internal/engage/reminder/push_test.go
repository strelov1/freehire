package reminder

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakePushTokenLister is a DB-free PushTokenLister serving canned tokens keyed
// by user id.
type fakePushTokenLister struct {
	tokens map[int64][]db.UserPushToken
	err    error
}

func (f *fakePushTokenLister) ListPushTokensForUser(_ context.Context, userID int64) ([]db.UserPushToken, error) {
	return f.tokens[userID], f.err
}

// fakePushTransport records every Send call, standing in for pushnotify.Notifier.
type fakePushTransport struct {
	calls []struct {
		token, title, body string
		data               map[string]string
	}
	err error
}

func (f *fakePushTransport) Send(_ context.Context, token, title, body string, data map[string]string) error {
	f.calls = append(f.calls, struct {
		token, title, body string
		data               map[string]string
	}{token, title, body, data})
	return f.err
}

func TestPushNotifier_RendersTitleBodyAndSlugDataToEveryDevice(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "tok-1"}, {Token: "tok-2"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)
	msg := ReminderMessage{JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme", URL: "https://ats/x"}

	if err := n.Send(context.Background(), "push", strconv.FormatInt(42, 10), []ReminderMessage{msg}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(transport.calls) != 2 {
		t.Fatalf("transport calls = %d, want 2 (one per device)", len(transport.calls))
	}
	gotTokens := map[string]bool{}
	for _, c := range transport.calls {
		gotTokens[c.token] = true
		if c.title != "⏰ Reminder" {
			t.Errorf("title = %q, want %q", c.title, "⏰ Reminder")
		}
		if c.body != "You saved Go Dev at Acme — still interested?" {
			t.Errorf("body = %q", c.body)
		}
		if c.data["slug"] != "go-dev-acme" {
			t.Errorf("data[slug] = %q, want %q", c.data["slug"], "go-dev-acme")
		}
	}
	if !gotTokens["tok-1"] || !gotTokens["tok-2"] {
		t.Errorf("tokens sent = %v, want tok-1 and tok-2", gotTokens)
	}
}

func TestPushNotifier_InvalidUserIDErrors(t *testing.T) {
	n := NewPushNotifier(&fakePushTokenLister{}, &fakePushTransport{})
	if err := n.Send(context.Background(), "push", "not-a-number", []ReminderMessage{{Slug: "s"}}); err == nil {
		t.Error("want an error for a non-numeric user id")
	}
}

func TestPushNotifier_NoDevicesReturnsAnError(t *testing.T) {
	// A caller reaches PushNotifier only via recipient()'s HasPushDevice check, so
	// this is a defense-in-depth path, not the normal soft-skip — but the fan-out
	// helper still reports it as an error (SendToDevices' empty-tokens rule).
	n := NewPushNotifier(&fakePushTokenLister{}, &fakePushTransport{})
	if err := n.Send(context.Background(), "push", "42", []ReminderMessage{{Slug: "s"}}); err == nil {
		t.Error("want an error when the user has no registered devices")
	}
}

// A batch is one notification, not one per saved job — the whole point of the
// grouping. The deep link is dropped because several jobs have no one destination.
func TestPushNotifier_BatchIsOneNotificationWithoutADeepLink(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{42: {{Token: "tok-1"}}}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	err := n.Send(context.Background(), "push", "42", []ReminderMessage{
		{JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"},
		{JobTitle: "SRE", Company: "Globex", Slug: "sre-globex"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(transport.calls) != 1 {
		t.Fatalf("transport calls = %d, want 1 for one device and one batch", len(transport.calls))
	}
	call := transport.calls[0]
	if !strings.Contains(call.body, "2 jobs") {
		t.Errorf("body = %q, want the batch count", call.body)
	}
	if call.data["slug"] != "" {
		t.Errorf("data[slug] = %q, want no deep link for a multi-job batch", call.data["slug"])
	}
}
