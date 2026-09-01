package reminder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
)

// senderName is the display name on these mails: they are the product speaking,
// not a person, so a message list should say so.
const senderName = "freehire"

// Compile-time proof the transports satisfy the engine's Notifier seam.
var (
	_ Notifier = (*TelegramNotifier)(nil)
	_ Notifier = (*EmailNotifier)(nil)
)

// TelegramNotifier delivers an account's due reminders as one Telegram HTML
// message, reusing the telegramnotify Bot API client. Links point at on-platform
// job pages so the nudge keeps the user on freehire and never exposes a
// login-gated source URL.
type TelegramNotifier struct {
	client     *telegramnotify.Client
	jobBaseURL string
}

// NewTelegramNotifier builds a TelegramNotifier sending through client, with the
// job link rooted at jobBaseURL (the frontend origin).
func NewTelegramNotifier(client *telegramnotify.Client, jobBaseURL string) *TelegramNotifier {
	return &TelegramNotifier{client: client, jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
}

// Send renders the batch and posts it to the chat encoded in dest. The channel
// argument is ignored — this notifier only serves the telegram channel.
//
// A 403 is translated to ErrRecipientGone, this engine's vocabulary for a chat
// permanently closed to us, so the runner unlinks it instead of retrying a send
// that cannot land. Same translation the digest and nudge notifiers do.
func (n *TelegramNotifier) Send(ctx context.Context, _ string, dest string, ms []ReminderMessage) error {
	if len(ms) == 0 {
		return nil
	}
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("reminder: invalid telegram chat id %q: %w", dest, err)
	}
	err = n.client.SendMessage(ctx, chatID, n.render(ms))
	if errors.Is(err, telegramnotify.ErrChatUnreachable) {
		return fmt.Errorf("%w: %w", ErrRecipientGone, err)
	}
	return err
}

// render builds the HTML body. Titles and companies are user/source data and are
// HTML-escaped; the freehire URLs are our own and safe.
//
// A single reminder keeps the sentence it has always been. A batch becomes a list,
// bounded twice: by notify.ListLimit for readability, and by Telegram's own
// MaxMessageLen — lines are added only while the next one plus the widest possible
// tail still fits, because an oversized body is rejected deterministically and
// every retry re-fails.
func (n *TelegramNotifier) render(ms []ReminderMessage) string {
	if len(ms) == 1 {
		return n.renderOne(ms[0])
	}

	// The list bound picks the candidates; the length cap decides how many of them
	// actually fit, so the tail is computed from what was written, not from it.
	shown, _ := notify.Listed(ms)
	var b strings.Builder
	fmt.Fprintf(&b, "⏰ You saved <b>%d</b> jobs and haven't applied yet.\n\n", len(ms))

	// Reserve room for the widest possible tail up front (the whole batch is its
	// worst-case count), so appending the actual tail can never push past the limit.
	tailReserve := telegramnotify.UTF16Len(n.moreLine(len(ms)))
	used := telegramnotify.UTF16Len(b.String())
	fitted := 0
	for _, m := range shown {
		line := n.jobLine(m)
		lineLen := telegramnotify.UTF16Len(line)
		if used+lineLen+tailReserve > telegramnotify.MaxMessageLen {
			break
		}
		b.WriteString(line)
		used += lineLen
		fitted++
	}
	if omitted := len(ms) - fitted; omitted > 0 {
		b.WriteString(n.moreLine(omitted))
	}
	return b.String()
}

// renderOne is the single-reminder body, kept verbatim — link included, which
// carries no UTM tag — because a batch of one must be indistinguishable from what
// shipped before grouping.
func (n *TelegramNotifier) renderOne(m ReminderMessage) string {
	return fmt.Sprintf(
		"⏰ Reminder: you saved <b>%s</b> at <b>%s</b>.\nStill interested? <a href=\"%s\">Open the job →</a>",
		html.EscapeString(m.JobTitle), html.EscapeString(m.Company), n.jobBaseURL+"/jobs/"+m.Slug)
}

// jobLine renders one saved job as a bullet linking to its freehire page.
func (n *TelegramNotifier) jobLine(m ReminderMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "• <a href=%q>%s</a>", n.jobURL(m), html.EscapeString(m.JobTitle))
	if m.Company != "" {
		fmt.Fprintf(&b, " — %s", html.EscapeString(m.Company))
	}
	b.WriteByte('\n')
	return b.String()
}

// moreLine is the overflow tail, or "" when nothing is omitted. It leads to the
// saved-jobs list rather than to this delivery's own notification page: a reminder
// records its notification AFTER it is sent, so no id exists while the message is
// still being built.
func (n *TelegramNotifier) moreLine(more int) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=%q>+ %d more in your saved jobs</a>", n.savedURL(), more)
}

// savedURL is the saved-jobs list, where a batch sends anyone who wants the jobs it
// could not itemize.
func (n *TelegramNotifier) savedURL() string { return n.jobBaseURL + "/my/activity" }

// jobURL is the on-platform freehire job page, tagged with a telegram UTM source.
// Slugs are our own normalized values, so the URL needs no escaping.
func (n *TelegramNotifier) jobURL(m ReminderMessage) string {
	return n.jobBaseURL + "/jobs/" + m.Slug + "?utm_source=telegram-bot"
}

