package emailnotify

import (
	"context"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// SendCandidateMessage delivers an application conversation email. The Reply-To
// is the application's hosted alias, so the candidate can respond from email.
func (m *ApplicationMailer) SendCandidateMessage(ctx context.Context, to, replyTo, title, company, body string) error {
	return m.sender.Send(ctx, Message{
		From: m.from, To: to, ReplyTo: replyTo,
		Subject: company + " on freehire: " + title,
		Text:    body + "\n\nReply to this email to continue the conversation with " + company + ".",
		Group:   emailprefs.GroupEssential,
	})
}
