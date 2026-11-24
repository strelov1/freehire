package nudge

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

// TelegramNotifier delivers a nudge as a Telegram HTML message. Every link stays
// on-platform: the tracking board for application kinds, the job's own page for
// job-closed (nothing is left to track for a closed listing). The send itself is
// the shared notify.Telegram; this package owns only the copy.
type TelegramNotifier = batchnotify.Telegram[Message]

// NewTelegramNotifier builds a TelegramNotifier sending through client, with links
// rooted at origin (the frontend origin). A 403 is translated to ErrRecipientGone
// (telegramnotify.SendToChat) so the runner unlinks the chat instead of retrying a
// send that cannot land.
func NewTelegramNotifier(client *telegramnotify.Client, origin string) *TelegramNotifier {
	return batchnotify.NewTelegram(client, origin, newTelegramCopy(origin).render)
}

// telegramCopy is the nudge copy for the Telegram channel.
type telegramCopy struct{ origin string }

func newTelegramCopy(origin string) telegramCopy {
	return telegramCopy{origin: strings.TrimRight(origin, "/")}
}

// render builds the HTML body: one nudge is a sentence, several are a headline and
// a list bounded by notify.ListLimit and by Telegram's MaxMessageLen (an oversized
// body is rejected deterministically, so every retry would re-fail). Every message
// of a batch carries the group's kind, so the first one names it.
func (n telegramCopy) render(ms []Message) string {
	if len(ms) == 1 {
		return n.renderOne(ms[0])
	}
	kind := ms[0].Kind

	var b strings.Builder
	b.WriteString(n.batchHeadline(kind, len(ms)) + "\n\n")

	tailReserve := telegramnotify.UTF16Len(n.moreLine(len(ms), kind))
	used := telegramnotify.UTF16Len(b.String())
	fitted := 0
	shown, _ := notify.Listed(ms)
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
		b.WriteString(n.moreLine(omitted, kind))
	}
	return b.String()
}

// batchHeadline is the one line saying what this batch is about. It holds only a
// count, so nothing is escaped.
func (n telegramCopy) batchHeadline(kind string, count int) string {
	switch kind {
	case KindFollowUp:
		return fmt.Sprintf("👋 <b>%d</b> of your applications have gone quiet. Worth a follow-up?", count)
	case KindInterviewPrep:
		return fmt.Sprintf("🎯 You're interviewing for <b>%d</b> roles. Ready to rehearse?", count)
	case KindJobClosed:
		return fmt.Sprintf("📪 <b>%d</b> jobs you were tracking were closed.", count)
	case KindAutoApplySubmitted:
		return fmt.Sprintf("🎉 Auto-apply submitted <b>%d</b> applications for you.", count)
	case KindAutoApplyBlocked:
		return fmt.Sprintf("⚠️ Auto-apply couldn't finish <b>%d</b> applications — a required question needs your own answer.", count)
	case KindAutoApplyFailed:
		return fmt.Sprintf("Auto-apply couldn't submit <b>%d</b> applications, and won't try again.", count)
	case KindAutoApplyReviewReady:
		return fmt.Sprintf("📝 <b>%d</b> tailored applications are waiting for your OK before auto-apply sends them.", count)
	default:
		return fmt.Sprintf("<b>%d</b> updates on jobs you are tracking.", count)
	}
}

// jobLine renders one nudged job as a bullet linking to its freehire page.
func (n telegramCopy) jobLine(m Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "• <a href=%q>%s</a>", n.origin+"/jobs/"+m.Slug, html.EscapeString(m.JobTitle))
	if m.Company != "" {
		fmt.Fprintf(&b, " — %s", html.EscapeString(m.Company))
	}
	b.WriteByte('\n')
	return b.String()
}

// moreLine is the overflow tail, or "" when nothing is omitted. It leads where the
// batch's own call to action leads.
func (n telegramCopy) moreLine(more int, kind string) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=%q>+ %d more</a>", n.batchURL(kind), more)
}