// EmailNotifier delivers an account's due reminders as one email, reusing the
// emailnotify SES transport. Like the Telegram notifier, its links stay
// on-platform.
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
	return &EmailNotifier{sender: sender, from: emailnotify.From(senderName, from), jobBaseURL: base, layout: mailtpl.New(base)}
}

// emailData is what the body template renders. Every field is emitted in an
// escaping context by html/template, which is the injection guard for the
// source-derived titles and company names.
type emailData struct {
	Jobs   []mailtpl.Job
	More   int
	Reason string       // the one sentence saying why this mail arrived
	CTA    mailtpl.Link // the single action: one job opens the job, a batch opens the list
}

// oneTemplate is the single-reminder body, unchanged since before grouping: a batch
// of one must be byte-identical to what shipped, so it keeps its own template rather
// than becoming a list of length one.
var oneTemplate = template.Must(mailtpl.Partials().New("reminder").Parse(`
{{template "job-row" .}}
<div style="height:18px;"></div>
{{template "p" "You saved this job and haven’t applied yet."}}
{{template "button" (mailLink .URL "Open the job and apply")}}`))

// batchTemplate renders the saved jobs as rows drawn by the shared partial, so they
// look the same here as in a digest, logo and all; the sentence underneath only says
// why the mail arrived. The list is a table rather than stacked divs because Outlook
// collapses margins between block elements unpredictably.
var batchTemplate = template.Must(mailtpl.Partials().New("reminder-batch").Parse(`
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
  {{range $i, $job := .Jobs}}
  <tr><td class="m-row" style="padding:14px 0;{{if $i}}border-top:1px solid #e4e4e4;{{end}}">{{template "job-row" $job}}</td></tr>
  {{end}}
</table>
<div style="height:18px;"></div>
{{template "p" .Reason}}
{{if gt .More 0}}{{template "muted" (printf "+ %d more not listed here." .More)}}{{end}}
{{template "button" .CTA}}`))

// Send renders the batch and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest string, ms []ReminderMessage) error {
	if len(ms) == 0 {
		return nil
	}
	if len(ms) == 1 {
		return n.sendOne(ctx, dest, ms[0])
	}
	return n.sendBatch(ctx, dest, ms)
}

// sendOne is the pre-grouping mail, kept verbatim.
func (n *EmailNotifier) sendOne(ctx context.Context, dest string, m ReminderMessage) error {
	url := n.jobURL(m)
	var content bytes.Buffer
	if err := oneTemplate.Execute(&content, mailtpl.NewJob(m.JobTitle, m.Company, "", url)); err != nil {
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
	subject := fmt.Sprintf("Reminder: %s at %s", m.JobTitle, m.Company)
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
}

// sendBatch is the multi-reminder mail: the same job rows a digest draws, one
// sentence saying why they arrived together, and one action.
func (n *EmailNotifier) sendBatch(ctx context.Context, dest string, ms []ReminderMessage) error {
	shown, more := notify.Listed(ms)
	rows := make([]mailtpl.Job, 0, len(shown))
	for _, m := range shown {
		rows = append(rows, mailtpl.NewJob(m.JobTitle, m.Company, "", n.jobURL(m)))
	}
	reason := "You saved these jobs and haven’t applied yet."

	var content bytes.Buffer
	if err := batchTemplate.Execute(&content, emailData{
		Jobs: rows, More: more, Reason: reason,
		CTA: mailtpl.Link{URL: n.savedURL(), Label: "Open your saved jobs"},
	}); err != nil {
		return err
	}
	htmlBody := n.layout.Render(mailtpl.Body{
		Preheader: fmt.Sprintf("%d jobs you saved are still open", len(ms)),
		Heading:   fmt.Sprintf("%d saved jobs are still open", len(ms)),
		Content:   template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above, which escaped every field in context
		Footer:    "You’re getting this because you saved these jobs on freehire.",
	})

	subject := fmt.Sprintf("Reminder: %d saved jobs", len(ms))
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, n.renderBatchText(shown, more, reason))
}

// savedURL is the saved-jobs list, tagged with an email UTM source.
func (n *EmailNotifier) savedURL() string { return n.jobBaseURL + "/my/activity?utm_source=email" }

// renderBatchText builds the plain-text alternative for a batch, mirroring the HTML
// body so non-HTML clients (and spam scorers) see the same content.
func (n *EmailNotifier) renderBatchText(shown []ReminderMessage, more int, reason string) string {
	var b strings.Builder
	b.WriteString(reason + "\n\n")
	for _, m := range shown {
		b.WriteString("- " + m.JobTitle)
		if m.Company != "" {
			b.WriteString(" — " + m.Company)
		}
		b.WriteString("\n  " + n.jobURL(m) + "\n")
	}
	if more > 0 {
		fmt.Fprintf(&b, "\n+ %d more at %s\n", more, n.savedURL())
	}
	return b.String()
}

// jobURL is the on-platform freehire job page, tagged with an email UTM source so
// the channel's traffic is attributable.
func (n *EmailNotifier) jobURL(m ReminderMessage) string {
	return n.jobBaseURL + "/jobs/" + m.Slug + "?utm_source=email"
}
