package emailnotify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordingSender captures the last rendered message instead of touching SES.
type recordingSender struct {
	from, to, subject, html, text string
	err                           error
	sends                         int
}

func (r *recordingSender) Send(_ context.Context, from, to, subject, htmlBody, textBody string) error {
	if r.err != nil {
		return r.err
	}
	r.sends++
	r.from, r.to, r.subject, r.html, r.text = from, to, subject, htmlBody, textBody
	return nil
}

func TestAuthMailer_SendsTheVerificationCode(t *testing.T) {
	sender := &recordingSender{}
	m := NewAuthMailer(sender, "no-reply@freehire.me", "https://freehire.me")

	if err := m.SendVerificationCode(context.Background(), "user@example.test", "123456"); err != nil {
		t.Fatalf("SendVerificationCode: %v", err)
	}
	if sender.sends != 1 {
		t.Fatalf("sends = %d, want 1", sender.sends)
	}
	if sender.to != "user@example.test" {
		t.Errorf("to = %q, want the account's address", sender.to)
	}
	// The header carries a display name over the configured address: a message list
	// showing a bare address renders it as "no-reply", which identifies nobody.
	if !strings.Contains(sender.from, "no-reply@freehire.me") || !strings.Contains(sender.from, "freehire") {
		t.Errorf("from = %q, want the configured address behind a readable name", sender.from)
	}
	if !strings.Contains(sender.text, "123456") {
		t.Errorf("plain-text body does not carry the code: %q", sender.text)
	}
	if !strings.Contains(sender.html, "123456") {
		t.Errorf("HTML body does not carry the code: %q", sender.html)
	}
	if sender.subject == "" {
		t.Error("the mail has no subject")
	}
}

func TestAuthMailer_SendsTheResetCode(t *testing.T) {
	sender := &recordingSender{}
	m := NewAuthMailer(sender, "no-reply@freehire.me", "https://freehire.me")

	if err := m.SendPasswordResetCode(context.Background(), "user@example.test", "654321"); err != nil {
		t.Fatalf("SendPasswordResetCode: %v", err)
	}
	if !strings.Contains(sender.text, "654321") || !strings.Contains(sender.html, "654321") {
		t.Error("the reset mail does not carry the code in both bodies")
	}
	if strings.EqualFold(sender.subject, "") {
		t.Error("the reset mail has no subject")
	}
}

func TestAuthMailer_TheTwoMailsAreDistinguishable(t *testing.T) {
	verify, reset := &recordingSender{}, &recordingSender{}
	if err := NewAuthMailer(verify, "f@x.test", "https://freehire.me").
		SendVerificationCode(context.Background(), "u@x.test", "111111"); err != nil {
		t.Fatalf("SendVerificationCode: %v", err)
	}
	if err := NewAuthMailer(reset, "f@x.test", "https://freehire.me").
		SendPasswordResetCode(context.Background(), "u@x.test", "222222"); err != nil {
		t.Fatalf("SendPasswordResetCode: %v", err)
	}
	if verify.subject == reset.subject {
		t.Errorf("both mails use the subject %q — a user cannot tell a reset from a sign-up", verify.subject)
	}
}

func TestAuthMailer_PropagatesASendFailure(t *testing.T) {
	m := NewAuthMailer(&recordingSender{err: errors.New("ses throttled")}, "f@x.test", "https://freehire.me")

	if err := m.SendVerificationCode(context.Background(), "u@x.test", "123456"); err == nil {
		t.Error("a transport failure must reach the caller, which decides whether it is fatal")
	}
}

// The code is the credential in the mail, so it must never leak into a link the mail
// client (or a proxy) might prefetch.
func TestAuthMailer_DoesNotPutTheCodeInAURL(t *testing.T) {
	sender := &recordingSender{}
	m := NewAuthMailer(sender, "f@x.test", "https://freehire.me")

	if err := m.SendVerificationCode(context.Background(), "u@x.test", "123456"); err != nil {
		t.Fatalf("SendVerificationCode: %v", err)
	}
	for _, body := range []string{sender.html, sender.text} {
		for _, line := range strings.Fields(body) {
			if strings.HasPrefix(line, "http") && strings.Contains(line, "123456") {
				t.Errorf("the code rides in a URL (%q); a prefetching client would burn it", line)
			}
		}
	}
}
