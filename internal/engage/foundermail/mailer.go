// Package foundermail is the envelope of the letters the founder writes in the first
// person: onboarding, prowelcome, limitnudge, broadcast and cvlifecycle. Each of those
// owns what its letters say; this owns who they come from and how they go out.
package foundermail

import (
	"context"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// senderName is what the recipient's message list shows: a person, since the letters are
// first-person and invite a reply. "freehire" would invite a reply to a company no one answers.
const senderName = "Ilya from freehire"

// Mailer sends founder letters through a Sender.
type Mailer struct {
	sender  emailnotify.Sender
	from    string
	replyTo string
	links   *emailprefs.Links
}

// New builds a Mailer. from is the verified sending address; replyTo is the human inbox
// that answers. A blank replyTo is a configuration error the caller must catch.
func New(sender emailnotify.Sender, from, replyTo string, links *emailprefs.Links) *Mailer {
	return &Mailer{
		sender:  sender,
		from:    emailnotify.From(senderName, from),
		replyTo: replyTo,
		links:   links,
	}
}

// Unsubscribe signs the one-click unsubscribe link every founder letter carries.
func (m *Mailer) Unsubscribe(userID int64) (string, error) {
	return m.links.For(userID, emailprefs.GroupNews)
}

// Send delivers one letter to one recipient. msg carries Subject, HTML, Text and any
// Campaign/Variant tag; the envelope around them is filled in here. unsubscribe is what
// Unsubscribe returned, since the body had to be rendered around it before this call.
func (m *Mailer) Send(ctx context.Context, to, unsubscribe string, msg emailnotify.Message) error {
	msg.From, msg.To, msg.ReplyTo = m.from, to, m.replyTo
	msg.Group, msg.UnsubscribeURL = emailprefs.GroupNews, unsubscribe
	return m.sender.Send(ctx, msg)
}
