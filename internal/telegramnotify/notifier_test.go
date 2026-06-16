package telegramnotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/notify"
)

func TestNotifier_Render(t *testing.T) {
	n := NewNotifier(NewClient("t"), "https://freehire.dev/")
	d := notify.Digest{
		SavedSearchName: "Go & <remote>",
		Total:           3,
		Jobs: []notify.DigestJob{
			{Title: "Go Dev <x>", Company: "Acme", Slug: "go-dev-acme"},
			{Title: "Rustacean", Company: "", Slug: "rustacean-foo"},
		},
	}
	got := n.render(d)

	// Heading reflects the true total and pluralizes; the name is HTML-escaped.
	if !strings.Contains(got, "<b>3</b> new jobs for") {
		t.Errorf("missing heading: %q", got)
	}
	if !strings.Contains(got, "Go &amp; &lt;remote&gt;") {
		t.Errorf("saved-search name not HTML-escaped: %q", got)
	}
	// Job title escaped; link points at the freehire job page (trailing slash trimmed).
	if !strings.Contains(got, `<a href="https://freehire.dev/jobs/go-dev-acme">Go Dev &lt;x&gt;</a> — Acme`) {
		t.Errorf("job line wrong: %q", got)
	}
	// A job with no company omits the dash suffix.
	if !strings.Contains(got, `<a href="https://freehire.dev/jobs/rustacean-foo">Rustacean</a>`) {
		t.Errorf("company-less job line wrong: %q", got)
	}
	// Total 3 but only 2 listed → "+ 1 more".
	if !strings.Contains(got, "+ 1 more") {
		t.Errorf("missing overflow summary: %q", got)
	}
}

func TestNotifier_RenderSingularNoOverflow(t *testing.T) {
	n := NewNotifier(NewClient("t"), "https://freehire.dev")
	got := n.render(notify.Digest{SavedSearchName: "x", Total: 1, Jobs: []notify.DigestJob{{Title: "A", Slug: "a"}}})
	if !strings.Contains(got, "<b>1</b> new job for") || strings.Contains(got, "more") {
		t.Errorf("singular render wrong: %q", got)
	}
}

func TestNotifier_Send(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("BOTTOKEN")
	c.base = srv.URL
	n := NewNotifier(c, "https://freehire.dev")

	err := n.Send(context.Background(), notify.ChannelTelegram, "12345", notify.Digest{SavedSearchName: "x", Total: 1, Jobs: []notify.DigestJob{{Title: "A", Slug: "a"}}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/botBOTTOKEN/sendMessage" {
		t.Errorf("path = %q, want /botBOTTOKEN/sendMessage", gotPath)
	}
	if gotBody["chat_id"].(float64) != 12345 {
		t.Errorf("chat_id = %v, want 12345", gotBody["chat_id"])
	}
	if gotBody["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %v, want HTML", gotBody["parse_mode"])
	}
}

func TestNotifier_SendBadChatID(t *testing.T) {
	n := NewNotifier(NewClient("t"), "https://freehire.dev")
	if err := n.Send(context.Background(), notify.ChannelTelegram, "not-a-number", notify.Digest{}); err == nil {
		t.Error("Send with non-numeric dest succeeded, want error")
	}
}

func TestClient_PropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer srv.Close()

	c := NewClient("t")
	c.base = srv.URL
	err := c.SendMessage(context.Background(), 1, "hi")
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("SendMessage err = %v, want it to carry the API description", err)
	}
}

func TestStartToken(t *testing.T) {
	cases := []struct {
		text      string
		wantOK    bool
		wantToken string
	}{
		{"/start abc123", true, "abc123"},
		{"/start   abc123  ", true, "abc123"},
		{"/start", false, ""},
		{"/start ", false, ""},
		{"hello", false, ""},
	}
	for _, tc := range cases {
		u := Update{}
		u.Message = &struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text"`
		}{Text: tc.text}
		u.Message.Chat.ID = 99

		tok, chat, ok := StartToken(u)
		if ok != tc.wantOK || tok != tc.wantToken {
			t.Errorf("StartToken(%q) = (%q,%v), want (%q,%v)", tc.text, tok, ok, tc.wantToken, tc.wantOK)
		}
		if ok && chat != 99 {
			t.Errorf("StartToken(%q) chat = %d, want 99", tc.text, chat)
		}
	}
}

func TestStartToken_NilMessage(t *testing.T) {
	if _, _, ok := StartToken(Update{}); ok {
		t.Error("StartToken(empty update) ok = true, want false")
	}
}