// batchURL is where a batch sends the reader, per batchDestination.
func (n telegramCopy) batchURL(kind string) string {
	path, _ := batchDestination(kind)
	return n.origin + path
}

// batchDestination decides where a batch of `kind` leads and what the action is
// called: the tracking board, or the saved list for job-closed. Both channels read
// it, so they never point different ways.
func batchDestination(kind string) (path, label string) {
	if kind == KindJobClosed {
		return "/my/activity", "Open your saved jobs"
	}
	return "/my/tracking", "Open your tracking board"
}

// renderOne is the single-nudge body; a batch of one must render as a real single
// message, never as a one-item batch headline.
func (n telegramCopy) renderOne(m Message) string {
	title, company := html.EscapeString(m.JobTitle), html.EscapeString(m.Company)
	trackingURL, jobURL := n.origin+"/my/tracking", n.origin+"/jobs/"+m.Slug
	applicationURL := n.origin + "/my/tracking/" + m.Slug
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
	case KindAutoApplySubmitted:
		return fmt.Sprintf(
			"🎉 Auto-apply submitted your application to <b>%s</b> at <b>%s</b>.\n<a href=\"%s\">Open your application →</a>",
			title, company, applicationURL)
	case KindAutoApplyBlocked:
		return fmt.Sprintf(
			"⚠️ Auto-apply couldn't finish <b>%s</b> at <b>%s</b> — a required question needs your own answer.\n<a href=\"%s\">Open your application →</a>",
			title, company, applicationURL)
	case KindAutoApplyFailed:
		return fmt.Sprintf(
			"Auto-apply couldn't submit <b>%s</b> at <b>%s</b>, and won't try again.\n<a href=\"%s\">Open your application →</a>",
			title, company, applicationURL)
	case KindAutoApplyReviewReady:
		return fmt.Sprintf(
			"📝 Your application to <b>%s</b> at <b>%s</b> is tailored and waiting for your OK.\n<a href=\"%s\">Review and send →</a>",
			title, company, applicationURL)
	default:
		return fmt.Sprintf("<b>%s</b> at <b>%s</b>: <a href=\"%s\">Open your tracking board →</a>", title, company, trackingURL)
	}
}

// EmailNotifier delivers a nudge as an email. Like the Telegram notifier, its links
// stay on-platform. The send itself is the shared notify.Email; this package owns
// only the copy.
type EmailNotifier = batchnotify.Email[Message]

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// links rooted at origin and unsubscribe links signed by links.
func NewEmailNotifier(sender emailnotify.Sender, from, origin string, links *emailprefs.Links) *EmailNotifier {
	return batchnotify.NewEmail(sender, from, origin, links, "nudge", func(ms []Message) int64 { return ms[0].UserID },
		newEmailCopy(origin).render)
}

// emailCopy is the nudge copy for the email channel.
type emailCopy struct {
	origin string
	layout *mailtpl.Layout
}

func newEmailCopy(origin string) emailCopy {
	base := strings.TrimRight(origin, "/")
	return emailCopy{origin: base, layout: mailtpl.New(base)}
}

// render renders the batch as one mail: a single nudge keeps its own wording, a
// batch shares one list template. Every message of a batch carries the group's
// kind, so the first one names it.
func (n emailCopy) render(ms []Message, unsubscribe string) (subject, htmlBody, textBody string) {
	if len(ms) == 1 {
		return n.renderOne(ms[0], unsubscribe)
	}
	return n.renderBatch(ms[0].Kind, ms, unsubscribe)
}

// batchBody is what the batch template renders from. Jobs carries the source data,
// escaped in context by html/template.
type batchBody struct {
	Jobs []mailtpl.Job
	More int
	Lead string
	URL  string
	CTA  string
}

