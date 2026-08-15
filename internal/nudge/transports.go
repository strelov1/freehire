package nudge

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
	layout *mailtpl.Layout
}

// NewEmailNotifier builds an EmailNotifier sending from `from` through sender, with
// links rooted at origin.
func NewEmailNotifier(sender emailnotify.Sender, from, origin string) *EmailNotifier {
	base := strings.TrimRight(origin, "/")
	return &EmailNotifier{sender: sender, from: emailnotify.From(senderName, from), origin: base, layout: mailtpl.New(base)}
}

// Send renders the nudge and delivers it to the address in dest.
func (n *EmailNotifier) Send(ctx context.Context, _ string, dest string, m Message) error {
	subject, htmlBody, textBody := n.render(m)
	return n.sender.Send(ctx, n.from, dest, subject, htmlBody, textBody)
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
