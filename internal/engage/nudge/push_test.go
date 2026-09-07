package nudge

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakePushTokenLister is a DB-free PushTokenLister.
type fakePushTokenLister struct {
	tokens map[int64][]db.UserPushToken
}

func (l *fakePushTokenLister) ListPushTokensForUser(_ context.Context, userID int64) ([]db.UserPushToken, error) {
	return l.tokens[userID], nil
}

// fakePushTransport is a DB-free pushnotify.Notifier that records every call.
type fakePushTransport struct {
	sent []pushCall
	err  error
}

type pushCall struct {
	token, title, body string
	data               map[string]string
}

func (t *fakePushTransport) Send(_ context.Context, token, title, body string, data map[string]string) error {
	if t.err != nil {
		return t.err
	}
	t.sent = append(t.sent, pushCall{token: token, title: title, body: body, data: data})
	return nil
}

func TestPushNotifier_FollowUp_RendersTitleBodyAndSlug(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "tok-1"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	msg := Message{Kind: KindFollowUp, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme", DaysSilent: 12}
	if err := n.Send(context.Background(), "push", "42", msg.Kind, []Message{msg}); err != nil {
		t.Fatal(err)
	}

	if len(transport.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(transport.sent))
	}
	got := transport.sent[0]
	if got.token != "tok-1" {
		t.Errorf("token = %q, want %q", got.token, "tok-1")
	}
	if got.title != "👋 Follow up?" {
		t.Errorf("title = %q, want %q", got.title, "👋 Follow up?")
	}
	wantBody := "It's been 12 days since anything moved on Go Dev at Acme."
	if got.body != wantBody {
		t.Errorf("body = %q, want %q", got.body, wantBody)
	}
	if got.data["slug"] != "go-dev-acme" {
		t.Errorf("data[slug] = %q, want %q", got.data["slug"], "go-dev-acme")
	}
}

func TestPushNotifier_InterviewPrep_RendersTitleBodyAndSlug(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "tok-1"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	msg := Message{Kind: KindInterviewPrep, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}
	if err := n.Send(context.Background(), "push", "42", msg.Kind, []Message{msg}); err != nil {
		t.Fatal(err)
	}

	if len(transport.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(transport.sent))
	}
	got := transport.sent[0]
	if got.title != "🎯 Interview coming up" {
		t.Errorf("title = %q, want %q", got.title, "🎯 Interview coming up")
	}
	wantBody := "You're interviewing for Go Dev at Acme."
	if got.body != wantBody {
		t.Errorf("body = %q, want %q", got.body, wantBody)
	}
	if got.data["slug"] != "go-dev-acme" {
		t.Errorf("data[slug] = %q, want %q", got.data["slug"], "go-dev-acme")
	}
}

func TestPushNotifier_JobClosed_RendersTitleBodyAndSlug(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "tok-1"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	msg := Message{Kind: KindJobClosed, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}
	if err := n.Send(context.Background(), "push", "42", msg.Kind, []Message{msg}); err != nil {
		t.Fatal(err)
	}

	if len(transport.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(transport.sent))
	}
	got := transport.sent[0]
	if got.title != "📪 Job closed" {
		t.Errorf("title = %q, want %q", got.title, "📪 Job closed")
	}
	wantBody := "Go Dev at Acme was closed."
	if got.body != wantBody {
		t.Errorf("body = %q, want %q", got.body, wantBody)
	}
	if got.data["slug"] != "go-dev-acme" {
		t.Errorf("data[slug] = %q, want %q", got.data["slug"], "go-dev-acme")
	}
}

func TestPushNotifier_AutoApplyOutcomeKinds_SingleAndBatch(t *testing.T) {
	cases := []struct {
		kind      string
		wantTitle string
	}{
		{KindAutoApplySubmitted, "🎉 Application submitted"},
		{KindAutoApplyBlocked, "⚠️ Needs your attention"},
		{KindAutoApplyFailed, "Auto-apply couldn't submit"},
	}
	for _, c := range cases {
		t.Run(c.kind+"/single", func(t *testing.T) {
			lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{42: {{Token: "tok-1"}}}}
			transport := &fakePushTransport{}
			n := NewPushNotifier(lister, transport)

			msg := Message{Kind: c.kind, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}
			if err := n.Send(context.Background(), "push", "42", msg.Kind, []Message{msg}); err != nil {
				t.Fatal(err)
			}
			if len(transport.sent) != 1 {
				t.Fatalf("sent = %d, want 1", len(transport.sent))
			}
			got := transport.sent[0]
			if got.title != c.wantTitle {
				t.Errorf("title = %q, want %q", got.title, c.wantTitle)
			}
			if !strings.Contains(got.body, "Go Dev") || !strings.Contains(got.body, "Acme") {
				t.Errorf("body = %q, want the job title and company", got.body)
			}
			if got.data["slug"] != "go-dev-acme" {
				t.Errorf("data[slug] = %q, want %q", got.data["slug"], "go-dev-acme")
			}
		})
		t.Run(c.kind+"/batch", func(t *testing.T) {
			lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{42: {{Token: "tok-1"}}}}
			transport := &fakePushTransport{}
			n := NewPushNotifier(lister, transport)

			ms := []Message{
				{Kind: c.kind, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"},
				{Kind: c.kind, JobTitle: "Rust Dev", Company: "Beta", Slug: "rust-dev-beta"},
			}
			if err := n.Send(context.Background(), "push", "42", c.kind, ms); err != nil {
				t.Fatal(err)
			}
			if len(transport.sent) != 1 {
				t.Fatalf("sent = %d, want 1", len(transport.sent))
			}
			if !strings.Contains(transport.sent[0].body, "2") {
				t.Errorf("body = %q, want the batch count", transport.sent[0].body)
			}
			// A batch names no single job, so no deep link is carried.
			if transport.sent[0].data != nil {
				t.Errorf("data = %v, want nil for a multi-job batch", transport.sent[0].data)
			}
		})
	}
}

func TestPushNotifier_FansOutToEveryDevice(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "tok-1"}, {Token: "tok-2"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	msg := Message{Kind: KindJobClosed, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}
	if err := n.Send(context.Background(), "push", "42", msg.Kind, []Message{msg}); err != nil {
		t.Fatal(err)
	}
	if len(transport.sent) != 2 {
		t.Fatalf("sent = %d, want 2 (one per device)", len(transport.sent))
	}
}

func TestPushNotifier_InvalidDestReturnsError(t *testing.T) {
	lister := &fakePushTokenLister{}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	err := n.Send(context.Background(), "push", "not-a-user-id", KindJobClosed, []Message{{Kind: KindJobClosed}})
	if err == nil {
		t.Fatal("want error for a non-numeric dest")
	}
}

func TestPushNotifier_NoDeviceReturnsError(t *testing.T) {
	lister := &fakePushTokenLister{}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	err := n.Send(context.Background(), "push", "42", KindJobClosed, []Message{{Kind: KindJobClosed}})
	if err == nil {
		t.Fatal("want error when the user has no registered device")
	}
}
