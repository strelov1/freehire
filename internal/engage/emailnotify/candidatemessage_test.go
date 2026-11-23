package emailnotify

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

type messageCapture struct{ got Message }

func (c *messageCapture) Send(_ context.Context, m Message) error { c.got = m; return nil }

func TestSendCandidateMessageSetsReplyAddress(t *testing.T) {
	c := &messageCapture{}
	m := NewApplicationMailer(c, "noreply@freehire.test", "https://freehire.test")
	if err := m.SendCandidateMessage(context.Background(), "c@example.test", "ats-reply-x@mail.test", "Engineer", "Acme", "Hello"); err != nil {
		t.Fatal(err)
	}
	if c.got.To != "c@example.test" || c.got.ReplyTo != "ats-reply-x@mail.test" || c.got.Group != emailprefs.GroupEssential {
		t.Errorf("message = %+v", c.got)
	}
	if !strings.Contains(c.got.Subject, "Acme") || !strings.Contains(c.got.Subject, "Engineer") || !strings.HasPrefix(c.got.Text, "Hello") {
		t.Errorf("subject/text = %q / %q", c.got.Subject, c.got.Text)
	}
}
