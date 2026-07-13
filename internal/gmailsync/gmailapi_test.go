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
