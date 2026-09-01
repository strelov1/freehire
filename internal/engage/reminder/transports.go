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

// listed splits a batch into the reminders a message itemizes and how many it only
// counts. The bound is notify.ListLimit, shared with the subscription digest so the
// two channels never disagree about how long a list may be; the batch itself is
// bounded separately and much higher by Config.SnapshotCap, because the message is
// short for readability while the record behind it is long by design.
func listed(ms []ReminderMessage) (shown []ReminderMessage, more int) {
	if len(ms) <= notify.ListLimit {
		return ms, 0
	}
	return ms[:notify.ListLimit:notify.ListLimit], len(ms) - notify.ListLimit
}

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
		m := ms[0]
		return fmt.Sprintf(
			"⏰ Reminder: you saved <b>%s</b> at <b>%s</b>.\nStill interested? <a href=%q>Open the job →</a>",
			html.EscapeString(m.JobTitle), html.EscapeString(m.Company), n.jobURL(m))
	}

	// The list bound picks the candidates; the length cap decides how many of them
	// actually fit, so the tail is computed from what was written, not from it.
	shown, _ := listed(ms)
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

// emailTemplate renders the saved jobs as rows drawn by the shared partial, so they
// look the same here as in a digest, logo and all; the sentence underneath only
// says why the mail arrived. The list is a table rather than stacked divs because
// Outlook collapses margins between block elements unpredictably.
var emailTemplate = template.Must(mailtpl.Partials().New("reminder").Parse(`
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
	shown, more := listed(ms)
	rows := make([]mailtpl.Job, 0, len(shown))
	for _, m := range shown {
		rows = append(rows, mailtpl.NewJob(m.JobTitle, m.Company, "", n.jobURL(m)))
	}

	subject, heading, preheader, reason := n.copy(ms)

	var content bytes.Buffer
	if err := emailTemplate.Execute(&content, emailData{
		Jobs: rows, More: more, Reason: reason, CTA: n.cta(ms),
	}); err != nil {
		return err
	}
	htmlBody := n.layout.Render(mailtpl.Body{
		Preheader: preheader,
		Heading:   heading,
		Content:   template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above, which escaped every field in context
		Footer:    "You’re getting this because you saved these jobs on freehire.",
	})

	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, n.renderText(shown, more, reason))
}

// copy is the wording for a batch: one saved job still reads as the single-job mail
// it has always been, several become a count.
func (n *EmailNotifier) copy(ms []ReminderMessage) (subject, heading, preheader, reason string) {
	if len(ms) == 1 {
		m := ms[0]
		return fmt.Sprintf("Reminder: %s at %s", m.JobTitle, m.Company),
			"Still interested?",
			"A job you saved is still open",
			"You saved this job and haven’t applied yet."
	}
	return fmt.Sprintf("Reminder: %d saved jobs", len(ms)),
		fmt.Sprintf("%d saved jobs are still open", len(ms)),
		fmt.Sprintf("%d jobs you saved are still open", len(ms)),
		"You saved these jobs and haven’t applied yet."
}

// cta is the mail's single action. One saved job still opens that job, because that
// is the whole errand; several have no one destination, so the action becomes the
// saved-jobs list.
func (n *EmailNotifier) cta(ms []ReminderMessage) mailtpl.Link {
	if len(ms) == 1 {
		return mailtpl.Link{URL: n.jobURL(ms[0]), Label: "Open the job and apply"}
	}
	return mailtpl.Link{URL: n.savedURL(), Label: "Open your saved jobs"}
}

// savedURL is the saved-jobs list, tagged with an email UTM source.
func (n *EmailNotifier) savedURL() string { return n.jobBaseURL + "/my/activity?utm_source=email" }

// renderText builds the plain-text alternative, mirroring the HTML body so
// non-HTML clients (and spam scorers) see the same content.
func (n *EmailNotifier) renderText(shown []ReminderMessage, more int, reason string) string {
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
