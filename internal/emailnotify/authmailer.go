package emailnotify

import (
	"bytes"
	"context"
	"html/template"

	"github.com/strelov1/freehire/internal/mailtpl"
)

// AuthMailer renders and sends the two transactional account mails: the sign-up
// verification code and the password-reset code. It is the sibling of Notifier —
// same Sender transport, different content — and satisfies accounts.CodeMailer, so the
// accounts service stays free of the AWS dependency graph.
//
// The code is never embedded in a link. A mail client, security scanner, or link
// prefetcher that follows URLs would otherwise consume a single-use credential before
// the human ever read it.
type AuthMailer struct {
	sender Sender
	from   string
	layout *mailtpl.Layout
}

// NewAuthMailer builds an AuthMailer sending from `from` through sender. baseURL is the
// site origin the branded shell links back to, so a recipient can tell which product
// mailed them.
func NewAuthMailer(sender Sender, from, baseURL string) *AuthMailer {
	return &AuthMailer{sender: sender, from: From(productName, from), layout: mailtpl.New(baseURL)}
}

// codeMail is the data both templates render from.
type codeMail struct {
	Code    string
	Minutes int
}

// codeTTLMinutes mirrors accounts.codeTTL. It is duplicated rather than imported to keep
// the dependency pointing one way (handler wires accounts → mailer, never the reverse);
// the copy is a display string, not the enforced deadline.
const codeTTLMinutes = 15

var verificationHTML = template.Must(mailtpl.Partials().New("verify").Parse(`
{{template "p" "Welcome to freehire. Confirm your email address with this code:"}}
{{template "code" .Code}}
{{template "muted" (printf "The code expires in %d minutes." .Minutes)}}
`))

var resetHTML = template.Must(mailtpl.Partials().New("reset").Parse(`
{{template "p" "Someone asked to reset the password on this freehire account. Use this code to set a new one:"}}
{{template "code" .Code}}
{{template "muted" (printf "The code expires in %d minutes." .Minutes)}}
`))

// SendVerificationCode mails a sign-up verification code.
func (m *AuthMailer) SendVerificationCode(ctx context.Context, email, code string) error {
	return m.send(ctx, email, "Confirm your freehire email", verificationHTML, code, mailtpl.Body{
		Preheader: "Your freehire confirmation code",
		Heading:   "Confirm your email address",
		Footer:    "If you did not create a freehire account, ignore this message — an account is never created without the code.",
		Essential: true,
	},
		"Confirm your email address with this code: "+code+
			"\nIt expires in 15 minutes. If you did not create an account, ignore this message.")
}

// SendPasswordResetCode mails a password-reset code.
func (m *AuthMailer) SendPasswordResetCode(ctx context.Context, email, code string) error {
	return m.send(ctx, email, "Reset your freehire password", resetHTML, code, mailtpl.Body{
		Preheader: "Your freehire password-reset code",
		Heading:   "Reset your password",
		Footer:    "If this was not you, ignore this message — your password has not changed.",
		Essential: true,
	},
		"Use this code to set a new password: "+code+
			"\nIt expires in 15 minutes. If this was not you, ignore this message — your password has not changed.")
}

// send renders the body, wraps it in the branded shell, and delivers both parts
// through the transport.
func (m *AuthMailer) send(ctx context.Context, email, subject string, tpl *template.Template, code string, body mailtpl.Body, text string) error {
	var content bytes.Buffer
	if err := tpl.Execute(&content, codeMail{Code: code, Minutes: codeTTLMinutes}); err != nil {
		return err
	}
	body.Content = template.HTML(content.String()) //nolint:gosec // rendered by the trusted template above, which escaped the code in context
	return m.sender.Send(ctx, m.from, email, subject, m.layout.Render(body), text)
}
