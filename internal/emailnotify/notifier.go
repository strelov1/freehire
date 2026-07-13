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
}

// NewNotifier builds a Notifier sending from `from` through sender, with links
// rooted at jobBaseURL (the frontend origin).
func NewNotifier(sender Sender, from, jobBaseURL string) *Notifier {
	return &Notifier{sender: sender, from: from, jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
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

// jobLine is one job's display fields, resolved once and shared by the HTML and
// text renderers so the two bodies stay in sync.
type jobLine struct {
	Title, Company, Salary, URL string
}

// htmlData is the data the HTML template renders. Every string field is emitted in
// an escaping context by html/template, which is the injection guard for the
// user/source-derived title, company, and saved-search name.
type htmlData struct {
	Preheader  string
	Total      int
	Word       string
	SearchName string
	Jobs       []jobLine
	More       int
	ManageURL  string
}

func (n *Notifier) render(d notify.Digest) renderedEmail {
	lines := make([]jobLine, 0, len(d.Jobs))
	for _, j := range d.Jobs {
		lines = append(lines, jobLine{
			Title:   j.Title,
			Company: j.Company,
			Salary:  j.SalaryString(),
			URL:     n.jobURL(j),
		})
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
	_ = htmlTemplate.Execute(&b, htmlData{
		Preheader:  fmt.Sprintf("%d new job%s matching your %q alert", d.Total, notify.Plural(d.Total), d.SavedSearchName),
		Total:      d.Total,
		Word:       "job" + notify.Plural(d.Total),
		SearchName: d.SavedSearchName,
		Jobs:       lines,
		More:       more,
		ManageURL:  n.jobBaseURL + "/my/notifications",
	})

	return renderedEmail{subject: subject, html: b.String(), text: n.renderText(d, lines, more)}
}

// renderText builds the plain-text alternative, mirroring the HTML body so
// non-HTML clients (and spam scorers) see the same content.
func (n *Notifier) renderText(d notify.Digest, lines []jobLine, more int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d new job%s for %q\n\n", d.Total, notify.Plural(d.Total), d.SavedSearchName)
	for _, l := range lines {
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
		fmt.Fprintf(&b, "\n+ %d more at %s/my/notifications\n", more, n.jobBaseURL)
	}
	b.WriteString("\nManage your alerts: " + n.jobBaseURL + "/my/notifications\n")
	return b.String()
}

// jobURL is the on-platform freehire job page for a digest job, tagged with an
// email UTM source so the channel's traffic is attributable. Slugs are our own
// normalized values, so the URL needs no escaping.
func (n *Notifier) jobURL(j notify.DigestJob) string {
	return n.jobBaseURL + "/jobs/" + j.Slug + "?utm_source=email"
}

// htmlTemplate is the digest email body: a single centered ~600px table with all
// styling inline (no <style>/external CSS, no JS, no remote images) so it renders
// across email clients. Compiled once at package load.
var htmlTemplate = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html>
<body style="margin:0;padding:0;background:#f4f5f7;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f5f7;">
  <tr><td align="center" style="padding:24px 12px;">
    <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:12px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
      <tr><td style="padding:24px 28px 8px 28px;">
        <span style="font-size:18px;font-weight:700;color:#111827;">freehire</span>
      </td></tr>
      <tr><td style="padding:8px 28px 16px 28px;">
        <h1 style="margin:0;font-size:20px;line-height:1.3;color:#111827;font-weight:600;">🔔 {{.Total}} new {{.Word}} for “{{.SearchName}}”</h1>
      </td></tr>
      {{range .Jobs}}
      <tr><td style="padding:8px 28px;">
        <a href="{{.URL}}" style="font-size:16px;font-weight:600;color:#2563eb;text-decoration:none;">{{.Title}}</a>
        {{if or .Company .Salary}}<div style="font-size:14px;color:#6b7280;padding-top:2px;">{{.Company}}{{if and .Company .Salary}} · {{end}}{{.Salary}}</div>{{end}}
      </td></tr>
      {{end}}
      {{if gt .More 0}}
      <tr><td style="padding:12px 28px 4px 28px;">
        <a href="{{.ManageURL}}" style="font-size:14px;color:#2563eb;text-decoration:none;">+ {{.More}} more — view all</a>
      </td></tr>
      {{end}}
      <tr><td style="padding:24px 28px;border-top:1px solid #e5e7eb;">
        <p style="margin:0;font-size:12px;color:#9ca3af;">
          You’re getting this because you set up a job alert on freehire.
          <a href="{{.ManageURL}}" style="color:#6b7280;">Manage your alerts</a>.
        </p>
      </td></tr>
    </table>
  </td></tr>
</table>
</body>
</html>`))
