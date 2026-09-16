package prowelcome

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// fakeSender captures what the mailer hands to the transport, mirroring
// emailnotify's own notifier_test.go pattern.
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

const welcomeUserID int64 = 42

func testLinks() *emailprefs.Links {
	return emailprefs.NewLinks("prowelcome-test-secret-padded-to-32b", "https://freehire.me")
}

func newMailer(sender Sender) *Mailer {
	return NewMailer(sender, "billing@freehire.me", "ilya@freehire.me", "https://freehire.me", testLinks())
}

func TestMailer_RenderMentionsTier(t *testing.T) {
	until := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		tier      plan.Tier
		wantLabel string
	}{
		{plan.TierPro, "Pro"},
		{plan.TierUltra, "Ultra"},
	} {
		mail, err := newMailer(&fakeSender{}).render(tc.tier, until, "unsubscribe-token")
		if err != nil {
			t.Fatalf("render(%s): %v", tc.tier, err)
		}
		if !strings.Contains(mail.subject, tc.wantLabel) {
			t.Errorf("%s subject = %q, want it to mention %q", tc.tier, mail.subject, tc.wantLabel)
		}
		if !strings.Contains(mail.html, tc.wantLabel) {
			t.Errorf("%s html does not mention %q", tc.tier, tc.wantLabel)
		}
		if !strings.Contains(mail.text, tc.wantLabel) {
			t.Errorf("%s text does not mention %q", tc.tier, tc.wantLabel)
		}
		if !strings.Contains(mail.html, "October 15, 2026") {
			t.Errorf("%s html does not mention the entitlement's end date", tc.tier)
		}
	}
}

func TestMailer_RenderRefusesFree(t *testing.T) {
	if _, err := newMailer(&fakeSender{}).render(plan.TierFree, time.Now(), "tok"); err == nil {
		t.Error("render(TierFree) = nil error, want a refusal — this mail is for a paying tier only")
	}
}

func TestMailer_RenderLinksDiscordAndLinkedIn(t *testing.T) {
	mail, err := newMailer(&fakeSender{}).render(plan.TierPro, time.Now().Add(30*24*time.Hour), "tok")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(mail.html, mailtpl.DiscordURL) {
		t.Error("html does not link the Discord community")
	}
	if !strings.Contains(mail.html, mailtpl.LinkedInURL) {
		t.Error("html does not link the founder's LinkedIn profile")
	}
}

func TestMailer_RenderExplainsHowToJoinTheProChat(t *testing.T) {
	mail, err := newMailer(&fakeSender{}).render(plan.TierPro, time.Now().Add(30*24*time.Hour), "tok")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	const wantURL = "https://freehire.me/my/integrations?utm_source=email"
	if !strings.Contains(mail.html, wantURL) {
		t.Errorf("html does not link the integrations page (%s) for connecting Discord", wantURL)
	}
	if !strings.Contains(mail.text, wantURL) {
		t.Errorf("text does not link the integrations page (%s) for connecting Discord", wantURL)
	}
}

func TestMailer_SendUsesHumanReplyTo(t *testing.T) {
	sender := &fakeSender{}
	m := newMailer(sender)

	if err := m.Send(context.Background(), welcomeUserID, "new-subscriber@example.test", plan.TierPro, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender called %d times, want 1", sender.calls)
	}
	if sender.to != "new-subscriber@example.test" {
		t.Errorf("to = %q, want the subscriber's address", sender.to)
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

func TestMailer_SendRefusesFree(t *testing.T) {
	sender := &fakeSender{}
	if err := newMailer(sender).Send(context.Background(), welcomeUserID, "x@example.test", plan.TierFree, time.Now()); err == nil {
		t.Error("Send(TierFree) = nil error, want a refusal")
	}
	if sender.calls != 0 {
		t.Errorf("sender called %d times, want 0 for a refused send", sender.calls)
	}
}
