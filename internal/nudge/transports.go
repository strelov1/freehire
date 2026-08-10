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
// telegramnotify Bot API client. Every link stays on-platform: the tracking board
// for kinds about the application (follow-up, interview-prep), the job's own page
// for job-closed — there is nothing left to track on the board for a listing that
// just closed, but the posting itself (and whatever the candidate captured from it)
// is still worth a look.
type TelegramNotifier struct {
	client *telegramnotify.Client
	origin string
}

// NewTelegramNotifier builds a TelegramNotifier sending through client, with links
// rooted at origin (the frontend origin).
func NewTelegramNotifier(client *telegramnotify.Client, origin string) *TelegramNotifier {
	return &TelegramNotifier{client: client, origin: strings.TrimRight(origin, "/")}
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
	trackingURL, jobURL := n.origin+"/my/tracking", n.origin+"/jobs/"+m.Slug
	switch m.Kind {
	case KindFollowUp:
		return fmt.Sprintf(
			"👋 It's been %d days since anything moved on <b>%s</b> at <b>%s</b>. Worth a follow-up?\n<a href=\"%s\">Open your tracking board →</a>",
			m.DaysSilent, title, company, trackingURL)
	case KindInterviewPrep:
		return fmt.Sprintf(
			"🎯 You're interviewing for <b>%s</b> at <b>%s</b>. Ready to rehearse?\n<a href=\"%s\">Open your tracking board →</a>",
			title, company, trackingURL)
	case KindJobClosed:
		return fmt.Sprintf(
			"📪 <b>%s</b> at <b>%s</b> was closed.\n<a href=\"%s\">Open the job →</a>",
			title, company, jobURL)
	default:
		return fmt.Sprintf("<b>%s</b> at <b>%s</b>: <a href=\"%s\">Open your tracking board →</a>", title, company, trackingURL)
	}
}

// EmailNotifier delivers a nudge as an email, reusing the emailnotify SES
// transport. Like the Telegram notifier, its links stay on-platform.
type EmailNotifier struct {
	sender emailnotify.Sender
	from   string
	origin string
}

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// links rooted at origin.
func NewEmailNotifier(sender emailnotify.Sender, from, origin string) *EmailNotifier {
	return &EmailNotifier{sender: sender, from: from, origin: strings.TrimRight(origin, "/")}
}

// Send renders the nudge and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest string, m Message) error {
	subject, htmlBody, textBody := n.render(m)
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
}

func (n *EmailNotifier) render(m Message) (subject, htmlBody, textBody string) {
	title, company := html.EscapeString(m.JobTitle), html.EscapeString(m.Company)
	trackingURL := n.origin + "/my/tracking?utm_source=email"
	jobURL := n.origin + "/jobs/" + m.Slug + "?utm_source=email"
	switch m.Kind {
	case KindFollowUp:
		subject = fmt.Sprintf("Time to follow up: %s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(
			`<p>It's been %d days since anything moved on <strong>%s</strong> at <strong>%s</strong>.</p>`+
				`<p><a href="%s">Open your tracking board and follow up →</a></p>`,
			m.DaysSilent, title, company, trackingURL)
		textBody = fmt.Sprintf("It's been %d days since anything moved on %s at %s.\n\nOpen your tracking board: %s\n",
			m.DaysSilent, m.JobTitle, m.Company, trackingURL)
	case KindInterviewPrep:
		subject = fmt.Sprintf("Prepare for your interview: %s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(
			`<p>You're interviewing for <strong>%s</strong> at <strong>%s</strong>.</p>`+
				`<p><a href="%s">Open your tracking board to rehearse →</a></p>`,
			title, company, trackingURL)
		textBody = fmt.Sprintf("You're interviewing for %s at %s.\n\nOpen your tracking board: %s\n",
			m.JobTitle, m.Company, trackingURL)
	case KindJobClosed:
		subject = fmt.Sprintf("Closed: %s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(
			`<p><strong>%s</strong> at <strong>%s</strong> was closed.</p>`+
				`<p><a href="%s">Open the job →</a></p>`,
			title, company, jobURL)
		textBody = fmt.Sprintf("%s at %s was closed.\n\nOpen the job: %s\n", m.JobTitle, m.Company, jobURL)
	default:
		subject = fmt.Sprintf("%s at %s", m.JobTitle, m.Company)
		htmlBody = fmt.Sprintf(`<p><a href="%s">Open your tracking board →</a></p>`, trackingURL)
		textBody = fmt.Sprintf("Open your tracking board: %s\n", trackingURL)
	}
	return subject, htmlBody, textBody
}
