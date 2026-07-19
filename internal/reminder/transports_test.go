package reminder

import (
	"context"
	"strings"
	"testing"
)

// captureSender records the last email a EmailNotifier sent.
type captureSender struct {
	from, to, subject, html, text string
}

func (s *captureSender) Send(_ context.Context, from, to, subject, html, text string) error {
	s.from, s.to, s.subject, s.html, s.text = from, to, subject, html, text
	return nil
}

func TestEmailNotifier_RendersSubjectAndOnPlatformLink(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.dev", "https://freehire.dev/")
	msg := ReminderMessage{JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme", URL: "https://ats/x"}

	if err := n.Send(context.Background(), "email", "u@x.com", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sender.to != "u@x.com" || sender.from != "jobs@freehire.dev" {
		t.Errorf("envelope from=%q to=%q", sender.from, sender.to)
	}
	if !strings.Contains(sender.subject, "Go Dev") || !strings.Contains(sender.subject, "Acme") {
		t.Errorf("subject = %q, want job + company", sender.subject)
	}
	if !strings.Contains(sender.html, "https://freehire.dev/jobs/go-dev-acme") {
		t.Errorf("html link must point on-platform, got %q", sender.html)
	}
	if strings.Contains(sender.html, "https://ats/x") {
		t.Errorf("must not leak the source URL, got %q", sender.html)
	}
}

func TestEmailNotifier_EscapesUserData(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.dev", "https://freehire.dev")
	msg := ReminderMessage{JobTitle: "<script>x</script>", Company: "Acme", Slug: "s", URL: "u"}

	if err := n.Send(context.Background(), "email", "u@x.com", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if strings.Contains(sender.html, "<script>") {
		t.Errorf("title must be HTML-escaped in the body, got %q", sender.html)
	}
}

func TestTelegramNotifier_RendersOnPlatformLink(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.dev/")
	got := n.render(ReminderMessage{JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"})
	if !strings.Contains(got, "https://freehire.dev/jobs/go-dev-acme") {
		t.Errorf("telegram render missing on-platform link: %q", got)
	}
	if !strings.Contains(got, "Go Dev") || !strings.Contains(got, "Acme") {
		t.Errorf("telegram render missing job/company: %q", got)
	}
}

func TestTelegramNotifier_InvalidChatIDErrors(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.dev")
	if err := n.Send(context.Background(), "telegram", "not-a-number", ReminderMessage{}); err == nil {
		t.Error("want an error for a non-numeric chat id")
	}
}
