package notify

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
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

// The notifier's own job is to hand what renderDigest produced to the
// transport untouched; the wording itself is asserted on the renderer below.
func TestPushNotifier_SendsTheRenderedCopy(t *testing.T) {
	tokens := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{
		42: {{Token: "ExponentPushToken[abc]"}},
	}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(tokens, transport)

	d := Digest{SavedSearchName: "Backend Engineer", Total: 3}
	wantTitle, wantBody, _ := renderDigest(d)
	if err := n.Send(context.Background(), ChannelPush, "42", d); err != nil {
		t.Fatalf("Send: %v", err)
	}
	call := transport.calls[0]
	if call.title != wantTitle || call.body != wantBody {
		t.Errorf("sent (%q, %q), want the rendered (%q, %q)", call.title, call.body, wantTitle, wantBody)
	}
}

// renderDigest is the copy both the push channel and the notification-center
// row read, so its wording is asserted on the renderer itself rather than once
// per reader.
func TestRenderDigest_Copy(t *testing.T) {
	tests := []struct {
		name                string
		digest              Digest
		wantTitle, wantBody string
	}{
		{
			name:      "titles with the saved search that fired",
			digest:    Digest{SavedSearchName: "Backend Engineer", Total: 3},
			wantTitle: "Backend Engineer",
			wantBody:  "3 new jobs",
		},
		{
			// The same Plural the email and Telegram channels have always
			// applied to this identical sentence.
			name:      "one job is singular",
			digest:    Digest{SavedSearchName: "Backend Engineer", Total: 1, Jobs: []DigestJob{{Slug: "a"}}},
			wantTitle: "Backend Engineer",
			wantBody:  "1 new job",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body, _ := renderDigest(tt.digest)
			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
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
