package telegramnotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/strelov1/freehire/internal/notify"
)

func TestNotifier_Render(t *testing.T) {
	n := NewNotifier(NewClient("t"), "https://freehire.dev/")
	d := notify.Digest{
		SavedSearchName: "Go & <remote>",
		Total:           3,
		Jobs: []notify.DigestJob{
			{Title: "Go Dev <x>", Company: "Acme", Slug: "go-dev-acme",
				SalaryMin: 130000, SalaryMax: 170000, SalaryCurrency: "USD", SalaryPeriod: "year"},
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
	// Card: escaped bold title, "at Company" line, salary line, and an Apply link
	// to the freehire job page (trailing slash trimmed) tagged with the telegram UTM.
	for _, want := range []string{
		"💼 <b>Go Dev &lt;x&gt;</b>",
		"🏛️ at Acme",
		"💰 $130K—$170K / year",
		`✅ <a href="https://freehire.dev/jobs/go-dev-acme?utm_source=telegram-bot">Apply →</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("card missing %q in: %q", want, got)
		}
	}
	// A job with no company/salary omits those lines but still renders title + Apply.
	if !strings.Contains(got, "💼 <b>Rustacean</b>") ||
		!strings.Contains(got, `href="https://freehire.dev/jobs/rustacean-foo?utm_source=telegram-bot"`) {
		t.Errorf("company/salary-less card wrong: %q", got)
	}
	if strings.Contains(got, "🏛️ at \n") || strings.Count(got, "💰") != 1 {
		t.Errorf("empty company/salary lines should be omitted: %q", got)
	}
	// Total 3 but only 2 listed → "+ 1 more".
	if !strings.Contains(got, "+ 1 more") {
		t.Errorf("missing overflow summary: %q", got)
	}
}

func TestFormatSalary(t *testing.T) {
	cases := []struct {
		name             string
		min, max         int
		currency, period string
		want             string
	}{
		{"range", 130000, 170000, "USD", "year", "$130K—$170K / year"},
		{"min only", 90000, 0, "EUR", "year", "€90K / year"},
		{"max only", 0, 50000, "GBP", "month", "£50K / month"},
		{"equal bounds collapse", 100000, 100000, "USD", "year", "$100K / year"},
		{"unknown currency is a prefix", 20000, 30000, "PLN", "month", "PLN 20K—PLN 30K / month"},
		{"hourly rate not abbreviated", 50, 80, "USD", "hour", "$50—$80 / hour"},
		{"fractional thousands", 4500, 0, "USD", "", "$4.5K"},
		{"no figure", 0, 0, "USD", "year", ""},
	}
	for _, tc := range cases {
		if got := formatSalary(tc.min, tc.max, tc.currency, tc.period); got != tc.want {
			t.Errorf("%s: formatSalary(%d,%d,%q,%q) = %q, want %q", tc.name, tc.min, tc.max, tc.currency, tc.period, got, tc.want)
		}
	}
}

func TestNotifier_RenderSingularNoOverflow(t *testing.T) {
	n := NewNotifier(NewClient("t"), "https://freehire.dev")
	got := n.render(notify.Digest{SavedSearchName: "x", Total: 1, Jobs: []notify.DigestJob{{Title: "A", Slug: "a"}}})
	if !strings.Contains(got, "<b>1</b> new job for") || strings.Contains(got, "more") {
		t.Errorf("singular render wrong: %q", got)
	}
}

// A digest of many long-title jobs must stay within Telegram's 4096-code-unit
// sendMessage limit — otherwise the send fails deterministically, every retry
// re-fails, and the whole batch is dead-lettered (the user loses all of it). The
// jobs that don't fit fall into the "+ N more" tail, and none are lost.
func TestNotifier_RenderCapsAtTelegramLimit(t *testing.T) {
	n := NewNotifier(NewClient("t"), "https://freehire.dev")
	const total = 20                                                              // DigestCap
	longTitle := strings.Repeat("Senior Staff Platform Reliability Engineer ", 6) // ~258 chars
	jobs := make([]notify.DigestJob, total)
	for i := range jobs {
		jobs[i] = notify.DigestJob{
			Title:   longTitle,
			Company: "A Rather Long Company Name Incorporated",
			Slug:    strings.Repeat("some-long-job-slug-", 5),
		}
	}
	got := n.render(notify.Digest{SavedSearchName: "big search", Total: total, Jobs: jobs})

	if n16 := len(utf16.Encode([]rune(got))); n16 > 4096 {
		t.Errorf("rendered %d UTF-16 units, want <= 4096 (Telegram sendMessage limit)", n16)
	}
	shown := strings.Count(got, "💼 ")
	if shown == 0 || shown >= total {
		t.Errorf("shown = %d, want some-but-not-all of %d jobs listed", shown, total)
	}
	if !strings.Contains(got, "more") {
		t.Errorf("dropped jobs must be summarized by a '+ N more' tail: %q", got)
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
