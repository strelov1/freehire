package report

import (
	"bytes"
	"context"
	"html/template"
	"strings"
	"unicode/utf8"
)

// EmailSender is the slice of the SES transport the notifier needs; *emailnotify.Client
// satisfies it. Declared here rather than imported so this package does not depend on the
// AWS dependency graph (the referral.ChannelPinger precedent).
type EmailSender interface {
	Send(ctx context.Context, from, to, subject, htmlBody, textBody string) error
}

// Compile-time proof that MailNotifier satisfies the seam the use cases depend on.
var _ ReporterNotifier = (*MailNotifier)(nil)

// MailNotifier emails a reporter what a moderator decided about their report. It renders the
// three outcomes a reporter can receive — the job was removed, the report was acted on with
// the listing left up, or nothing changed — because "we reviewed it" alone tells them
// nothing they could not have assumed from silence.
type MailNotifier struct {
	sender  EmailSender
	from    string
	baseURL string
}

// NewMailNotifier builds a MailNotifier sending from `from` through sender. baseURL is the
// site origin the reported job is linked under.
func NewMailNotifier(sender EmailSender, from, baseURL string) *MailNotifier {
	return &MailNotifier{sender: sender, from: from, baseURL: strings.TrimRight(baseURL, "/")}
}

// maxQuotedDetails bounds how much of the original report is quoted back. The reporter wrote
// it and knows what it said; the quotation is there to jog their memory, not to reprint an
// essay.
const maxQuotedDetails = 400

// noticeMail is what the templates render from. Lead and Action are prose the outcome
// decides; Quoted is the reporter's own words, already truncated.
type noticeMail struct {
	JobTitle string
	JobURL   string
	Lead     string
	Action   string
	Note     string
	Quoted   string
}

var noticeHTML = template.Must(template.New("notice").Parse(`
<p>{{.Lead}}</p>
{{if .Note}}<p>{{.Action}}</p>
<blockquote style="margin:0 0 1em;padding-left:12px;border-left:3px solid #ddd">{{.Note}}</blockquote>{{end}}
{{if .Quoted}}<p style="color:#666">You reported: {{.Quoted}}</p>{{end}}
<p><a href="{{.JobURL}}">{{.JobTitle}}</a></p>
<p style="color:#666">Thanks for flagging it — reports are how the listings stay honest.</p>
`))

// NotifyDecision renders the outcome and sends it. A transport failure is returned as-is:
// this type does not decide what a failed notice means, the use case does.
func (m *MailNotifier) NotifyDecision(ctx context.Context, d Decision) error {
	subject, mail := m.compose(d)

	var html bytes.Buffer
	if err := noticeHTML.Execute(&html, mail); err != nil {
		return err
	}
	return m.sender.Send(ctx, m.from, d.Email, subject, html.String(), textBody(mail))
}

// compose picks the subject and the prose for one outcome.
func (m *MailNotifier) compose(d Decision) (string, noticeMail) {
	title := d.JobTitle
	if title == "" {
		title = "the job you reported"
	}
	mail := noticeMail{
		JobTitle: title,
		JobURL:   m.baseURL + "/jobs/" + d.JobSlug,
		Note:     strings.TrimSpace(d.Note),
		Quoted:   truncate(strings.TrimSpace(d.Details), maxQuotedDetails),
	}

	switch {
	case d.Outcome == OutcomeDismissed:
		mail.Lead = "We looked into your report on " + title + " and left the listing as it is."
		mail.Action = "Why:"
		return "Your report on " + title + " — no change", mail
	case d.JobClosed:
		mail.Lead = "You reported " + title + ", and we have removed it from the listings."
		mail.Action = "What we found:"
		return "We removed the job you reported", mail
	default:
		mail.Lead = "We looked into your report on " + title + " and acted on it. The listing is still up."
		mail.Action = "What we did:"
		return "We looked into your report on " + title, mail
	}
}

// textBody renders the plain-text alternative. It mirrors the HTML rather than sharing a
// template with it, because the two differ in exactly the places templating makes awkward:
// no markup, and the link spelled out.
func textBody(m noticeMail) string {
	var b strings.Builder
	b.WriteString(m.Lead)
	if m.Note != "" {
		b.WriteString("\n\n" + m.Action + "\n" + m.Note)
	}
	if m.Quoted != "" {
		b.WriteString("\n\nYou reported: " + m.Quoted)
	}
	b.WriteString("\n\n" + m.JobTitle + ": " + m.JobURL)
	b.WriteString("\n\nThanks for flagging it — reports are how the listings stay honest.")
	return b.String()
}

// truncate cuts s to at most n bytes, marking the cut so the reader can tell their report
// was quoted in part. The cut lands on a rune boundary: a report written in Cyrillic or CJK
// has multi-byte runes and often no spaces to back off to, and half a rune is invalid UTF-8
// that renders as a replacement character in the reader's mail client.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	// Prefer a word boundary; fall back to a rune boundary when there is no usable space.
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	} else {
		for len(cut) > 0 && !utf8.ValidString(cut) {
			cut = cut[:len(cut)-1]
		}
	}
	return strings.TrimRight(cut, " ") + "…"
}
