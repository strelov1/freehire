package gmailsync

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestParseMessage(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	raw := fmt.Sprintf(`{
      "id": "m1", "threadId": "t1", "internalDate": "1700000000000",
      "payload": {
        "mimeType": "multipart/alternative",
        "headers": [
          {"name":"From","value":"Acme Hiring <no-reply@greenhouse-mail.io>"},
          {"name":"Subject","value":"Thank you for applying"}
        ],
        "parts": [
          {"mimeType":"text/plain","body":{"data":%q}},
          {"mimeType":"text/html","body":{"data":%q}}
        ]
      }
    }`, enc("Hello, Ilya"), enc("<p>Hi</p>"))

	m, err := parseMessage([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.ID != "m1" || m.ThreadID != "t1" {
		t.Errorf("ids = %q/%q", m.ID, m.ThreadID)
	}
	if m.FromName != "Acme Hiring" || m.FromAddr != "no-reply@greenhouse-mail.io" {
		t.Errorf("from = %q / %q", m.FromName, m.FromAddr)
	}
	if m.Subject != "Thank you for applying" {
		t.Errorf("subject = %q", m.Subject)
	}
	if m.BodyText != "Hello, Ilya" || m.BodyHTML != "<p>Hi</p>" {
		t.Errorf("bodies = %q / %q", m.BodyText, m.BodyHTML)
	}
	if m.ReceivedAt.Unix() != 1_700_000_000 {
		t.Errorf("received = %d, want 1700000000", m.ReceivedAt.Unix())
	}
}

func TestParseMessageSinglePart(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString([]byte("plain only"))
	raw := fmt.Sprintf(`{
      "id":"m2","threadId":"t2","internalDate":"1700000000000",
      "payload":{"mimeType":"text/plain","headers":[{"name":"Subject","value":"S"}],"body":{"data":%q}}
    }`, enc)
	m, err := parseMessage([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.BodyText != "plain only" {
		t.Errorf("body = %q", m.BodyText)
	}
}

// The Gmail API is the second way an invitation reaches us, and it must yield the same
// meeting identifier the SES path does — the deterministic link is UID equality, and a
// candidate whose mail arrives by Gmail would otherwise never get one.
func TestParseMessageCapturesTheCalendarUID(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:derq-interview\r\n -folded@ashbyhq.com\r\n" +
		"DTSTART:20260813T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	raw := fmt.Sprintf(`{
      "id": "m2", "threadId": "t2", "internalDate": "1700000000000",
      "payload": {
        "mimeType": "multipart/mixed",
        "headers": [{"name":"Subject","value":"Interview with Derq"}],
        "parts": [
          {"mimeType":"text/plain","body":{"data":%q}},
          {"mimeType":"text/calendar","body":{"data":%q}}
        ]
      }
    }`, enc("We would like to invite you."), enc(ics))

	m, err := parseMessage([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := "derq-interview-folded@ashbyhq.com"; m.CalendarUID != want {
		t.Errorf("CalendarUID = %q, want the unfolded %q", m.CalendarUID, want)
	}
	if m.BodyText == "" {
		t.Error("the calendar part swallowed the text body")
	}
}

func TestParseMessageLeavesTheCalendarUIDEmptyWithoutAMeeting(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	raw := fmt.Sprintf(`{
      "id": "m3", "payload": {"mimeType":"text/plain","body":{"data":%q}}
    }`, enc("No meeting here."))

	m, err := parseMessage([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.CalendarUID != "" {
		t.Errorf("CalendarUID = %q, want empty", m.CalendarUID)
	}
}
