package notify

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// fakePushTokenLister stands in for *db.Queries's ListPushTokensForUser.
type fakePushTokenLister struct {
	tokens map[int64][]db.UserPushToken
}

func (f *fakePushTokenLister) ListPushTokensForUser(_ context.Context, userID int64) ([]db.UserPushToken, error) {
	return f.tokens[userID], nil
}

// fakePushTransport is a pushnotify.Notifier test double that records every
// call so a test can assert on the rendered title/body/data.
type fakePushTransport struct {
	calls []pushCall
}

type pushCall struct {
	token, title, body string
	data               map[string]string
}

func (f *fakePushTransport) Send(_ context.Context, token, title, body string, data map[string]string) error {
	f.calls = append(f.calls, pushCall{token: token, title: title, body: body, data: data})
	return nil
}

func TestPushNotifier_SingleJobDigestCarriesSlugDeepLink(t *testing.T) {
	tokens := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "ExponentPushToken[abc]"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(tokens, transport)

	d := Digest{
		SavedSearchName: "Backend Engineer",
		Total:           1,
		Jobs:            []DigestJob{{Title: "Backend Engineer", Company: "Acme", Slug: "acme-backend-engineer"}},
	}
	if err := n.Send(context.Background(), ChannelPush, "42", d); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(transport.calls) != 1 {
		t.Fatalf("transport calls = %d, want 1", len(transport.calls))
	}
	call := transport.calls[0]
	if call.token != "ExponentPushToken[abc]" {
		t.Errorf("token = %q, want the user's registered token", call.token)
	}
	if call.data["slug"] != "acme-backend-engineer" {
		t.Errorf("data[slug] = %q, want %q", call.data["slug"], "acme-backend-engineer")
	}
}

func TestPushNotifier_MultiJobDigestCarriesNoDeepLink(t *testing.T) {
	tokens := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "ExponentPushToken[abc]"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(tokens, transport)

	d := Digest{
		SavedSearchName: "Backend Engineer",
		Total:           3,
		Jobs: []DigestJob{
			{Title: "A", Slug: "a"},
			{Title: "B", Slug: "b"},
			{Title: "C", Slug: "c"},
		},
	}
	if err := n.Send(context.Background(), ChannelPush, "42", d); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(transport.calls) != 1 {
		t.Fatalf("transport calls = %d, want 1", len(transport.calls))
	}
	if transport.calls[0].data != nil {
		t.Errorf("data = %v, want nil for a multi-job digest", transport.calls[0].data)
	}
}

func TestPushNotifier_TitleAndBodyContent(t *testing.T) {
	tokens := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "ExponentPushToken[abc]"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(tokens, transport)

	d := Digest{SavedSearchName: "Backend Engineer", Total: 3}
	if err := n.Send(context.Background(), ChannelPush, "42", d); err != nil {
		t.Fatalf("Send: %v", err)
	}
	call := transport.calls[0]
	if call.title != "freehire" {
		t.Errorf("title = %q, want %q", call.title, "freehire")
	}
	wantBody := `3 new jobs for "Backend Engineer"`
	if call.body != wantBody {
		t.Errorf("body = %q, want %q", call.body, wantBody)
	}
}

func TestPushNotifier_FansOutToEveryRegisteredDevice(t *testing.T) {
	tokens := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "tok-1"}, {Token: "tok-2"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(tokens, transport)

	d := Digest{SavedSearchName: "Backend Engineer", Total: 3}
	if err := n.Send(context.Background(), ChannelPush, "42", d); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(transport.calls) != 2 {
		t.Fatalf("transport calls = %d, want 2 (one per device)", len(transport.calls))
	}
}

func TestPushNotifier_InvalidDestErrors(t *testing.T) {
	n := NewPushNotifier(&fakePushTokenLister{}, &fakePushTransport{})
	if err := n.Send(context.Background(), ChannelPush, "not-a-user-id", Digest{}); err == nil {
		t.Fatal("Send: want error for a non-numeric dest")
	}
}
