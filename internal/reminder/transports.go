package reminder

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/mailtpl"
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
	layout     *mailtpl.Layout
}

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// the job link rooted at jobBaseURL.
func NewEmailNotifier(sender emailnotify.Sender, from, jobBaseURL string) *EmailNotifier {
	base := strings.TrimRight(jobBaseURL, "/")
	return &EmailNotifier{sender: sender, from: from, jobBaseURL: base, layout: mailtpl.New(base)}
}

// emailTemplate renders the body from a mailtpl.Job. The saved job leads, drawn by
// the shared row so it looks the same here as in a digest, logo and all; the
// sentence underneath only says why the mail arrived.
//
// The job's fields are source data and are escaped in context by html/template —
// the reason this notifier no longer calls html.EscapeString by hand.
var emailTemplate = template.Must(mailtpl.Partials().New("reminder").Parse(`
{{template "job-row" .}}
<div style="height:18px;"></div>
{{template "p" "You saved this job and haven’t applied yet."}}
{{template "button" (mailLink .URL "Open the job and apply")}}`))

// Send renders the reminder and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest string, m ReminderMessage) error {
	url := n.jobBaseURL + "/jobs/" + m.Slug + "?utm_source=email"
	subject := fmt.Sprintf("Reminder: %s at %s", m.JobTitle, m.Company)

	var content bytes.Buffer
	if err := emailTemplate.Execute(&content, mailtpl.NewJob(m.JobTitle, m.Company, "", url)); err != nil {
		return err
	}
	htmlBody := n.layout.Render(mailtpl.Body{
		Preheader: "A job you saved is still open",
		Heading:   "Still interested?",
		Content:   template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above, which escaped both fields in context
		Footer:    "You’re getting this because you saved this job on freehire.",
	})

	textBody := fmt.Sprintf("You saved %s at %s and haven't applied yet.\n\nOpen the job: %s\n",
		m.JobTitle, m.Company, url)
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
}