// batchTemplate is the list body every kind's batch shares: the jobs, one lead
// sentence and a single call to action. The kinds differ in copy, not layout.
var batchTemplate = template.Must(mailtpl.Partials().New("nudge-batch").Parse(`
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
  {{range $i, $job := .Jobs}}
  <tr><td class="m-row" style="padding:14px 0;{{if $i}}border-top:1px solid #e4e4e4;{{end}}">{{template "job-row" $job}}</td></tr>
  {{end}}
</table>
<div style="height:18px;"></div>
{{template "p" .Lead}}
{{if gt .More 0}}{{template "muted" (printf "+ %d more not listed here." .More)}}{{end}}
{{template "button" (mailLink .URL .CTA)}}`))

// renderBatch builds the multi-nudge mail for one kind.
func (n emailCopy) renderBatch(kind string, ms []Message, unsubscribe string) (subject, htmlBody, textBody string) {
	shown, more := notify.Listed(ms)
	rows := make([]mailtpl.Job, 0, len(shown))
	for _, m := range shown {
		rows = append(rows, mailtpl.NewJob(m.JobTitle, m.Company, "", n.jobURL(m)))
	}

	subject, head, pre, lead := n.batchCopy(kind, len(ms))
	url, cta := n.batchCTA(kind)

	var content bytes.Buffer
	// The template is a trusted constant and the data is escaped in context, so this
	// fails only on a template bug — caught by the golden previews.
	_ = batchTemplate.Execute(&content, batchBody{Jobs: rows, More: more, Lead: lead, URL: url, CTA: cta})

	htmlBody = n.layout.Render(mailtpl.Body{
		Preheader:      pre,
		Heading:        head,
		Content:        template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above
		Footer:         "You’re getting this because you are tracking these applications on freehire.",
		UnsubscribeURL: unsubscribe,
	})

	var b strings.Builder
	b.WriteString(lead + "\n\n")
	for _, m := range shown {
		b.WriteString("- " + m.JobTitle)
		if m.Company != "" {
			b.WriteString(" — " + m.Company)
		}
		b.WriteString("\n  " + n.jobURL(m) + "\n")
	}
	if more > 0 {
		fmt.Fprintf(&b, "\n+ %d more\n", more)
	}
	fmt.Fprintf(&b, "\n%s: %s\n", cta, url)
	return subject, htmlBody, b.String()
}

// batchCopy is the per-kind wording for a batch of `count` nudges.
func (n emailCopy) batchCopy(kind string, count int) (subject, head, pre, lead string) {
	switch kind {
	case KindFollowUp:
		return fmt.Sprintf("Time to follow up on %d applications", count),
			"Worth a follow-up?",
			fmt.Sprintf("%d applications have gone quiet", count),
			fmt.Sprintf("Nothing has moved on %d of your applications.", count)
	case KindInterviewPrep:
		return fmt.Sprintf("Prepare for %d interviews", count),
			"Ready to rehearse?",
			fmt.Sprintf("%d interviews are coming up", count),
			"Your interviews are coming up. A rehearsal beats a re-read."
	case KindJobClosed:
		return fmt.Sprintf("Closed: %d jobs you were tracking", count),
			fmt.Sprintf("%d jobs were closed", count),
			fmt.Sprintf("%d jobs you were tracking have closed", count),
			"These listings were closed. They are off the board, so they are fewer things to wait on."
	case KindAutoApplySubmitted:
		return fmt.Sprintf("Submitted: %d applications", count),
			fmt.Sprintf("%d applications submitted", count),
			fmt.Sprintf("Auto-apply submitted %d applications for you", count),
			"Auto-apply submitted these applications for you."
	case KindAutoApplyBlocked:
		return fmt.Sprintf("Needs your attention: %d applications", count),
			fmt.Sprintf("%d applications need your attention", count),
			fmt.Sprintf("Auto-apply couldn't finish %d applications", count),
			"Auto-apply couldn't finish these applications — a required question on each one needs your own answer."
	case KindAutoApplyFailed:
		return fmt.Sprintf("Couldn't submit: %d applications", count),
			fmt.Sprintf("%d applications couldn't be submitted", count),
			fmt.Sprintf("Auto-apply couldn't submit %d applications", count),
			"Auto-apply couldn't submit these applications, and won't try again."
	case KindAutoApplyReviewReady:
		return fmt.Sprintf("Ready to send: %d applications", count),
			fmt.Sprintf("%d applications are waiting for your OK", count),
			fmt.Sprintf("%d tailored applications are waiting for your OK", count),
			"Auto-apply tailored your CV for these jobs. Nothing is sent until you approve each one."
	default:
		return fmt.Sprintf("%d updates on jobs you are tracking", count),
			"An update on your applications",
			"An update on jobs you are tracking",
			fmt.Sprintf("%d jobs you are tracking were updated.", count)
	}
}

