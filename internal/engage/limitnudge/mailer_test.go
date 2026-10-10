package limitnudge

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// fakeSender captures what the mailer hands to the transport, mirroring
// prowelcome's own mailer_test.go pattern.
type fakeSender struct {
	from, to, subject, html, text, replyTo string
	group                                  emailprefs.Group
	unsubscribeURL                         string
	calls                                  int
	err                                    error
}

func (s *fakeSender) Send(_ context.Context, m emailnotify.Message) error {
	s.calls++
	s.from, s.to, s.subject, s.html, s.text, s.replyTo = m.From, m.To, m.Subject, m.HTML, m.Text, m.ReplyTo
	s.group, s.unsubscribeURL = m.Group, m.UnsubscribeURL
	return s.err
}

const nudgeUserID int64 = 42

func testLinks() *emailprefs.Links {
	return emailprefs.NewLinks("limitnudge-test-secret-padded-to-32b", "https://freehire.me")
}

func newMailer(sender Sender) *Mailer {
	return NewMailer(sender, "billing@freehire.me", "ilya@freehire.me", "https://freehire.me", testLinks())
}

func TestMailer_RenderLinksPricing(t *testing.T) {
	mail, err := newMailer(&fakeSender{}).render("unsubscribe-token")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	const wantURL = "https://freehire.me/pricing?utm_source=email"
	if !strings.Contains(mail.html, wantURL) {
		t.Errorf("html does not link the pricing page (%s)", wantURL)
	}
	if !strings.Contains(mail.text, wantURL) {
		t.Errorf("text does not link the pricing page (%s)", wantURL)
	}
}

func TestMailer_RenderLinksLinkedIn(t *testing.T) {
	mail, err := newMailer(&fakeSender{}).render("tok")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(mail.html, mailtpl.LinkedInURL) {
		t.Error("html does not link the founder's LinkedIn profile")
	}
}

func TestMailer_SendUsesHumanReplyTo(t *testing.T) {
	sender := &fakeSender{}
	m := newMailer(sender)

	if err := m.Send(context.Background(), nudgeUserID, "capped-user@example.test"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender called %d times, want 1", sender.calls)
	}
	if sender.to != "capped-user@example.test" {
		t.Errorf("to = %q, want the account's address", sender.to)
	}
	if sender.replyTo != "ilya@freehire.me" {
		t.Errorf("reply-to = %q, want the configured human inbox, not the sending address", sender.replyTo)
	}
	if sender.group != emailprefs.GroupNews {
		t.Errorf("group = %q, want %q", sender.group, emailprefs.GroupNews)
	}
	if sender.unsubscribeURL == "" {
		t.Error("unsubscribe URL is empty")
	}
}
