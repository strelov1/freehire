// Package emailnotify is the email implementation of notify.Notifier: it renders
// a filter-subscription digest into an HTML + plain-text email and sends it via a
// Sender (AWS SES in production). It is the email-channel sibling of
// internal/telegramnotify; the matching engine depends only on notify.Notifier, so
// this package is an additive channel, not a change to the engine.
package emailnotify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/mailtpl"
	"github.com/strelov1/freehire/internal/notify"
)

// Compile-time guarantee that Notifier satisfies the channel abstraction.
var _ notify.Notifier = (*Notifier)(nil)

// Sender is the email transport: it delivers one rendered message (subject + HTML
// and plain-text bodies) from `from` to `to`. *Client (AWS SES) satisfies it in
// production; tests inject a fake so rendering is verified without touching AWS.
type Sender interface {
	Send(ctx context.Context, from, to, subject, htmlBody, textBody string) error
}

// Notifier renders a digest to an email and sends it from `from` through the
// Sender. Digest links point at the on-platform freehire job page
// (jobBaseURL/jobs/<slug>) so notifications keep the user on the platform and
// never expose a source URL that may be login-gated.
type Notifier struct {
	sender     Sender
	from       string
	jobBaseURL string
	layout     *mailtpl.Layout
}

// NewNotifier builds a Notifier sending from `from` through sender, with links
// rooted at jobBaseURL (the frontend origin).
func NewNotifier(sender Sender, from, jobBaseURL string) *Notifier {
	base := strings.TrimRight(jobBaseURL, "/")
	return &Notifier{sender: sender, from: from, jobBaseURL: base, layout: mailtpl.New(base)}
}

// Send renders the digest and delivers it to the email address in dest. The
// channel argument is ignored — the worker routes only the email channel to this
// notifier.
func (n *Notifier) Send(ctx context.Context, _ string, dest string, d notify.Digest) error {
	e := n.render(d)
	return n.sender.Send(ctx, n.from, dest, e.subject, e.html, e.text)
}

// renderedEmail is a digest rendered into the three parts a Sender needs.
type renderedEmail struct {
	subject, html, text string
}

// htmlData is the data the HTML template renders. Every field is emitted in an
// escaping context by html/template, which is the injection guard for the
// user/source-derived job titles and company names.
type htmlData struct {
	Jobs      []mailtpl.Job
	More      int
	ManageURL string
}

func (n *Notifier) render(d notify.Digest) renderedEmail {
	rows := make([]mailtpl.Job, 0, len(d.Jobs))
	for _, j := range d.Jobs {
		rows = append(rows, mailtpl.NewJob(j.Title, j.Company, j.SalaryString(), n.jobURL(j)))
	}
	// Digest.Jobs is already capped by the engine (Config.DigestCap); Total is the
	// true count, so the remainder becomes the "and N more" tail.
	more := d.Total - len(d.Jobs)
	if more < 0 {
		more = 0
	}

	subject := fmt.Sprintf(`%d new job%s for "%s"`, d.Total, notify.Plural(d.Total), d.SavedSearchName)

	var b bytes.Buffer
	// The template is a trusted constant and the data is escaped in context, so
	// Execute can only fail on a template bug — surfaced by the render tests.
	_ = htmlTemplate.Execute(&b, htmlData{Jobs: rows, More: more, ManageURL: n.manageURL()})

	html := n.layout.Render(mailtpl.Body{
		Preheader: fmt.Sprintf("%d new job%s matching your %q alert", d.Total, notify.Plural(d.Total), d.SavedSearchName),
		Heading: fmt.Sprintf("%d new job%s for “%s”",
			d.Total, notify.Plural(d.Total), d.SavedSearchName),
		Content: template.HTML(b.String()), //nolint:gosec // rendered by the trusted template below, which escaped every field in context
		// The shell already carries the notification-settings link, so the footer
		// only has to answer "why am I getting this".
		Footer: "You’re getting this because you set up a job alert on freehire.",
	})

	return renderedEmail{subject: subject, html: html, text: n.renderText(d, rows, more)}
}

// renderText builds the plain-text alternative, mirroring the HTML body so
// non-HTML clients (and spam scorers) see the same content.
func (n *Notifier) renderText(d notify.Digest, rows []mailtpl.Job, more int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d new job%s for %q\n\n", d.Total, notify.Plural(d.Total), d.SavedSearchName)
	for _, l := range rows {
		b.WriteString("- " + l.Title)
		if l.Company != "" {
			b.WriteString(" — " + l.Company)
		}
		if l.Salary != "" {
			b.WriteString(" · " + l.Salary)
		}
		b.WriteString("\n  " + l.URL + "\n")
	}
	if more > 0 {
		fmt.Fprintf(&b, "\n+ %d more at %s\n", more, n.manageURL())
	}
	b.WriteString("\nManage your alerts: " + n.manageURL() + "\n")
	return b.String()
}

// manageURL is the saved-search settings page, where the digest sends anyone who
// wants more results or fewer mails.
func (n *Notifier) manageURL() string { return n.jobBaseURL + "/my/notifications" }

// jobURL is the on-platform freehire job page for a digest job, tagged with an
// email UTM source so the channel's traffic is attributable. Slugs are our own
// normalized values, so the URL needs no escaping.
func (n *Notifier) jobURL(j notify.DigestJob) string {
	return n.jobBaseURL + "/jobs/" + j.Slug + "?utm_source=email"
}

// htmlTemplate is the digest body — the job list only. The surrounding chrome
// (header, card, footer) belongs to mailtpl, so this template holds nothing that
// another mail would also need.
//
// The list is a table rather than stacked divs because Outlook collapses the
// margins between block elements unpredictably; table cell padding it honours.
var htmlTemplate = template.Must(mailtpl.Partials().New("digest").Parse(`
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
  {{range $i, $job := .Jobs}}
  <tr><td class="m-row" style="padding:14px 0;{{if $i}}border-top:1px solid #e4e4e4;{{end}}">{{template "job-row" $job}}</td></tr>
  {{end}}
</table>
{{if gt .More 0}}
<div style="padding-top:20px;">{{template "button-right" (mailLink .ManageURL (printf "View all — %d more" .More))}}</div>
{{end}}`))
