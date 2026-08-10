package nudge

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

// Compile-time proof the transports satisfy the engine's Notifier seam.
var (
	_ Notifier = (*TelegramNotifier)(nil)
	_ Notifier = (*EmailNotifier)(nil)
)

// TelegramNotifier delivers a nudge as a Telegram HTML message, reusing the
// telegramnotify Bot API client. The link points at the caller's tracking board so
// the nudge keeps the user on freehire.
type TelegramNotifier struct {
	client      *telegramnotify.Client
	trackingURL string
}

// NewTelegramNotifier builds a TelegramNotifier sending through client, with the
// tracking link rooted at jobBaseURL (the frontend origin).
func NewTelegramNotifier(client *telegramnotify.Client, jobBaseURL string) *TelegramNotifier {
	return &TelegramNotifier{client: client, trackingURL: strings.TrimRight(jobBaseURL, "/") + "/my/tracking"}
}

// Send renders the nudge and posts it to the chat encoded in dest. The channel
// argument is ignored — this notifier only serves the telegram channel.
func (n *TelegramNotifier) Send(ctx context.Context, _ string, dest string, m Message) error {
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("nudge: invalid telegram chat id %q: %w", dest, err)
	}
	return n.client.SendMessage(ctx, chatID, n.render(m))
}

func (n *TelegramNotifier) render(m Message) string {
	title, company := html.EscapeString(m.JobTitle), html.EscapeString(m.Company)
	switch m.Kind {
	case KindFollowUp:
		return fmt.Sprintf(
			"👋 It's been %d days since anything moved on <b>%s</b> at <b>%s</b>. Worth a follow-up?\n<a href=\"%s\">Open your tracking board →</a>",
			m.DaysSilent, title, company, n.trackingURL)
	case KindInterviewPrep:
		return fmt.Sprintf(
			"🎯 You're interviewing for <b>%s</b> at <b>%s</b>. Ready to rehearse?\n<a href=\"%s\">Open your tracking board →</a>",
			title, company, n.trackingURL)
	default:
		return fmt.Sprintf("<b>%s</b> at <b>%s</b>: <a href=\"%s\">Open your tracking board →</a>", title, company, n.trackingURL)
	}
}

// EmailNotifier delivers a nudge as an email, reusing the emailnotify SES
// transport. Like the Telegram notifier, its link stays on-platform.
type EmailNotifier struct {
	sender      emailnotify.Sender
	from        string
	trackingURL string
}

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// the tracking link rooted at jobBaseURL.
func NewEmailNotifier(sender emailnotify.Sender, from, jobBaseURL string) *EmailNotifier {
	return &EmailNotifier{sender: sender, from: from, trackingURL: strings.TrimRight(jobBaseURL, "/") + "/my/tracking?utm_source=email"}
}

// Send renders the nudge and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest string, m Message) error {
	subject, htmlBody, textBody := n.render(m)
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
}

func (n *EmailNotifier) render(m Message) (subject, htmlBody, textBody string) {
	title, company := html.EscapeString(m.JobTitle), html.EscapeString(m.Company)
	switch m.Kind {
	case KindFollowUp:
		subject = fmt.Sprintf("Time to follow up: %s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(
			`<p>It's been %d days since anything moved on <strong>%s</strong> at <strong>%s</strong>.</p>`+
				`<p><a href="%s">Open your tracking board and follow up →</a></p>`,
			m.DaysSilent, title, company, n.trackingURL)
		textBody = fmt.Sprintf("It's been %d days since anything moved on %s at %s.\n\nOpen your tracking board: %s\n",
			m.DaysSilent, m.JobTitle, m.Company, n.trackingURL)
	case KindInterviewPrep:
		subject = fmt.Sprintf("Prepare for your interview: %s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(
			`<p>You're interviewing for <strong>%s</strong> at <strong>%s</strong>.</p>`+
				`<p><a href="%s">Open your tracking board to rehearse →</a></p>`,
			title, company, n.trackingURL)
		textBody = fmt.Sprintf("You're interviewing for %s at %s.\n\nOpen your tracking board: %s\n",
			m.JobTitle, m.Company, n.trackingURL)
	default:
		subject = fmt.Sprintf("%s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(`<p><a href="%s">Open your tracking board →</a></p>`, n.trackingURL)
		textBody = fmt.Sprintf("Open your tracking board: %s\n", n.trackingURL)
	}
	return subject, htmlBody, textBody
}
