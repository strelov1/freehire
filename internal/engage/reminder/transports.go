package reminder

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/engage/batchnotify"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/mailtpl"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
)

// TelegramNotifier delivers an account's due reminders as one Telegram HTML
// message. Links point at on-platform job pages, never a login-gated source URL.
// The send itself is the shared batchnotify.Telegram; this package owns only the
// copy. A 403 is translated to ErrRecipientGone so the runner unlinks the chat
// instead of retrying; the digest and nudge notifiers do the same.
type TelegramNotifier = batchnotify.Telegram[ReminderMessage]

// NewTelegramNotifier builds a TelegramNotifier sending through client, with the
// job link rooted at jobBaseURL (the frontend origin).
func NewTelegramNotifier(client *telegramnotify.Client, jobBaseURL string) *TelegramNotifier {
	return batchnotify.NewTelegram(client, jobBaseURL, newTelegramCopy(jobBaseURL).render)
}

// telegramCopy is the reminder copy for the Telegram channel.
type telegramCopy struct{ jobBaseURL string }

func newTelegramCopy(jobBaseURL string) telegramCopy {
	return telegramCopy{jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
}

// render builds the HTML body. Titles and companies are user/source data and are
// HTML-escaped; the freehire URLs are our own and safe.
//
// A batch becomes a list bounded by notify.ListLimit and by Telegram's
// MaxMessageLen: lines are added only while the next one plus the widest possible
// tail fits, because an oversized body is rejected and every retry re-fails.
func (n telegramCopy) render(ms []ReminderMessage) string {
	if len(ms) == 1 {
		return n.renderOne(ms[0])
	}

	shown, _ := notify.Listed(ms)
	var b strings.Builder
	fmt.Fprintf(&b, "⏰ You saved <b>%d</b> jobs and haven't applied yet.\n\n", len(ms))

	// Reserve room for the widest possible tail so appending the real one cannot pass the limit.
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

// renderOne is the single-reminder body. Its link carries no UTM tag; a batch of
// one must stay identical to the pre-grouping message.
func (n telegramCopy) renderOne(m ReminderMessage) string {
	return fmt.Sprintf(
		"⏰ Reminder: you saved <b>%s</b> at <b>%s</b>.\nStill interested? <a href=\"%s\">Open the job →</a>",
		html.EscapeString(m.JobTitle), html.EscapeString(m.Company), n.jobBaseURL+"/jobs/"+m.Slug)
}

// jobLine renders one saved job as a bullet linking to its freehire page.
func (n telegramCopy) jobLine(m ReminderMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "• <a href=%q>%s</a>", n.jobURL(m), html.EscapeString(m.JobTitle))
	if m.Company != "" {
		fmt.Fprintf(&b, " — %s", html.EscapeString(m.Company))
	}
	b.WriteByte('\n')
	return b.String()
}

// moreLine is the overflow tail, or "" when nothing is omitted. It links to the
// saved-jobs list because the notification id does not exist until after the send.
func (n telegramCopy) moreLine(more int) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=%q>+ %d more in your saved jobs</a>", n.savedURL(), more)
}

// savedURL is the saved-jobs list, for jobs the batch could not itemize.
func (n telegramCopy) savedURL() string { return n.jobBaseURL + "/my/activity" }

// jobURL is the on-platform freehire job page, tagged with a telegram UTM source.
// Slugs are our own normalized values, so the URL needs no escaping.
func (n telegramCopy) jobURL(m ReminderMessage) string {
	return n.jobBaseURL + "/jobs/" + m.Slug + "?utm_source=telegram-bot"
}

// EmailNotifier delivers an account's due reminders as one email. Links stay
// on-platform. The send itself is the shared batchnotify.Email; this package owns
// only the copy.
type EmailNotifier = batchnotify.Email[ReminderMessage]

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// the job link rooted at jobBaseURL and unsubscribe links signed by links.
func NewEmailNotifier(sender emailnotify.Sender, from, jobBaseURL string, links *emailprefs.Links) *EmailNotifier {
	return batchnotify.NewEmail(sender, from, jobBaseURL, links, "reminder",
		func(ms []ReminderMessage) int64 { return ms[0].UserID }, newEmailCopy(jobBaseURL).send)
}

// emailCopy is the reminder copy for the email channel.
type emailCopy struct {
	jobBaseURL string
	layout     *mailtpl.Layout
}

