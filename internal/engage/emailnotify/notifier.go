// Package emailnotify is the email implementation of notify.Notifier: it renders
// a filter-subscription digest into an HTML + plain-text email and sends it via a
// Sender (AWS SES in production). It is the email-channel sibling of
// internal/engage/telegramnotify; the matching engine depends only on notify.Notifier, so
// this package is an additive channel, not a change to the engine.
package emailnotify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/notify"
)

// Compile-time guarantee that Notifier satisfies the channel abstraction.
var _ notify.Notifier = (*Notifier)(nil)

// Sender is the email transport: it delivers one Message. *Client (AWS SES)
// satisfies it in production; tests inject a fake so rendering is verified without
// touching AWS.
//
// One method, not three. It replaced Send / SendWithReplyTo / SendWithAttachments,
// which had begun enumerating the combinations of their optional parts — and every
// mail now passing through a single call is what lets one validate() refuse a
// silenceable mail with no way out of it, on both the templated and the hand-built
// rendering paths.
type Sender interface {
	Send(ctx context.Context, m Message) error
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
	links      *emailprefs.Links
}

// NewNotifier builds a Notifier sending from `from` through sender, with links
// rooted at jobBaseURL (the frontend origin) and unsubscribe links signed by links.
func NewNotifier(sender Sender, from, jobBaseURL string, links *emailprefs.Links) *Notifier {
	base := strings.TrimRight(jobBaseURL, "/")
	return &Notifier{
		sender:     sender,
		from:       From(productName, from),
		jobBaseURL: base,
		layout:     mailtpl.New(base),
		links:      links,
	}
}

// Send renders the digest and delivers it to the email address in dest. The
// channel argument is ignored — the worker routes only the email channel to this
// notifier.
func (n *Notifier) Send(ctx context.Context, _ string, dest string, d notify.Digest) error {
	unsubscribe, err := n.links.For(d.UserID, emailprefs.GroupAlerts)
	if err != nil {
		return fmt.Errorf("emailnotify: unsubscribe link for user %d: %w", d.UserID, err)
	}
	e := n.render(d, unsubscribe)
	return n.sender.Send(ctx, Message{
		From:           n.from,
		To:             dest,
		Subject:        e.subject,
		HTML:           e.html,
		Text:           e.text,
		Group:          emailprefs.GroupAlerts,
		UnsubscribeURL: unsubscribe,
	})
}

// renderedEmail is a digest rendered into the three parts a Sender needs.
type renderedEmail struct {
	subject, html, text string
}

// htmlData is the data the HTML template renders. Every field is emitted in an
// escaping context by html/template, which is the injection guard for the
// user/source-derived job titles and company names.
type htmlData struct {
	Jobs       []mailtpl.Job
	More       int
	ViewAllURL string
}

func (n *Notifier) render(d notify.Digest, unsubscribeURL string) renderedEmail {
	listed := d.Listed()
	rows := make([]mailtpl.Job, 0, len(listed))
	for _, j := range listed {
		rows = append(rows, mailtpl.NewJob(j.Title, j.Company, j.SalaryString(), n.jobURL(j)))
	}
	// A message itemizes at most notify.ListLimit jobs while Total is the true
	// count, so the remainder becomes the "and N more" tail — which links to the
	// page carrying the whole match set, not to the alerts settings.
	more := d.Total - len(listed)
	if more < 0 {
		more = 0
	}
	viewAll := n.viewAllURL(d)

	subject := fmt.Sprintf(`%s for "%s"`, notify.JobCount(d.Total), d.SavedSearchName)

	var b bytes.Buffer
	// The template is a trusted constant and the data is escaped in context, so
	// Execute can only fail on a template bug — surfaced by the render tests.
	_ = htmlTemplate.Execute(&b, htmlData{Jobs: rows, More: more, ViewAllURL: viewAll})

	html := n.layout.Render(mailtpl.Body{
		Preheader: fmt.Sprintf("%s matching your %q alert", notify.JobCount(d.Total), d.SavedSearchName),
		Heading:   fmt.Sprintf("%s for “%s”", notify.JobCount(d.Total), d.SavedSearchName),
		Content:   template.HTML(b.String()), //nolint:gosec // rendered by the trusted template below, which escaped every field in context
		// The shell carries the unsubscribe and settings links, so the footer only
		// has to answer "why am I getting this".
		Footer:         "You’re getting this because you set up a job alert on freehire.",
		UnsubscribeURL: unsubscribeURL,
	})

	return renderedEmail{subject: subject, html: html, text: n.renderText(d, rows, more, viewAll, unsubscribeURL)}
}

// renderText builds the plain-text alternative, mirroring the HTML body so
// non-HTML clients (and spam scorers) see the same content.
func (n *Notifier) renderText(d notify.Digest, rows []mailtpl.Job, more int, viewAllURL, unsubscribeURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s for %q\n\n", notify.JobCount(d.Total), d.SavedSearchName)
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
		fmt.Fprintf(&b, "\n+ %d more at %s\n", more, viewAllURL)
	}
	b.WriteString("\nManage your alerts: " + n.manageURL() + "\n")
	// The plain-text alternative carries the way out too. A reader who sees only
	// this body has the same right to leave as one whose client renders HTML, and a
	// spam scorer reads both.
	b.WriteString("Unsubscribe: " + unsubscribeURL + "\n")
	return b.String()
}

// manageURL is the saved-search settings page, where the digest sends anyone who
// wants more results or fewer mails.
func (n *Notifier) manageURL() string { return n.jobBaseURL + "/my/notifications" }

// viewAllURL is where the "and N more" tail leads: the digest's own in-app
// notification, whose page lists every job this digest matched. A digest whose
// recording failed carries no id, and the tail falls back to the notification
// section — a weaker destination, but never a broken one.
func (n *Notifier) viewAllURL(d notify.Digest) string {
	if d.NotificationID == 0 {
		return n.manageURL()
	}
	return n.jobBaseURL + "/my/notifications/" + strconv.FormatInt(d.NotificationID, 10) + "/jobs?utm_source=email"
}

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
<div style="padding-top:20px;">{{template "button-right" (mailLink .ViewAllURL (printf "View all — %d more" .More))}}</div>
{{end}}`))
