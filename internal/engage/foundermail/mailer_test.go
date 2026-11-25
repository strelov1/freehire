package foundermail

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

type captureSender struct {
	sent []emailnotify.Message
	err  error
}

func (s *captureSender) Send(_ context.Context, m emailnotify.Message) error {
	s.sent = append(s.sent, m)
	return s.err
}

func newMailer(sender emailnotify.Sender) *Mailer {
	return New(sender, "notifications@freehire.me", "ilya@freehire.me", emailprefs.NewLinks("foundermail-test-secret-padded-to-3", "https://freehire.me"))
}

func TestUnsubscribeSignsTheNewsGroupLink(t *testing.T) {
	link, err := newMailer(&captureSender{}).Unsubscribe(42)
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if !strings.HasPrefix(link, "https://freehire.me/") {
		t.Errorf("unsubscribe link %q is not rooted at the site", link)
	}
}

func TestUnsubscribeFailsWithoutASigningSecret(t *testing.T) {
	m := New(&captureSender{}, "notifications@freehire.me", "ilya@freehire.me", emailprefs.NewLinks("short", "https://freehire.me"))
	if _, err := m.Unsubscribe(42); err == nil {
		t.Fatal("Unsubscribe succeeded with a secret too short to sign")
	}
}

func TestSendFillsTheEnvelopeAndKeepsTheBody(t *testing.T) {
	sender := &captureSender{}
	unsubscribe, err := newMailer(sender).Unsubscribe(42)
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	err = newMailer(sender).Send(context.Background(), "reader@example.test", unsubscribe, emailnotify.Message{
		Subject: "Hello", HTML: "<p>Hello</p>", Text: "Hello",
		Campaign: "a-campaign", Variant: "b",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sent))
	}
	m := sender.sent[0]
	if m.To != "reader@example.test" || m.ReplyTo != "ilya@freehire.me" {
		t.Errorf("To/ReplyTo = %q / %q, want the recipient and the human inbox", m.To, m.ReplyTo)
	}
	if !strings.Contains(m.From, "Ilya from freehire") || !strings.Contains(m.From, "notifications@freehire.me") {
		t.Errorf("From = %q, want a person's name over the sending address", m.From)
	}
	if m.Group != emailprefs.GroupNews || m.UnsubscribeURL != unsubscribe {
		t.Errorf("Group/UnsubscribeURL = %q / %q, want news and the link the body was rendered with", m.Group, m.UnsubscribeURL)
	}
	if m.Subject != "Hello" || m.HTML != "<p>Hello</p>" || m.Text != "Hello" {
		t.Errorf("the body was altered: %q / %q / %q", m.Subject, m.HTML, m.Text)
	}
	if m.Campaign != "a-campaign" || m.Variant != "b" {
		t.Errorf("Campaign/Variant = %q / %q, want them passed through", m.Campaign, m.Variant)
	}
}

func TestSendPropagatesTransportErrors(t *testing.T) {
	boom := errors.New("ses: down")
	err := newMailer(&captureSender{err: boom}).Send(context.Background(), "x@example.test", "https://freehire.me/u", emailnotify.Message{Subject: "s"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the transport error", err)
	}
}
