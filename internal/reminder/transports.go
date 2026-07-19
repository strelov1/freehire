package reminder

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

// TelegramNotifier delivers a reminder as a Telegram HTML message, reusing the
// telegramnotify Bot API client. The link points at the on-platform job page so
// the nudge keeps the user on freehire and never exposes a login-gated source URL.
type TelegramNotifier struct {
	client     *telegramnotify.Client
	jobBaseURL string
}

// NewTelegramNotifier builds a TelegramNotifier sending through client, with the
// job link rooted at jobBaseURL (the frontend origin).
func NewTelegramNotifier(client *telegramnotify.Client, jobBaseURL string) *TelegramNotifier {
	return &TelegramNotifier{client: client, jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
}

// Send renders the reminder and posts it to the chat encoded in dest. The channel
// argument is ignored — this notifier only serves the telegram channel.
func (n *TelegramNotifier) Send(ctx context.Context, _ string, dest string, m ReminderMessage) error {
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("reminder: invalid telegram chat id %q: %w", dest, err)
	}
	return n.client.SendMessage(ctx, chatID, n.render(m))
}

// render builds the HTML body. Title and company are user/source data and are
// HTML-escaped; the freehire URL is our own and safe.
func (n *TelegramNotifier) render(m ReminderMessage) string {
	url := n.jobBaseURL + "/jobs/" + m.Slug
	return fmt.Sprintf(
		"⏰ Reminder: you saved <b>%s</b> at <b>%s</b>.\nStill interested? <a href=\"%s\">Open the job →</a>",
		html.EscapeString(m.JobTitle), html.EscapeString(m.Company), url)
}

// EmailNotifier delivers a reminder as an email, reusing the emailnotify SES
// transport. Like the Telegram notifier, its link stays on-platform.
type EmailNotifier struct {
	sender     emailnotify.Sender
	from       string
	jobBaseURL string
}

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// the job link rooted at jobBaseURL.
func NewEmailNotifier(sender emailnotify.Sender, from, jobBaseURL string) *EmailNotifier {
	return &EmailNotifier{sender: sender, from: from, jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
}

// Send renders the reminder and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest string, m ReminderMessage) error {
	url := n.jobBaseURL + "/jobs/" + m.Slug + "?utm_source=email"
	subject := fmt.Sprintf("Reminder: %s at %s", m.JobTitle, m.Company)
	title := html.EscapeString(m.JobTitle)
	company := html.EscapeString(m.Company)
	htmlBody := fmt.Sprintf(
		`<p>You saved <strong>%s</strong> at <strong>%s</strong> and haven't applied yet.</p>`+
			`<p><a href="%s">Open the job and apply →</a></p>`,
		title, company, url)
	textBody := fmt.Sprintf("You saved %s at %s and haven't applied yet.\n\nOpen the job: %s\n",
		m.JobTitle, m.Company, url)
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
}
