// Package batchnotify holds the two channel wrappers the reminder and nudge
// engines share: posting a batch to a Telegram chat, and rendering it as one
// email. They cannot live in internal/engage/notify, because telegramnotify and
// emailnotify implement notify.Notifier and would import it back.
//
// An engine's Notifier is notify.Notifier[[]ItsMessage]: one call carries one
// account's whole batch. Each engine keeps its own message type and copy and
// passes a render callback; what is identical — the chat post, the unsubscribe
// link, the mail envelope — is here once. An engine's message carries its own
// group key (a nudge's Kind), so the renderer reads it off the first message.
package batchnotify

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/mailtpl"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
)

// Telegram delivers a batch as ONE Telegram HTML message. A 403 (the bot was
// blocked or removed) comes back as notify.ErrRecipientGone from
// telegramnotify.SendToChat, so the engine unlinks the chat instead of retrying a
// send that cannot land.
type Telegram[T any] struct {
	client *telegramnotify.Client
	origin string
	render func(ms []T) string
}

// NewTelegram builds a Telegram notifier sending through client, with links
// rooted at origin (the frontend origin). render produces the HTML body for the
// whole batch.
func NewTelegram[T any](client *telegramnotify.Client, origin string, render func(ms []T) string) *Telegram[T] {
	return &Telegram[T]{client: client, origin: strings.TrimRight(origin, "/"), render: render}
}

// Origin is the frontend origin without a trailing slash, for the engine's renderer.
func (n *Telegram[T]) Origin() string { return n.origin }

// Send posts the rendered batch to the chat encoded in dest. The channel argument
// is ignored.
func (n *Telegram[T]) Send(ctx context.Context, _ string, dest string, ms []T) error {
	if len(ms) == 0 {
		return nil
	}
	return n.client.SendToChat(ctx, dest, n.render(ms))
}

// Email delivers a batch as one email through the SES sender.
type Email[T any] struct {
	sender emailnotify.Sender
	from   string
	origin string
	layout *mailtpl.Layout
	links  *emailprefs.Links
	render func(ms []T, unsubscribe string) (subject, htmlBody, textBody string)
	userID func(ms []T) int64
	engine string
}

// senderName is the display name on these mails: the product, not a person.
const senderName = "freehire"

// NewEmail builds an Email notifier sending from `from` through sender, with links
// rooted at origin and unsubscribe links signed by links. render produces the
// subject, HTML and text of the mail for the whole batch. userID reads whose mail
// it is off the batch: every message is the same account's, since the engines
// group by account. engine prefixes the error line, as every worker's does.
func NewEmail[T any](sender emailnotify.Sender, from, origin string, links *emailprefs.Links, engine string,
	userID func(ms []T) int64, render func(ms []T, unsubscribe string) (subject, htmlBody, textBody string)) *Email[T] {
	base := strings.TrimRight(origin, "/")
	return &Email[T]{
		sender: sender,
		from:   emailnotify.From(senderName, from),
		origin: base,
		layout: mailtpl.New(base),
		links:  links,
		render: render,
		userID: userID,
		engine: engine,
	}
}

// Layout is the shared mail shell the engine's renderer needs.
func (n *Email[T]) Layout() *mailtpl.Layout { return n.layout }

// Origin is the frontend origin without a trailing slash.
func (n *Email[T]) Origin() string { return n.origin }

// Send signs the unsubscribe link, renders the mail and delivers it to the
// address in dest. A paused SES account soft-skips as
// notify.ErrChannelNotConfigured rather than counting an attempt.
func (n *Email[T]) Send(ctx context.Context, _ string, dest string, ms []T) error {
	if len(ms) == 0 {
		return nil
	}
	uid := n.userID(ms)
	unsubscribe, err := n.links.For(uid, emailprefs.GroupActivity)
	if err != nil {
		return fmt.Errorf("%s: unsubscribe link for user %d: %w", n.engine, uid, err)
	}
	subject, htmlBody, textBody := n.render(ms, unsubscribe)
	return emailnotify.SoftSkipIfPaused(n.sender.Send(ctx, emailnotify.Message{
		From: n.from, To: dest, Subject: subject, HTML: htmlBody,
		Text:  textBody + emailprefs.TextFooter(unsubscribe),
		Group: emailprefs.GroupActivity, UnsubscribeURL: unsubscribe,
	}), notify.ErrChannelNotConfigured)
}

// TelegramUnlinker is the one store method UnlinkTelegram needs.
type TelegramUnlinker interface {
	DeleteTelegramLink(ctx context.Context, userID int64) (int64, error)
}

// UnlinkTelegram forgets a user's Telegram chat after a send reported it
// permanently closed, logging it because this changes the user's settings without
// them asking. A failure is only logged: the next delivery meets the same 403 and
// tries again.
func UnlinkTelegram(ctx context.Context, store TelegramUnlinker, engine string, userID int64, cause error) {
	rows, err := store.DeleteTelegramLink(ctx, userID)
	if err != nil {
		log.Printf("%s: unlink telegram for user %d: %v", engine, userID, err)
		return
	}
	if rows > 0 {
		log.Printf("%s: unlinked telegram for user %d: %v", engine, userID, cause)
	}
}
