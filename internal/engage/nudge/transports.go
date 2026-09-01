package nudge

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
// A 403 is translated to ErrRecipientGone, this engine's vocabulary for a chat
// permanently closed to us, so the runner unlinks it instead of retrying a send
// that cannot land. Same translation the digest and reminder notifiers do.
func (n *TelegramNotifier) Send(ctx context.Context, _ string, dest, kind string, ms []Message) error {
	if len(ms) == 0 {
		return nil
	}
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("nudge: invalid telegram chat id %q: %w", dest, err)
	}
	err = n.client.SendMessage(ctx, chatID, n.render(kind, ms))
	if errors.Is(err, telegramnotify.ErrChatUnreachable) {
		return fmt.Errorf("%w: %w", ErrRecipientGone, err)
	}
	return err
}

// render builds the HTML body: one nudge keeps the sentence it has always been,
// several become a headline and a list bounded twice — by notify.ListLimit for
// readability, and by Telegram's own MaxMessageLen, because an oversized body is
// rejected deterministically and every retry re-fails.
func (n *TelegramNotifier) render(kind string, ms []Message) string {
	if len(ms) == 1 {
		return n.renderOne(ms[0])
	}

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

// batchHeadline is the one line that says what this batch is about. It is escaped
// nowhere because it holds no source data — only a count.
func (n *TelegramNotifier) batchHeadline(kind string, count int) string {
	switch kind {
	case KindFollowUp:
		return fmt.Sprintf("👋 <b>%d</b> of your applications have gone quiet. Worth a follow-up?", count)
	case KindInterviewPrep:
		return fmt.Sprintf("🎯 You're interviewing for <b>%d</b> roles. Ready to rehearse?", count)
	case KindJobClosed:
		return fmt.Sprintf("📪 <b>%d</b> jobs you were tracking were closed.", count)
	default:
		return fmt.Sprintf("<b>%d</b> updates on jobs you are tracking.", count)
	}
}

// jobLine renders one nudged job as a bullet linking to its freehire page.
func (n *TelegramNotifier) jobLine(m Message) string {
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
func (n *TelegramNotifier) moreLine(more int, kind string) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=%q>+ %d more</a>", n.batchURL(kind), more)
}

// batchURL is where a batch sends the reader, per batchDestination.
func (n *TelegramNotifier) batchURL(kind string) string {
	path, _ := batchDestination(kind)
	return n.origin + path
}

// batchDestination is the one place that decides where a batch of `kind` leads and
// what the action is called. Most kinds are about applications, so the tracking
// board; job-closed has nothing left to track, so the saved list. Both channels read
// it, so the mail's button and the bot's tail can never point different ways.
func batchDestination(kind string) (path, label string) {
	if kind == KindJobClosed {
		return "/my/activity", "Open your saved jobs"
	}
	return "/my/tracking", "Open your tracking board"
}

// renderOne is the single-nudge body, kept per kind and unchanged, because a batch
// of one must be indistinguishable from what shipped before grouping.
func (n *TelegramNotifier) renderOne(m Message) string {
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
	layout *mailtpl.Layout
}

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// links rooted at origin.
func NewEmailNotifier(sender emailnotify.Sender, from, origin string) *EmailNotifier {
	base := strings.TrimRight(origin, "/")
	return &EmailNotifier{sender: sender, from: emailnotify.From(senderName, from), origin: base, layout: mailtpl.New(base)}
}

// Send renders the batch as one mail and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest, kind string, ms []Message) error {
	if len(ms) == 0 {
		return nil
	}
	var subject, htmlBody, textBody string
	if len(ms) == 1 {
		subject, htmlBody, textBody = n.render(ms[0])
	} else {
		subject, htmlBody, textBody = n.renderBatch(kind, ms)
	}
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
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

// batchTemplate is the list body every kind's batch shares: the jobs, the one
// sentence saying what happened to them, and a single call to action. The kinds
// differ in copy, not in layout, so one template serves all three — unlike the
// single-nudge blocks above, whose lead sentences sit inside the markup.
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
func (n *EmailNotifier) renderBatch(kind string, ms []Message) (subject, htmlBody, textBody string) {
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
		Preheader: pre,
		Heading:   head,
		Content:   template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above
		Footer:    "You’re getting this because you are tracking these applications on freehire.",
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
func (n *EmailNotifier) batchCopy(kind string, count int) (subject, head, pre, lead string) {
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
	default:
		return fmt.Sprintf("%d updates on jobs you are tracking", count),
			"An update on your applications",
			"An update on jobs you are tracking",
			fmt.Sprintf("%d jobs you are tracking were updated.", count)
	}
}

// batchCTA is the mail's single action, per batchDestination, tagged with an email
// UTM source so the channel's traffic is attributable.
func (n *EmailNotifier) batchCTA(kind string) (url, label string) {
	path, label := batchDestination(kind)
	return n.origin + path + "?utm_source=email", label
}

// jobURL is the on-platform freehire job page, tagged with an email UTM source.
func (n *EmailNotifier) jobURL(m Message) string {
	return n.origin + "/jobs/" + m.Slug + "?utm_source=email"
}

// nudgeBody is what the body templates render from. Job carries the source data,
// escaped in context by html/template.
//
// Job.URL is the job's own page — what the row links to — while URL is where the
// call to action sends the reader, usually the tracking board. They differ because
// the nudge is about the application, not the listing.
type nudgeBody struct {
	Job        mailtpl.Job
	DaysSilent int
	URL        string
	CTA        string
}

// bodies holds one block per nudge kind, selected by name in render. Each renders
// the lead sentence and the call to action; the heading and the chrome around them
// belong to the shell.
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
`))

func (n *EmailNotifier) render(m Message) (subject, htmlBody, textBody string) {
	trackingURL := n.origin + "/my/tracking?utm_source=email"
	jobURL := n.origin + "/jobs/" + m.Slug + "?utm_source=email"

	// block names the body template; head and pre are the shell's heading and the
	// inbox preview line.
	var block, head, pre string
	// Most nudges are about the application, so the tracking board is the default
	// destination; job-closed overrides it, having nothing left to track.
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
		Preheader: pre,
		Heading:   head,
		Content:   template.HTML(content.String()), //nolint:gosec // rendered by the trusted templates above
		Footer:    "You’re getting this because you are tracking this application on freehire.",
	})
	return subject, htmlBody, textBody
}
