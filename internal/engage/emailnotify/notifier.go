// Package emailnotify is the email implementation of notify.Notifier: it renders
// a filter-subscription digest into an HTML + plain-text email and sends it via a
// Sender (AWS SES in production).
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

var _ notify.Notifier = (*Notifier)(nil)

// Sender is the email transport: it delivers one Message. *Client (AWS SES)
// satisfies it in production; tests inject a fake.
//
// One method so every mail passes through a single call, which lets one validate()
// refuse a silenceable mail with no way out of it, on templated and hand-built
// rendering paths alike.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Notifier renders a digest to an email and sends it from `from` through the
// Sender. Digest links point at the on-platform job page (jobBaseURL/jobs/<slug>),
// never a source URL that may be login-gated.
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
// channel argument is ignored.
func (n *Notifier) Send(ctx context.Context, _ string, dest string, m notify.Message) error {
	unsubscribe, err := n.links.For(m.UserID, emailprefs.GroupAlerts)
	if err != nil {
		return fmt.Errorf("emailnotify: unsubscribe link for user %d: %w", m.UserID, err)
	}
	e := n.render(m, unsubscribe)
	return SoftSkipIfPaused(n.sender.Send(ctx, Message{
		From:           n.from,
		To:             dest,
		Subject:        e.subject,
		HTML:           e.html,
		Text:           e.text,
		Group:          emailprefs.GroupAlerts,
		UnsubscribeURL: unsubscribe,
		Campaign:       digestCampaign,
	}), notify.ErrChannelNotConfigured)
}

// digestCampaign is the engagement-tracking name of the saved-search digest.
const digestCampaign = "alerts-digest"

// renderedEmail is a digest rendered into the three parts a Sender needs.
type renderedEmail struct {
	subject, html, text string
}

// htmlData is the data the HTML template renders. html/template escapes every
// field in context, which guards the user/source-derived titles and company names.
type htmlData struct {
	Sections []section
}

// section is one digest's part of the mail. Title is empty when the mail carries a
// single digest, whose name is already the mail's heading.
type section struct {
	Title      string
	Jobs       []mailtpl.Job
	More       int
	ViewAllURL string
}

func (n *Notifier) section(d notify.Digest, titled bool) section {
	listed := d.Listed()
	rows := make([]mailtpl.Job, 0, len(listed))
	for _, j := range listed {
		rows = append(rows, mailtpl.NewJob(j.Title, j.Company, j.SalaryString(), n.jobURL(j)))
	}
	// A message itemizes at most notify.ListLimit jobs; the remainder becomes the
	// "and N more" tail, linking to the page with the whole match set.
	s := section{Jobs: rows, More: max(d.Total-len(listed), 0), ViewAllURL: n.viewAllURL(d)}
	if titled {
		s.Title = fmt.Sprintf("%s for “%s”", notify.JobCount(d.Total), d.SavedSearchName)
	}
	return s
}

func (n *Notifier) render(m notify.Message, unsubscribeURL string) renderedEmail {
	titled := len(m.Digests) > 1
	sections := make([]section, len(m.Digests))
	for i, d := range m.Digests {
		sections[i] = n.section(d, titled)
	}

	var subject, preheader, heading, textHeading string
	if titled {
		subject = fmt.Sprintf("%s from %d job alerts", notify.JobCount(m.Total()), len(m.Digests))
		preheader = fmt.Sprintf("%s across %d of your job alerts", notify.JobCount(m.Total()), len(m.Digests))
		heading, textHeading = subject, subject
	} else {
		d := m.Digests[0]
		subject = fmt.Sprintf(`%s for "%s"`, notify.JobCount(d.Total), d.SavedSearchName)
		preheader = fmt.Sprintf("%s matching your %q alert", notify.JobCount(d.Total), d.SavedSearchName)
		heading = fmt.Sprintf("%s for “%s”", notify.JobCount(d.Total), d.SavedSearchName)
		textHeading = fmt.Sprintf("%s for %q", notify.JobCount(d.Total), d.SavedSearchName)
	}

	var b bytes.Buffer
	// The template is a trusted constant, so Execute can only fail on a template
	// bug, which the render tests surface.
	_ = htmlTemplate.Execute(&b, htmlData{Sections: sections})

	html := n.layout.Render(mailtpl.Body{
		Preheader: preheader,
		Heading:   heading,
		Content:   template.HTML(b.String()), //nolint:gosec // rendered by the trusted template below, which escaped every field in context
		// The shell carries the unsubscribe and settings links.
		Footer:         "You’re getting this because you set up a job alert on freehire.",
		UnsubscribeURL: unsubscribeURL,
	})

	return renderedEmail{subject: subject, html: html, text: n.renderText(textHeading, sections, unsubscribeURL)}
}

