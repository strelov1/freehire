package referral

import (
	"context"
	"errors"
	"fmt"
	"html"
)

// EmailSender is the slice of the SES transport the pinger needs; *emailnotify.Client
// satisfies it. Kept local so the referral package does not depend on the email package.
type EmailSender interface {
	Send(ctx context.Context, from, to, subject, htmlBody, textBody string) error
}

// TelegramSender is the slice of the Telegram client the pinger needs;
// *telegramnotify.Client satisfies it.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, html string) error
}

// Compile-time proof that ChannelPinger satisfies Pinger.
var _ Pinger = (*ChannelPinger)(nil)

// ChannelPinger is the production Pinger: it emails every referrer (email is always
// present) and additionally messages Telegram when the referrer linked it. The notice is
// deliberately minimal — "you have a new referral request" plus a link to the cabinet inbox,
// where the seeker's contact and CV live behind authorization — so nothing leaks over the
// channel itself.
type ChannelPinger struct {
	email EmailSender
	from  string
	tg    TelegramSender
}

// NewChannelPinger builds a ChannelPinger. A nil email sender disables the email channel
// (e.g. SES unconfigured) and a nil Telegram sender disables Telegram; a referrer with no
// enabled channel still sees the request in-cabinet.
func NewChannelPinger(email EmailSender, from string, tg TelegramSender) *ChannelPinger {
	return &ChannelPinger{email: email, from: from, tg: tg}
}

// PingReferrer sends the notice over every enabled channel the recipient can receive, joining
// any per-channel failures so the caller can log them.
func (p *ChannelPinger) PingReferrer(ctx context.Context, r Recipient, cabinetURL string) error {
	link := html.EscapeString(cabinetURL)
	var errs []error

	if p.email != nil && r.Email != "" {
		subject := "New referral request on freehire"
		htmlBody := fmt.Sprintf(
			`<p>A job seeker asked for a referral. <a href="%s">View the request</a> in your inbox.</p>`, link)
		textBody := "A job seeker asked for a referral. View the request in your inbox: " + cabinetURL
		if err := p.email.Send(ctx, p.from, r.Email, subject, htmlBody, textBody); err != nil {
			errs = append(errs, err)
		}
	}

	if p.tg != nil && r.ChatID != 0 {
		msg := fmt.Sprintf(
			`You have a new <b>referral request</b>. <a href="%s">Open it in your inbox</a>.`, link)
		if err := p.tg.SendMessage(ctx, r.ChatID, msg); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
