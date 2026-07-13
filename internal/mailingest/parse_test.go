package mailingest

import (
	"strings"
	"testing"
)

const sampleMIME = "From: Acme Careers <careers@acme.com>\r\n" +
	"To: ivan@inbox.freehire.dev\r\n" +
	"Subject: Interview invite\r\n" +
	"Message-ID: <abc123@acme.com>\r\n" +
	"Date: Mon, 12 Jul 2026 10:00:00 +0000\r\n" +
	"Content-Type: multipart/alternative; boundary=\"b\"\r\n" +
	"\r\n" +
	"--b\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"Hello Ivan, plain body.\r\n" +
	"--b\r\n" +
	"Content-Type: text/html\r\n" +
	"\r\n" +
	"<p>Hello Ivan, html body.</p>\r\n" +
	"--b--\r\n"

func TestParse(t *testing.T) {
	p, err := Parse([]byte(sampleMIME))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.FromAddr != "careers@acme.com" {
		t.Errorf("FromAddr = %q", p.FromAddr)
	}
	if p.FromName != "Acme Careers" {
		t.Errorf("FromName = %q", p.FromName)
	}
	if p.Subject != "Interview invite" {
		t.Errorf("Subject = %q", p.Subject)
	}
	if p.MessageID != "abc123@acme.com" {
		t.Errorf("MessageID = %q (angles should be trimmed)", p.MessageID)
	}
	if !strings.Contains(p.TextBody, "plain body") {
		t.Errorf("TextBody = %q", p.TextBody)
	}
	if !strings.Contains(p.HTMLBody, "html body") {
		t.Errorf("HTMLBody = %q", p.HTMLBody)
	}
	if p.ReceivedAt.IsZero() {
		t.Error("ReceivedAt not parsed")
	}
}

func TestParseMissingHeaders(t *testing.T) {
	// No Message-ID, no Date, plain single-part: best-effort, no error.
	raw := "From: solo@x.io\r\nSubject: hi\r\n\r\njust text\r\n"
	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.MessageID != "" {
		t.Errorf("MessageID = %q, want empty", p.MessageID)
	}
	if !p.ReceivedAt.IsZero() {
		t.Errorf("ReceivedAt = %v, want zero", p.ReceivedAt)
	}
	if p.FromAddr != "solo@x.io" {
		t.Errorf("FromAddr = %q", p.FromAddr)
	}
	if !strings.Contains(p.TextBody, "just text") {
		t.Errorf("TextBody = %q", p.TextBody)
	}
}
