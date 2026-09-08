package report

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// EmailSender is the slice of the SES transport the notifier needs; *emailnotify.Client
// satisfies it. The interface stays declared here — a consumer owns its own seam — but it
// now names emailnotify.Message, so the import this package used to avoid is back.
//
// That is the price of one message type. Every mail in the product now goes out through
// one Send so that one check can refuse a silenceable mail carrying no way out of it, and
// a private copy of the message shape per sender would put that check back to being per
// sender, which is where it was missing in the first place.
type EmailSender interface {
	Send(ctx context.Context, m emailnotify.Message) error
}

// Compile-time proof that MailNotifier satisfies the seam the use cases depend on.
var _ ReporterNotifier = (*MailNotifier)(nil)

// MailNotifier emails a reporter what a moderator decided about their report. It renders the
// three outcomes a reporter can receive — the job was removed, the report was acted on with
// the listing left up, or nothing changed — because "we reviewed it" alone tells them
// nothing they could not have assumed from silence.
// Silencer answers whether an account has turned a group of mail off.
// *emailprefs.Service satisfies it.
//
// This notice has no selection query to gate — a moderator decides, and the mail
// goes out on that request path — so the check is here rather than in SQL, where
// every queue-driven sender's gate lives. Without it the footer would advertise an
// unsubscribe control this mail does not honour, which is worse than not offering
// one.
type Silencer interface {
	Silenced(ctx context.Context, userID int64, g emailprefs.Group) bool
}

type MailNotifier struct {
	sender   EmailSender
	from     string
	baseURL  string
	layout   *mailtpl.Layout
	links    *emailprefs.Links
	silenced Silencer
}

// NewMailNotifier builds a MailNotifier sending from `from` through sender. baseURL is the
// site origin the reported job is linked under; links signs the unsubscribe URL.
func NewMailNotifier(sender EmailSender, from, baseURL string, links *emailprefs.Links, silenced Silencer) *MailNotifier {
	base := strings.TrimRight(baseURL, "/")
	return &MailNotifier{
		sender:   sender,
		from:     senderFrom(from),
		baseURL:  base,
		layout:   mailtpl.New(base),
		links:    links,
		silenced: silenced,
	}
}

// senderFrom puts the product's name on the From header. It is spelled out here
// rather than imported from emailnotify because this package deliberately does not
// depend on that one (see EmailSender above) — one duplicated word is cheaper than
// dragging the AWS dependency graph into moderation.
func senderFrom(address string) string {
	parsed, err := mail.ParseAddress(address)
	if err != nil {
		return address
	}
	return (&mail.Address{Name: "freehire", Address: parsed.Address}).String()
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

var noticeHTML = template.Must(mailtpl.Partials().New("notice").Parse(`
{{template "p" .Lead}}
{{if .Note}}{{template "p" .Action}}
{{template "quote" .Note}}{{end}}
{{if .Quoted}}{{template "muted" (printf "You reported: %s" .Quoted)}}{{end}}
{{template "button" (mailLink .JobURL "Open the listing")}}
{{template "muted" "Thanks for flagging it — reports are how the listings stay honest."}}
`))

// NotifyDecision renders the outcome and sends it. A transport failure is returned as-is:
// this type does not decide what a failed notice means, the use case does.
func (m *MailNotifier) NotifyDecision(ctx context.Context, d Decision) error {
	// Honour the switch this mail's own footer offers. Returning nil rather than an
	// error: not sending because somebody asked us not to is a success, and the use
	// case above records "notified: false", which is the truth.
	if m.silenced != nil && m.silenced.Silenced(ctx, d.UserID, emailprefs.GroupActivity) {
		return nil
	}
	subject, heading, mail := m.compose(d)

	var content bytes.Buffer
	if err := noticeHTML.Execute(&content, mail); err != nil {
		return err
	}
	unsubscribe, err := m.links.For(d.UserID, emailprefs.GroupActivity)
	if err != nil {
		return fmt.Errorf("report: unsubscribe link for user %d: %w", d.UserID, err)
	}
	html := m.layout.Render(mailtpl.Body{
		Preheader:      subject,
		Heading:        heading,
		Content:        template.HTML(content.String()), //nolint:gosec // rendered by the trusted template above, which escaped the reporter's own words in context
		Footer:         "You’re getting this because you reported a listing on freehire.",
		UnsubscribeURL: unsubscribe,
	})
	return m.sender.Send(ctx, emailnotify.Message{
		From: m.from, To: d.Email, Subject: subject, HTML: html,
		Text:  textBody(mail) + emailprefs.TextFooter(unsubscribe),
		Group: emailprefs.GroupActivity, UnsubscribeURL: unsubscribe,
	})
}

// compose picks the subject, the shell heading, and the prose for one outcome.
func (m *MailNotifier) compose(d Decision) (string, string, noticeMail) {
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
		return "Your report on " + title + " — no change", "We left the listing up", mail
	case d.JobClosed:
		mail.Lead = "You reported " + title + ", and we have removed it from the listings."
		mail.Action = "What we found:"
		return "We removed the job you reported", "We removed the listing", mail
	default:
		mail.Lead = "We looked into your report on " + title + " and acted on it. The listing is still up."
		mail.Action = "What we did:"
		return "We looked into your report on " + title, "We acted on your report", mail
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
