package emailnotify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// ApplicationStatusMail is the employer's answer to an on-site application, as the
// candidate receives it. Stage is one of the directapply stages that notify — interview,
// offer, rejected — which this package cannot import (the blocks share a layer).
type ApplicationStatusMail struct {
	To          string // the candidate
	ReplyTo     string // the employer account's confirmed work email
	CompanyName string
	JobTitle    string
	JobURL      string // the vacancy's page on this site
	Stage       string
	Message     string // the employer's own words, optional
}

// applicationStatusCopy is each stage's subject and lead. A stage outside it is refused
// rather than sent with a generic line: a letter that does not say what happened is not
// worth sending.
var applicationStatusCopy = map[string]struct{ subject, lead string }{
	"interview": {"%s would like to interview you", "%s would like to interview you for %s."},
	"offer":     {"An offer from %s", "%s would like to make you an offer for %s."},
	"rejected":  {"Your application to %s", "%s has decided not to move forward with your application for %s."},
}

var applicationStatusHTML = template.Must(mailtpl.Partials().New("application-status").Parse(`
{{template "lead" .Lead}}
{{if .Message}}{{template "muted" (printf "From %s" .CompanyName)}}{{template "quote" .Message}}{{end}}
{{template "p-link" (mailTextLink "The vacancy on freehire:" .JobURL .JobTitle)}}
{{template "p" (printf "Reply to this email to answer %s directly. Your application's status is in your tracking on freehire." .CompanyName)}}
`))

// SendStatusUpdate tells the candidate the employer moved their application, Reply-To the
// employer. GroupEssential with no unsubscribe link, like the application it answers: it is
// the employer's reply to a letter the candidate sent, and silencing it would bring back
// exactly the silence an employer on freehire is not allowed.
func (m *ApplicationMailer) SendStatusUpdate(ctx context.Context, a ApplicationStatusMail) error {
	cp, ok := applicationStatusCopy[a.Stage]
	if !ok {
		return fmt.Errorf("emailnotify: no letter for application stage %q", a.Stage)
	}
	lead := fmt.Sprintf(cp.lead, a.CompanyName, a.JobTitle)
	var content bytes.Buffer
	if err := applicationStatusHTML.Execute(&content, struct {
		ApplicationStatusMail
		Lead string
	}{a, lead}); err != nil {
		return err
	}
	return m.sender.Send(ctx, Message{
		From:    m.from,
		To:      a.To,
		ReplyTo: a.ReplyTo,
		Subject: fmt.Sprintf(cp.subject, a.CompanyName),
		HTML: m.layout.Render(mailtpl.Body{
			Preheader: lead,
			Heading:   a.JobTitle,
			Content:   template.HTML(content.String()),                                                                                                                                  //nolint:gosec // rendered by the trusted template above, which escaped every value in context
			Footer:    template.HTML("You receive this because you applied to " + template.HTMLEscapeString(a.CompanyName) + " on freehire, and they answered your application there."), //nolint:gosec // the only interpolated value is escaped
		}),
		Text:  applicationStatusText(a, lead),
		Group: emailprefs.GroupEssential,
	})
}

func applicationStatusText(a ApplicationStatusMail, lead string) string {
	var b strings.Builder
	b.WriteString(lead + "\n\n")
	if a.Message != "" {
		b.WriteString("From " + a.CompanyName + ":\n" + a.Message + "\n\n")
	}
	b.WriteString("The vacancy: " + a.JobURL + "\n\n")
	b.WriteString("Reply to this email to answer " + a.CompanyName + " directly.\n")
	return b.String()
}
