package emailnotify

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

type messageRecorder struct{ got []Message }

func (r *messageRecorder) Send(_ context.Context, m Message) error {
	r.got = append(r.got, m)
	return nil
}

func TestApplicationMailer_DeliversTheApplicationToTheEmployer(t *testing.T) {
	rec := &messageRecorder{}
	m := NewApplicationMailer(rec, "no-reply@freehire.me", "https://freehire.me")

	err := m.SendApplication(context.Background(), ApplicationMail{
		To:             "hr@acme.test",
		CandidateEmail: "c@example.test",
		CompanyName:    "Acme",
		JobTitle:       "Platform <Engineer>",
		JobURL:         "https://freehire.me/jobs/platform-engineer-acme",
		Answers:        []ApplicationAnswer{{Question: "EU work permit?", Answer: "Yes"}},
		Note:           "Happy to talk.",
		CV:             []byte("%PDF-1.7"),
		CVFilename:     "cv.pdf",
		CVContentType:  "application/pdf",
	})
	if err != nil {
		t.Fatalf("SendApplication: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("sends = %d, want 1", len(rec.got))
	}
	msg := rec.got[0]
	if msg.To != "hr@acme.test" || msg.ReplyTo != "c@example.test" {
		t.Errorf("to/reply-to = %q/%q, want the employer and the candidate", msg.To, msg.ReplyTo)
	}
	if msg.Group != emailprefs.GroupEssential || msg.UnsubscribeURL != "" {
		t.Errorf("group/unsubscribe = %q/%q, want essential with no unsubscribe", msg.Group, msg.UnsubscribeURL)
	}
	if !strings.Contains(msg.Subject, "Platform <Engineer>") {
		t.Errorf("subject = %q, want the vacancy named", msg.Subject)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].ContentType != "application/pdf" || msg.Attachments[0].Filename != "cv.pdf" || string(msg.Attachments[0].Content) != "%PDF-1.7" {
		t.Errorf("attachments = %+v, want the CV as one PDF", msg.Attachments)
	}
	for _, part := range []string{msg.HTML, msg.Text} {
		for _, want := range []string{"EU work permit?", "Yes", "Happy to talk.", "https://freehire.me/jobs/platform-engineer-acme"} {
			if !strings.Contains(part, want) {
				t.Errorf("body lacks %q:\n%s", want, part)
			}
		}
	}
	if strings.Contains(msg.HTML, "<Engineer>") {
		t.Error("the HTML body carries the job title unescaped")
	}
}