// renderText builds the plain-text alternative, mirroring the HTML body.
func (n *Notifier) renderText(heading string, sections []section, unsubscribeURL string) string {
	var b strings.Builder
	b.WriteString(heading + "\n")
	for _, s := range sections {
		if s.Title != "" {
			b.WriteString("\n" + s.Title + "\n")
		}
		b.WriteString("\n")
		for _, l := range s.Jobs {
			b.WriteString("- " + l.Title)
			if l.Company != "" {
				b.WriteString(" — " + l.Company)
			}
			if l.Salary != "" {
				b.WriteString(" · " + l.Salary)
			}
			b.WriteString("\n  " + l.URL + "\n")
		}
		if s.More > 0 {
			fmt.Fprintf(&b, "\n+ %d more at %s\n", s.More, s.ViewAllURL)
		}
	}
	b.WriteString("\nManage your alerts: " + n.manageURL() + "\n")
	b.WriteString(emailprefs.TextFooter(unsubscribeURL))
	return b.String()
}

// manageURL is the saved-search settings page.
func (n *Notifier) manageURL() string { return n.jobBaseURL + "/my/notifications" }

// viewAllURL is where the "and N more" tail leads: the digest's own in-app
// notification page. A digest whose recording failed has no id and falls back to
// the notification section.
//
// An overflowed digest (counts more than it carries) links to the saved search's
// live results, since its own page would list fewer jobs than the mail announced.
func (n *Notifier) viewAllURL(d notify.Digest) string {
	if d.Overflowed() {
		return n.jobBaseURL + d.SearchPath("email")
	}
	if d.NotificationID == 0 {
		return n.manageURL()
	}
	return n.jobBaseURL + "/my/notifications/" + strconv.FormatInt(d.NotificationID, 10) + "/jobs?utm_source=email"
}

// jobURL is the on-platform job page, tagged with an email UTM source. Slugs are
// our own normalized values, so the URL needs no escaping.
func (n *Notifier) jobURL(j notify.DigestJob) string {
	return n.jobBaseURL + "/jobs/" + j.Slug + "?utm_source=email"
}

// htmlTemplate is the digest body, the job list only; the surrounding chrome
// belongs to mailtpl.
//
// A table rather than stacked divs because Outlook collapses block margins
// unpredictably but honours cell padding.
var htmlTemplate = template.Must(mailtpl.Partials().New("digest").Parse(`
{{range $n, $s := .Sections}}
{{if $s.Title}}<h2 class="m-ink" style="margin:{{if $n}}32px{{else}}0{{end}} 0 4px;font-size:16px;line-height:1.4;font-weight:600;color:#070707;">{{$s.Title}}</h2>{{end}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
  {{range $i, $job := $s.Jobs}}
  <tr><td class="m-row" style="padding:14px 0;{{if $i}}border-top:1px solid #e4e4e4;{{end}}">{{template "job-row" $job}}</td></tr>
  {{end}}
</table>
{{if gt $s.More 0}}
<div style="padding-top:20px;">{{template "button-right" (mailLink $s.ViewAllURL (printf "View all — %d more" $s.More))}}</div>
{{end}}
{{end}}`))