// batchCTA is the mail's single action, per batchDestination, tagged with an email
// UTM source.
func (n emailCopy) batchCTA(kind string) (url, label string) {
	path, label := batchDestination(kind)
	return n.origin + path + "?utm_source=email", label
}

// jobURL is the on-platform freehire job page, tagged with an email UTM source.
func (n emailCopy) jobURL(m Message) string {
	return n.origin + "/jobs/" + m.Slug + "?utm_source=email"
}

// nudgeBody is what the body templates render from. Job carries the source data,
// escaped in context by html/template. Job.URL is the job's own page, while URL is
// where the call to action leads, usually the tracking board.
type nudgeBody struct {
	Job        mailtpl.Job
	DaysSilent int
	URL        string
	CTA        string
}

// bodies holds one block per nudge kind, selected by name in render; the heading
// and chrome belong to the shell.
var bodies = template.Must(mailtpl.Partials().New("nudge").Parse(`
{{define "followup"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" (printf "Nothing has moved here in %d days." .DaysSilent)}}
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "interview"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" "Your interview is coming up. A rehearsal beats a re-read."}}
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "closed"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" "This listing was closed. It is off the board, so it is one less thing to wait on."}}
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "plain"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "auto_apply_submitted"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" "Auto-apply submitted this application for you."}}
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "auto_apply_blocked"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" "Auto-apply couldn't finish this application — a required question needs your own answer."}}
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "auto_apply_failed"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" "Auto-apply couldn't submit this application, and won't try again."}}
{{template "button" (mailLink .URL .CTA)}}{{end}}

{{define "auto_apply_review_ready"}}{{template "job-row" .Job}}
<div style="height:18px;"></div>
{{template "p" "Auto-apply tailored your CV for this job. Nothing is sent until you approve it."}}
{{template "button" (mailLink .URL .CTA)}}{{end}}
`))

