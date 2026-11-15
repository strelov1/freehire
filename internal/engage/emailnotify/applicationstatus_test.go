package emailnotify

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

func TestApplicationMailer_TellsTheCandidateWhatTheEmployerDecided(t *testing.T) {
	rec := &messageRecorder{}
	m := NewApplicationMailer(rec, "no-reply@freehire.me", "https://freehire.me")

	err := m.SendStatusUpdate(context.Background(), ApplicationStatusMail{
		To:          "c@example.test",
		ReplyTo:     "hr@acme.test",
		CompanyName: "Acme <Corp>",
		JobTitle:    "Platform Engineer",
		JobURL:      "https://freehire.me/jobs/platform-engineer-acme",
		Stage:       "rejected",
		Message:     "We went with someone closer to our stack.",
	})
	if err != nil {
		t.Fatalf("SendStatusUpdate: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("sends = %d, want 1", len(rec.got))
	}
	msg := rec.got[0]
	if msg.To != "c@example.test" || msg.ReplyTo != "hr@acme.test" {
		t.Errorf("to/reply-to = %q/%q, want the candidate and the employer", msg.To, msg.ReplyTo)
	}
	if msg.Group != emailprefs.GroupEssential || msg.UnsubscribeURL != "" {
		t.Errorf("group/unsubscribe = %q/%q, want essential with no unsubscribe", msg.Group, msg.UnsubscribeURL)
	}
	if !strings.Contains(msg.Subject, "Acme <Corp>") {
		t.Errorf("subject = %q, want the company named", msg.Subject)
	}
	for _, part := range []string{msg.HTML, msg.Text} {
		for _, want := range []string{"not to move forward", "We went with someone closer to our stack.", "https://freehire.me/jobs/platform-engineer-acme"} {
			if !strings.Contains(part, want) {
				t.Errorf("body lacks %q:\n%s", want, part)
			}
		}
	}
	if strings.Contains(msg.HTML, "<Corp>") {
		t.Error("the company name reached the HTML unescaped")
	}
}

func TestApplicationMailer_RefusesAStageWithNoLetter(t *testing.T) {
	rec := &messageRecorder{}
	m := NewApplicationMailer(rec, "no-reply@freehire.me", "https://freehire.me")
	if err := m.SendStatusUpdate(context.Background(), ApplicationStatusMail{To: "c@example.test", Stage: "reviewing"}); err == nil {
		t.Error("a stage with no letter was sent")
	}
	if len(rec.got) != 0 {
		t.Error("something was sent")
	}
}
