package emailnotify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

func TestTeamInviteMailer_SendsTheLink(t *testing.T) {
	rec := &messageRecorder{}
	m := NewTeamInviteMailer(rec, "no-reply@freehire.me", "https://freehire.me")
	err := m.SendTeamInvitation(context.Background(), TeamInvitation{
		To:          "dev@acme.test",
		CompanyName: "Acme <Corp>",
		InvitedBy:   "hr@acme.test",
		Link:        "https://freehire.me/my/employer/join?token=abc",
		ExpiresAt:   time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("SendTeamInvitation: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("sends = %d", len(rec.got))
	}
	msg := rec.got[0]
	if msg.To != "dev@acme.test" || msg.ReplyTo != "hr@acme.test" {
		t.Errorf("to/reply-to = %q/%q", msg.To, msg.ReplyTo)
	}
	if msg.Group != emailprefs.GroupEssential || msg.UnsubscribeURL != "" {
		t.Errorf("group/unsubscribe = %q/%q", msg.Group, msg.UnsubscribeURL)
	}
	for _, part := range []string{msg.HTML, msg.Text} {
		for _, want := range []string{"https://freehire.me/my/employer/join?token=abc", "hr@acme.test", "21 November 2026"} {
			if !strings.Contains(part, want) {
				t.Errorf("body lacks %q:\n%s", want, part)
			}
		}
	}
	if strings.Contains(msg.HTML, "<Corp>") {
		t.Error("the company name reached the HTML unescaped")
	}
}