func newEmailCopy(jobBaseURL string) emailCopy {
	base := strings.TrimRight(jobBaseURL, "/")
	return emailCopy{jobBaseURL: base, layout: mailtpl.New(base)}
}

// emailData is what the body template renders. html/template escapes every field,
// which guards against injection via source-derived titles and company names.
type emailData struct {
	Jobs   []mailtpl.Job
	More   int
	Reason string       // the one sentence saying why this mail arrived
	CTA    mailtpl.Link // the single action: one job opens the job, a batch opens the list
}

// oneTemplate is the single-reminder body. A batch of one must stay byte-identical
// to the pre-grouping mail, so it keeps its own template.
var oneTemplate = template.Must(mailtpl.Partials().New("reminder").Parse(`
{{template "job-row" .}}
<div style="height:18px;"></div>
{{template "p" "You saved this job and haven’t applied yet."}}
{{template "button" (mailLink .URL "Open the job and apply")}}`))

// batchTemplate renders saved jobs with the shared job-row partial, like a digest.
// It is a table because Outlook collapses margins between block elements.
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

// send renders the batch: a single reminder keeps its own mail, several share one.
func (n emailCopy) send(ms []ReminderMessage, unsubscribe string) (subject, htmlBody, textBody string) {
	if len(ms) == 1 {
		return n.sendOne(ms[0], unsubscribe)
	}
	return n.sendBatch(ms, unsubscribe)
}

// sendOne is the single-reminder mail, kept as it was before grouping.
func (n emailCopy) sendOne(m ReminderMessage, unsubscribe string) (subject, htmlBody, textBody string) {
	url := n.jobURL(m)
	var content bytes.Buffer
	// The template is a trusted constant and the data is escaped in context, so this
	// fails only on a template bug — caught by the golden previews.
	_ = oneTemplate.Execute(&content, mailtpl.NewJob(m.JobTitle, m.Company, "", url))
	htmlBody = n.layout.Render(mailtpl.Body{
		Preheader:      "A job you saved is still open",
		Heading:        "Still interested?",
		Content:        template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above, which escaped both fields in context
		Footer:         "You’re getting this because you saved this job on freehire.",
		UnsubscribeURL: unsubscribe,
	})

	textBody = fmt.Sprintf("You saved %s at %s and haven't applied yet.\n\nOpen the job: %s\n",
		m.JobTitle, m.Company, url)
	subject = fmt.Sprintf("Reminder: %s at %s", m.JobTitle, m.Company)
	return subject, htmlBody, textBody
}

// sendBatch is the multi-reminder mail: digest-style job rows, one reason line, one action.
func (n emailCopy) sendBatch(ms []ReminderMessage, unsubscribe string) (subject, htmlBody, textBody string) {
	shown, more := notify.Listed(ms)
	rows := make([]mailtpl.Job, 0, len(shown))
	for _, m := range shown {
		rows = append(rows, mailtpl.NewJob(m.JobTitle, m.Company, "", n.jobURL(m)))
	}
	reason := "You saved these jobs and haven’t applied yet."

	var content bytes.Buffer
	// The template is a trusted constant and the data is escaped in context, so this
	// fails only on a template bug — caught by the golden previews.
	_ = batchTemplate.Execute(&content, emailData{
		Jobs: rows, More: more, Reason: reason,
		CTA: mailtpl.Link{URL: n.savedURL(), Label: "Open your saved jobs"},
	})
	htmlBody = n.layout.Render(mailtpl.Body{
		Preheader:      fmt.Sprintf("%d jobs you saved are still open", len(ms)),
		Heading:        fmt.Sprintf("%d saved jobs are still open", len(ms)),
		Content:        template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above, which escaped every field in context
		Footer:         "You’re getting this because you saved these jobs on freehire.",
		UnsubscribeURL: unsubscribe,
	})

	subject = fmt.Sprintf("Reminder: %d saved jobs", len(ms))
	textBody = n.renderBatchText(shown, more, reason)
	return subject, htmlBody, textBody
}

// savedURL is the saved-jobs list, tagged with an email UTM source.
func (n emailCopy) savedURL() string { return n.jobBaseURL + "/my/activity?utm_source=email" }

// renderBatchText builds the plain-text alternative for a batch, mirroring the HTML
// body so non-HTML clients (and spam scorers) see the same content.
func (n emailCopy) renderBatchText(shown []ReminderMessage, more int, reason string) string {
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
func (n emailCopy) jobURL(m ReminderMessage) string {
	return n.jobBaseURL + "/jobs/" + m.Slug + "?utm_source=email"
}
