package referral

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// requestHTML is the mail body; its dot is the cabinet URL. The notice stays
// deliberately contentless — who is asking, and about what, lives behind
// authorization in the cabinet, not in an inbox.
var requestHTML = template.Must(mailtpl.Partials().New("referralRequest").Parse(`
{{template "p" "A job seeker asked you for a referral. The details are in your inbox."}}
{{template "button" (mailLink . "View the request")}}`))

// EmailSender is the slice of the SES transport the pinger needs; *emailnotify.Client
// satisfies it. Kept local so the referral package does not depend on the email package.
type EmailSender interface {
	Send(ctx context.Context, m emailnotify.Message) error
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
	email  EmailSender
	from   string
	tg     TelegramSender
	layout *mailtpl.Layout
	links  *emailprefs.Links
}

// NewChannelPinger builds a ChannelPinger. A nil email sender disables the email channel
// (e.g. SES unconfigured) and a nil Telegram sender disables Telegram; a referrer with no
// enabled channel still sees the request in-cabinet. baseURL is the site origin the mail's
// branded shell links back to — the cabinet link itself arrives per-call, because it points
// at one specific request.
func NewChannelPinger(email EmailSender, from string, tg TelegramSender, baseURL string, links *emailprefs.Links) *ChannelPinger {
	return &ChannelPinger{email: email, from: from, tg: tg, layout: mailtpl.New(baseURL), links: links}
}

// PingReferrer sends the notice over every enabled channel the recipient can receive, joining
// any per-channel failures so the caller can log them.
func (p *ChannelPinger) PingReferrer(ctx context.Context, r Recipient, cabinetURL string) error {
	link := html.EscapeString(cabinetURL)
	var errs []error

	if p.email != nil && r.Email != "" {
		var content bytes.Buffer
		// Trusted template, data escaped in context: a failure here is a template bug,
		// caught by the golden previews.
		_ = requestHTML.Execute(&content, cabinetURL)

		// GroupActivity, not GroupNews. A referral request is somebody asking THIS
		// person for something, arriving because they offered to be asked — filing it
		// with campaigns would mean declining our product news silently switches off
		// the marketplace they signed up for.
		unsubscribe, err := p.links.For(r.UserID, emailprefs.GroupActivity)
		if err != nil {
			return fmt.Errorf("referral: unsubscribe link for user %d: %w", r.UserID, err)
		}
		htmlBody := p.layout.Render(mailtpl.Body{
			Preheader:      "Someone asked you for a referral",
			Heading:        "New referral request",
			Content:        template.HTML(content.String()), //nolint:gosec // rendered by the trusted template below
			Footer:         "You’re getting this because you offered to refer people at your company.",
			UnsubscribeURL: unsubscribe,
		})
		textBody := "A job seeker asked for a referral. View the request in your inbox: " + cabinetURL +
			"\n" + emailprefs.TextFooter(unsubscribe)
		if err := p.email.Send(ctx, emailnotify.Message{
			From: p.from, To: r.Email, Subject: "New referral request on freehire",
			HTML: htmlBody, Text: textBody,
			Group: emailprefs.GroupActivity, UnsubscribeURL: unsubscribe,
		}); err != nil {
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
