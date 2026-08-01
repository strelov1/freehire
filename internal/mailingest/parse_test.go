package mailingest

import (
	"strings"
	"testing"
)

const sampleMIME = "From: Acme Careers <careers@acme.com>\r\n" +
	"To: ivan@inbox.freehire.me\r\n" +
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

// An ATS invitation carries the meeting as a text/calendar part, and its UID is the one
// thing that later proves the candidate's calendar entry is this same meeting. Losing it
// here costs the only link internal/calmatch may make without asking.
const invitationMIME = "From: Ashby <no-reply@ashbyhq.com>\r\n" +
	"To: ivan@inbox.freehire.me\r\n" +
	"Subject: Interview with Derq\r\n" +
	"Message-ID: <inv-1@ashbyhq.com>\r\n" +
	"Date: Mon, 12 Jul 2026 10:00:00 +0000\r\n" +
	"Content-Type: multipart/mixed; boundary=\"m\"\r\n" +
	"\r\n" +
	"--m\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"We would like to invite you to interview.\r\n" +
	"--m\r\n" +
	"Content-Type: text/calendar; method=REQUEST; charset=UTF-8\r\n" +
	"Content-Disposition: attachment; filename=\"invite.ics\"\r\n" +
	"\r\n" +
	"BEGIN:VCALENDAR\r\n" +
	"METHOD:REQUEST\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:0400000082-derq-interview\r\n" +
	" -continued@ashbyhq.com\r\n" +
	"DTSTART:20260813T090000Z\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n" +
	"--m--\r\n"

func TestParseCapturesTheInvitationsCalendarUID(t *testing.T) {
	p, err := Parse([]byte(invitationMIME))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "0400000082-derq-interview-continued@ashbyhq.com"
	if p.CalendarUID != want {
		t.Errorf("CalendarUID = %q, want the unfolded %q", p.CalendarUID, want)
	}
	if p.TextBody == "" {
		t.Error("the calendar part swallowed the text body")
	}
}

// Most mail carries no meeting, and that must read as absence rather than as an error:
// the ingest path may not start failing messages over a part they never had.
func TestParseLeavesTheCalendarUIDEmptyWhenThereIsNoMeeting(t *testing.T) {
	p, err := Parse([]byte(sampleMIME))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.CalendarUID != "" {
		t.Errorf("CalendarUID = %q, want empty", p.CalendarUID)
	}
}
