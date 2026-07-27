package emailnotify

import (
	"bytes"
	"context"
	"html/template"
	"strings"
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
	sender  Sender
	from    string
	baseURL string
}

// NewAuthMailer builds an AuthMailer sending from `from` through sender. baseURL is the
// site origin shown in the footer so a recipient can tell which product mailed them.
func NewAuthMailer(sender Sender, from, baseURL string) *AuthMailer {
	return &AuthMailer{sender: sender, from: from, baseURL: strings.TrimRight(baseURL, "/")}
}

// codeMail is the data both templates render from.
type codeMail struct {
	Code    string
	Minutes int
	BaseURL string
}

// codeTTLMinutes mirrors accounts.codeTTL. It is duplicated rather than imported to keep
// the dependency pointing one way (handler wires accounts → mailer, never the reverse);
// the copy is a display string, not the enforced deadline.
const codeTTLMinutes = 15

var verificationHTML = template.Must(template.New("verify").Parse(`
<p>Welcome to freehire.</p>
<p>Confirm your email address with this code:</p>
<p style="font-size:28px;letter-spacing:4px;font-weight:700">{{.Code}}</p>
<p>It expires in {{.Minutes}} minutes. If you did not create an account, ignore this message.</p>
<p><a href="{{.BaseURL}}">{{.BaseURL}}</a></p>
`))

var resetHTML = template.Must(template.New("reset").Parse(`
<p>Someone asked to reset the password for this freehire account.</p>
<p>Use this code to set a new one:</p>
<p style="font-size:28px;letter-spacing:4px;font-weight:700">{{.Code}}</p>
<p>It expires in {{.Minutes}} minutes. If this was not you, ignore this message — your
password has not changed.</p>
<p><a href="{{.BaseURL}}">{{.BaseURL}}</a></p>
`))

// SendVerificationCode mails a sign-up verification code.
func (m *AuthMailer) SendVerificationCode(ctx context.Context, email, code string) error {
	return m.send(ctx, email, "Confirm your freehire email", verificationHTML, code,
		"Confirm your email address with this code: "+code+
			"\nIt expires in 15 minutes. If you did not create an account, ignore this message.")
}

// SendPasswordResetCode mails a password-reset code.
func (m *AuthMailer) SendPasswordResetCode(ctx context.Context, email, code string) error {
	return m.send(ctx, email, "Reset your freehire password", resetHTML, code,
		"Use this code to set a new password: "+code+
			"\nIt expires in 15 minutes. If this was not you, ignore this message — your password has not changed.")
}

// send renders the HTML body and delivers both parts through the transport.
func (m *AuthMailer) send(ctx context.Context, email, subject string, tpl *template.Template, code, text string) error {
	var html bytes.Buffer
	if err := tpl.Execute(&html, codeMail{Code: code, Minutes: codeTTLMinutes, BaseURL: m.baseURL}); err != nil {
		return err
	}
	return m.sender.Send(ctx, m.from, email, subject, html.String(), text)
}