func (n emailCopy) renderOne(m Message, unsubscribe string) (subject, htmlBody, textBody string) {
	trackingURL := n.origin + "/my/tracking?utm_source=email"
	jobURL := n.origin + "/jobs/" + m.Slug + "?utm_source=email"
	applicationURL := n.origin + "/my/tracking/" + m.Slug + "?utm_source=email"

	// block names the body template; head and pre are the heading and inbox preview.
	var block, head, pre string
	// The tracking board is the default destination; job-closed overrides it.
	data := nudgeBody{
		Job:        mailtpl.NewJob(m.JobTitle, m.Company, "", jobURL),
		DaysSilent: m.DaysSilent,
		URL:        trackingURL,
		CTA:        "Open your tracking board",
	}

	switch m.Kind {
	case KindFollowUp:
		block, head, pre = "followup", "Worth a follow-up?", "Nothing has moved on this application"
		subject = fmt.Sprintf("Time to follow up: %s at %s", m.JobTitle, m.Company)
		textBody = fmt.Sprintf("It's been %d days since anything moved on %s at %s.\n\nOpen your tracking board: %s\n",
			m.DaysSilent, m.JobTitle, m.Company, trackingURL)
	case KindInterviewPrep:
		block, head, pre = "interview", "Ready to rehearse?", "Your interview is coming up"
		subject = fmt.Sprintf("Prepare for your interview: %s at %s", m.JobTitle, m.Company)
		textBody = fmt.Sprintf("You're interviewing for %s at %s.\n\nOpen your tracking board: %s\n",
			m.JobTitle, m.Company, trackingURL)
	case KindJobClosed:
		block, head, pre = "closed", "This job was closed", "A job you were tracking has closed"
		subject = fmt.Sprintf("Closed: %s at %s", m.JobTitle, m.Company)
		data.URL, data.CTA = jobURL, "Open the job"
		textBody = fmt.Sprintf("%s at %s was closed.\n\nOpen the job: %s\n", m.JobTitle, m.Company, jobURL)
	case KindAutoApplySubmitted:
		block, head, pre = "auto_apply_submitted", "Application submitted", "Auto-apply submitted your application"
		subject = fmt.Sprintf("Submitted: %s at %s", m.JobTitle, m.Company)
		data.URL, data.CTA = applicationURL, "Open your application"
		textBody = fmt.Sprintf("Auto-apply submitted your application to %s at %s.\n\nOpen your application: %s\n",
			m.JobTitle, m.Company, data.URL)
	case KindAutoApplyBlocked:
		block, head, pre = "auto_apply_blocked", "Needs your attention", "Auto-apply couldn't finish this application"
		subject = fmt.Sprintf("Needs your attention: %s at %s", m.JobTitle, m.Company)
		data.URL, data.CTA = applicationURL, "Open your application"
		textBody = fmt.Sprintf("Auto-apply couldn't finish %s at %s — a required question needs your own answer.\n\nOpen your application: %s\n",
			m.JobTitle, m.Company, data.URL)
	case KindAutoApplyFailed:
		block, head, pre = "auto_apply_failed", "Auto-apply couldn't submit this application", "Auto-apply couldn't submit this application"
		subject = fmt.Sprintf("Couldn't submit: %s at %s", m.JobTitle, m.Company)
		data.URL, data.CTA = applicationURL, "Open your application"
		textBody = fmt.Sprintf("Auto-apply couldn't submit %s at %s, and won't try again.\n\nOpen your application: %s\n",
			m.JobTitle, m.Company, data.URL)
	case KindAutoApplyReviewReady:
		block, head, pre = "auto_apply_review_ready", "Ready to send", "Your application is waiting for your OK"
		subject = fmt.Sprintf("Ready to send: %s at %s", m.JobTitle, m.Company)
		data.URL, data.CTA = applicationURL, "Review and send"
		textBody = fmt.Sprintf("Auto-apply tailored your CV for %s at %s. Nothing is sent until you approve it.\n\nReview and send: %s\n",
			m.JobTitle, m.Company, data.URL)
	default:
		block, head, pre = "plain", fmt.Sprintf("%s at %s", m.JobTitle, m.Company), "An update on a job you are tracking"
		subject = fmt.Sprintf("%s at %s", m.JobTitle, m.Company)
		textBody = fmt.Sprintf("Open your tracking board: %s\n", trackingURL)
	}

	var content bytes.Buffer
	// The templates are trusted constants and the data is escaped in context, so
	// this fails only on a template bug — caught by the golden previews.
	_ = bodies.ExecuteTemplate(&content, block, data)

	htmlBody = n.layout.Render(mailtpl.Body{
		Preheader:      pre,
		Heading:        head,
		UnsubscribeURL: unsubscribe,
		Content:        template.HTML(content.String()), //nolint:gosec // rendered by the trusted templates above
		Footer:         "You’re getting this because you are tracking this application on freehire.",
	})
	return subject, htmlBody, textBody
}
