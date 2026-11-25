package messaging

import (
	"context"
	"html/template"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/mailtpl"
	"github.com/strelov1/freehire/internal/platform/db"
)

type noticeSender struct{ calls int }

func (s *noticeSender) Send(context.Context, emailnotify.Message) error {
	s.calls++
	return nil
}

// A notice whose body failed to render must not be mailed: the channel reports
// the error, so the claim is released and the next run retries.
func TestNotifier_RenderErrorSendsNoEmail(t *testing.T) {
	previous := noticeHTML
	t.Cleanup(func() { noticeHTML = previous })
	noticeHTML = template.Must(mailtpl.Partials().New("messageUnread").Parse(`{{template "no-such-partial" .}}`))

	s := &noticeSender{}
	n := NewNotifier(nil, NotifierConfig{
		Email: s, From: "noreply@example.test",
		Links: emailprefs.NewLinks("test-secret-test-secret-test-secret-0123", "https://freehire.test"), Origin: "https://freehire.test",
	})
	err := n.sendEmail(context.Background(), db.ListUnreadDueRow{UserID: 7}, Notice{Title: "t", Line: "l"}, "https://freehire.test/x")
	if err == nil || !strings.Contains(err.Error(), "no-such-partial") {
		t.Fatalf("sendEmail = %v, want the template error", err)
	}
	if s.calls != 0 {
		t.Errorf("email sent %d times, want 0", s.calls)
	}
}
