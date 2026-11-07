package emailnotify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"time"
)

// smtpRelay sends the assembled RFC 5322 message over implicit-TLS SMTP. It exists so
// mail can leave through another provider (Resend, Postmark, ...) while SES refuses
// the account: every message is built by buildRawMessage, the same bytes SES's raw
// path carries, so the two transports cannot disagree about headers or bodies.
type smtpRelay struct {
	host, port string
	auth       smtp.Auth
	tls        *tls.Config
}

// smtpSendTimeout bounds one delivery, connect to QUIT.
const smtpSendTimeout = 30 * time.Second

// parseSMTPURL reads smtps://user:password@host:465. Only implicit TLS is accepted:
// PlainAuth over a cleartext connection would hand the password to the network.
func parseSMTPURL(raw string) (*smtpRelay, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("emailnotify: SMTP_URL: %w", err)
	}
	pass, _ := u.User.Password()
	if u.Scheme != "smtps" || u.Hostname() == "" || u.User.Username() == "" || pass == "" {
		return nil, fmt.Errorf("emailnotify: SMTP_URL must be smtps://user:password@host[:port]")
	}
	port := u.Port()
	if port == "" {
		port = "465"
	}
	return &smtpRelay{host: u.Hostname(), port: port, auth: smtp.PlainAuth("", u.User.Username(), pass, u.Hostname()),
		tls: &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}}, nil
}

func (r *smtpRelay) send(ctx context.Context, from, to string, raw []byte) error {
	// The envelope needs bare addresses; From may carry a display name.
	envFrom, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("emailnotify: smtp from %q: %w", from, err)
	}
	ctx, cancel := context.WithTimeout(ctx, smtpSendTimeout)
	defer cancel()
	d := tls.Dialer{Config: r.tls}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(r.host, r.port))
	if err != nil {
		return fmt.Errorf("emailnotify: smtp dial: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, r.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("emailnotify: smtp hello: %w", err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Auth(r.auth); err != nil {
		return fmt.Errorf("emailnotify: smtp auth: %w", err)
	}
	if err := c.Mail(envFrom.Address); err != nil {
		return fmt.Errorf("emailnotify: smtp MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("emailnotify: smtp RCPT TO %s: %w", to, err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("emailnotify: smtp DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("emailnotify: smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("emailnotify: smtp send to %s: %w", to, err)
	}
	return c.Quit()
}
