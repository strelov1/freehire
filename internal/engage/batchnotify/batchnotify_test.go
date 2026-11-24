package batchnotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
)

type msg struct{ UserID int64 }

func render(ms []msg) string { return "body" }

// --- Telegram ---------------------------------------------------------------

func TestTelegram_SendsTheRenderedBatchToTheChat(t *testing.T) {
	var gotPath string
	var got struct {
		ChatID float64 `json:"chat_id"`
		Text   string  `json:"text"`
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()

	n := NewTelegram(telegramnotify.NewClientWithBase("t", api.URL), render)
	if err := n.Send(context.Background(), notify.ChannelTelegram, "42", []msg{{7}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/bott/sendMessage" || got.ChatID != 42 || got.Text != "body" {
		t.Errorf("posted %q to chat %v with text %q, want /bott/sendMessage, 42, the rendered body", gotPath, got.ChatID, got.Text)
	}
}

func TestTelegram_EmptyBatchIsNotSent(t *testing.T) {
	n := NewTelegram(nil, render) // a nil client would panic on a send
	if err := n.Send(context.Background(), notify.ChannelTelegram, "42", nil); err != nil {
		t.Fatalf("Send of an empty batch: %v", err)
	}
}

func TestTelegram_BlockedChatIsRecipientGone(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
	}))
	defer api.Close()

	n := NewTelegram(telegramnotify.NewClientWithBase("t", api.URL), render)
	err := n.Send(context.Background(), notify.ChannelTelegram, "42", []msg{{7}})
	if !errors.Is(err, notify.ErrRecipientGone) {
		t.Fatalf("Send on 403 = %v, want notify.ErrRecipientGone", err)
	}
}

// --- Email ------------------------------------------------------------------

type sender struct {
	sent []emailnotify.Message
	err  error
}

func (s *sender) Send(_ context.Context, m emailnotify.Message) error {
	s.sent = append(s.sent, m)
	return s.err
}

func TestEmail_SendsOneMailWithSignedUnsubscribeLinkAndFooter(t *testing.T) {
	s := &sender{}
	n := NewEmail(s, "jobs@freehire.me", emailprefs.NewLinks("mail-test-secret-padded-to-32-byte", "https://freehire.me"), "reminder",
		func(ms []msg) int64 { return ms[0].UserID },
		func(ms []msg, unsubscribe string) (subject, htmlBody, textBody string) {
			return "s", "<p>" + unsubscribe + "</p>", "text"
		})

	if err := n.Send(context.Background(), notify.ChannelEmail, "u@x.com", []msg{{7}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(s.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(s.sent))
	}
	m := s.sent[0]
	if m.To != "u@x.com" || m.Subject != "s" || m.From != `"freehire" <jobs@freehire.me>` {
		t.Errorf("envelope = to %q, from %q, subject %q", m.To, m.From, m.Subject)
	}
	if !strings.HasPrefix(m.UnsubscribeURL, "https://freehire.me/") || !strings.Contains(m.HTML, m.UnsubscribeURL) {
		t.Errorf("renderer did not get the signed link %q", m.UnsubscribeURL)
	}
	if want := "text" + emailprefs.TextFooter(m.UnsubscribeURL); m.Text != want {
		t.Errorf("Text = %q, want the rendered body plus the unsubscribe footer", m.Text)
	}
}

func TestEmail_EmptyBatchIsNotSent(t *testing.T) {
	s := &sender{}
	n := NewEmail(s, "jobs@freehire.me", emailprefs.NewLinks("mail-test-secret-padded-to-32-byte", "https://freehire.me"),
		"nudge", func(ms []msg) int64 { return ms[0].UserID }, nil)
	if err := n.Send(context.Background(), notify.ChannelEmail, "u@x.com", nil); err != nil {
		t.Fatalf("Send of an empty batch: %v", err)
	}
	if len(s.sent) != 0 {
		t.Errorf("sent %d mails for an empty batch, want 0", len(s.sent))
	}
}

func TestEmail_PausedSESSoftSkipsAndKeepsTheCause(t *testing.T) {
	s := &sender{err: emailnotify.ErrSendingPaused}
	n := NewEmail(s, "jobs@freehire.me", emailprefs.NewLinks("mail-test-secret-padded-to-32-byte", "https://freehire.me"),
		"nudge", func(ms []msg) int64 { return ms[0].UserID },
		func([]msg, string) (string, string, string) { return "", "", "" })

	err := n.Send(context.Background(), notify.ChannelEmail, "u@x.com", []msg{{7}})
	if !errors.Is(err, notify.ErrChannelNotConfigured) || !errors.Is(err, emailnotify.ErrSendingPaused) {
		t.Fatalf("Send while paused = %v, want ErrChannelNotConfigured wrapping ErrSendingPaused", err)
	}
}

// The engine name prefixes the error, as every worker's does, and the cause stays
// inspectable: the engines' run summaries group failures by it.
func TestEmail_UnsignableUnsubscribeLinkFailsBeforeSending(t *testing.T) {
	s := &sender{}
	links := emailprefs.NewLinks("too-short", "https://freehire.me") // no usable secret
	n := NewEmail(s, "jobs@freehire.me", links, "reminder", func(ms []msg) int64 { return ms[0].UserID }, nil)

	err := n.Send(context.Background(), notify.ChannelEmail, "u@x.com", []msg{{7}})
	if err == nil || !strings.HasPrefix(err.Error(), "reminder: unsubscribe link for user 7: ") || !errors.Is(err, emailprefs.ErrCannotMint) {
		t.Fatalf("Send = %v, want the engine-prefixed unsubscribe error wrapping ErrCannotMint", err)
	}
	if len(s.sent) != 0 {
		t.Errorf("sent %d mails without a working unsubscribe link, want 0", len(s.sent))
	}
}

// --- UnlinkTelegram ---------------------------------------------------------

type unlinker struct {
	rows    int64
	err     error
	deleted []int64
}

func (u *unlinker) DeleteTelegramLink(_ context.Context, userID int64) (int64, error) {
	u.deleted = append(u.deleted, userID)
	return u.rows, u.err
}

// UnlinkTelegram is fire-and-forget: it asks the store once for the user and
// returns whether or not the store succeeds, since the outcome is only logged.
func TestUnlinkTelegram_AskTheStoreOnceAndSurvivesItsFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows int64
		err  error
	}{
		{name: "a link was removed", rows: 1},
		{name: "nothing was linked", rows: 0},
		{name: "the store fails", err: errors.New("boom")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &unlinker{rows: tc.rows, err: tc.err}
			UnlinkTelegram(context.Background(), u, "reminder", 42, errors.New("gone"))
			if len(u.deleted) != 1 || u.deleted[0] != 42 {
				t.Errorf("DeleteTelegramLink calls = %v, want [42]", u.deleted)
			}
		})
	}
}
