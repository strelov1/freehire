package emailnotify

import (
	"bytes"
	"context"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// ApplicationMailer delivers a candidate's on-site application to the employer who posted
// the vacancy (internal/ingest/directapply). It is a separate type from AuthMailer because
// its recipient is a third party and it carries the candidate's CV, not a code.
type ApplicationMailer struct {
	sender Sender
	from   string
	layout *mailtpl.Layout
}

// NewApplicationMailer builds an ApplicationMailer sending from `from` through sender.
// baseURL is the site origin the branded shell links back to.
func NewApplicationMailer(sender Sender, from, baseURL string) *ApplicationMailer {
	return &ApplicationMailer{sender: sender, from: From(productName, from), layout: mailtpl.New(baseURL)}
}

// ApplicationMail is one application as the employer receives it. This package cannot
// import internal/ingest/directapply (the two blocks share a layer), so the handler wiring
// translates that package's Mail into this one.
type ApplicationMail struct {
	To             string // the employer account's confirmed work email
	CandidateEmail string // the Reply-To: the employer answers the candidate directly
	CompanyName    string
	JobTitle       string
	JobURL         string // the vacancy's page on this site
	Answers        []ApplicationAnswer
	Note           string
	// The CV as the employer receives it: a rendered PDF, or the file the candidate
	// uploaded to their profile, which may be a PDF or pasted text.
	CV            []byte
	CVFilename    string
	CVContentType string
}

// ApplicationAnswer is one question with the candidate's answer, both as a person reads
// them (a yes/no answer arrives as "Yes"/"No").
type ApplicationAnswer struct {
	Question string
	Answer   string
}

var applicationHTML = template.Must(mailtpl.Partials().New("application").Parse(`
{{template "lead" (printf "A candidate applied to %s." .JobTitle)}}
{{template "p-link" (mailTextLink "The vacancy on freehire:" .JobURL .JobTitle)}}
{{range .Answers}}{{template "muted" .Question}}{{template "p" .Answer}}{{end}}
{{if .Note}}{{template "muted" "Note from the candidate"}}{{template "quote" .Note}}{{end}}
{{template "p" "Their CV is attached. Reply to this email to answer the candidate directly."}}
`))

// SendApplication renders and delivers the application, the CV attached as a PDF and the
// Reply-To set to the candidate. GroupEssential with no unsubscribe link: the employer
// asked to receive applications by publishing a vacancy that takes them, and closing the
// vacancy is how they stop.
func (m *ApplicationMailer) SendApplication(ctx context.Context, a ApplicationMail) error {
	var content bytes.Buffer
	if err := applicationHTML.Execute(&content, a); err != nil {
		return err
	}
	return m.sender.Send(ctx, Message{
		From:    m.from,
		To:      a.To,
		ReplyTo: a.CandidateEmail,
		Subject: "New application: " + a.JobTitle,
		HTML: m.layout.Render(mailtpl.Body{
			Preheader: "A candidate applied to " + a.JobTitle + " on freehire",
			Heading:   "New application",
			Content:   template.HTML(content.String()),                                                                                                                                                                       //nolint:gosec // rendered by the trusted template above, which escaped every value in context
			Footer:    template.HTML("You receive this because " + template.HTMLEscapeString(a.CompanyName) + " publishes this vacancy on freehire and takes applications there. Close the vacancy to stop receiving them."), //nolint:gosec // the only interpolated value is escaped
		}),
		Text:        applicationText(a),
		Group:       emailprefs.GroupEssential,
		Attachments: []Attachment{{Filename: a.CVFilename, ContentType: a.CVContentType, Content: a.CV}},
	})
}

func applicationText(a ApplicationMail) string {
	var b strings.Builder
	b.WriteString("A candidate applied to " + a.JobTitle + ".\n" + a.JobURL + "\n\n")
	for _, ans := range a.Answers {
		b.WriteString(ans.Question + "\n" + ans.Answer + "\n\n")
	}
	if a.Note != "" {
		b.WriteString("Note from the candidate:\n" + a.Note + "\n\n")
	}
	b.WriteString("Their CV is attached. Reply to this email to answer the candidate directly.\n")
	return b.String()
}
