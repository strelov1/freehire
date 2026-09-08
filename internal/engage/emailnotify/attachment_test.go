package emailnotify

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// decodeBase64Part reads one MIME part and decodes its base64 body. multipart.Part
// handles quoted-printable on its own and base64 not at all, which is exactly the sort of
// thing that makes a substring assertion pass on a message no client could read.
func decodeBase64Part(t *testing.T, part io.Reader) string {
	t.Helper()
	encoded, err := io.ReadAll(part)
	if err != nil {
		t.Fatalf("reading a part: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(encoded), "\r\n", ""))
	if err != nil {
		t.Fatalf("decoding a part: %v", err)
	}
	return string(decoded)
}

// The assertion that matters: the message this builds must PARSE, with Go's own mail and
// multipart readers, all the way down to the attachment's bytes. Checking for substrings
// would pass on a message no client could read.
func TestTheRawMessageParsesBackToItsParts(t *testing.T) {
	raw, err := buildRawMessage(
		"mentors@example.test", "seeker@example.test", "Your session is confirmed",
		"<p>See you Tuesday</p>", "See you Tuesday",
		[]Attachment{{
			Filename:    "invite.ics",
			ContentType: "text/calendar; charset=utf-8; method=REQUEST",
			Content:     []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"),
		}},
	)
	if err != nil {
		t.Fatalf("buildRawMessage: %v", err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("the message does not parse as mail: %v", err)
	}
	if got := msg.Header.Get("To"); got != "seeker@example.test" {
		t.Errorf("To = %q", got)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type does not parse: %v", err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("top-level type = %q, want multipart/mixed", mediaType)
	}

	var sawAlternative bool
	var invite string
	reader := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading a part: %v", err)
		}

		partType, partParams, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("part Content-Type does not parse: %v", err)
		}

		switch partType {
		case "multipart/alternative":
			sawAlternative = true
			assertAlternativeHoldsBothBodies(t, part, partParams["boundary"])
		case "text/calendar":
			// The method parameter is what makes a client offer "add to calendar"
			// instead of showing a file to download.
			if partParams["method"] != "REQUEST" {
				t.Errorf("the calendar part carries method=%q, want REQUEST", partParams["method"])
			}
			if got := part.FileName(); got != "invite.ics" {
				t.Errorf("attachment filename = %q, want invite.ics", got)
			}
			// multipart.Part decodes quoted-printable transparently and base64 NOT at
			// all, so the decode is explicit here.
			invite = decodeBase64Part(t, part)
		default:
			t.Errorf("unexpected part %q", partType)
		}
	}

	if !sawAlternative {
		t.Error("the message carries no multipart/alternative body")
	}
	if invite != "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" {
		t.Errorf("the attachment did not survive the round trip: %q", invite)
	}
}

// Plain text must come first and HTML last: in multipart/alternative a client shows the
// LAST part it understands, so the order is what decides whether anybody sees the HTML.
func assertAlternativeHoldsBothBodies(t *testing.T, part io.Reader, boundary string) {
	t.Helper()

	var order []string
	var bodies []string
	reader := multipart.NewReader(part, boundary)
	for {
		sub, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading an alternative: %v", err)
		}
		mediaType, _, err := mime.ParseMediaType(sub.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("alternative Content-Type: %v", err)
		}
		order = append(order, mediaType)
		bodies = append(bodies, decodeBase64Part(t, sub))
	}

	if len(order) != 2 || order[0] != "text/plain" || order[1] != "text/html" {
		t.Fatalf("alternatives are %v, want text/plain then text/html", order)
	}
	if bodies[0] != "See you Tuesday" || bodies[1] != "<p>See you Tuesday</p>" {
		t.Errorf("bodies did not survive: %q and %q", bodies[0], bodies[1])
	}
}

// A non-ASCII subject must be header-encoded, or it is rejected or mangled in transit.
func TestANonASCIISubjectIsEncoded(t *testing.T) {
	raw, err := buildRawMessage("a@example.test", "b@example.test",
		"Ваша сессия подтверждена", "<p>hi</p>", "hi", nil)
	if err != nil {
		t.Fatalf("buildRawMessage: %v", err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("the message does not parse: %v", err)
	}
	encoded := msg.Header.Get("Subject")
	if !strings.HasPrefix(encoded, "=?utf-8?") {
		t.Errorf("Subject = %q, want an encoded-word", encoded)
	}

	decoded, err := new(mime.WordDecoder).DecodeHeader(encoded)
	if err != nil {
		t.Fatalf("decoding the subject: %v", err)
	}
	if decoded != "Ваша сессия подтверждена" {
		t.Errorf("decoded subject = %q", decoded)
	}
}

// Long base64 lines are legal to produce and routinely rewritten in transit, which for an
// attachment means a file that no longer decodes.
func TestBase64LinesStayWithinTheLimit(t *testing.T) {
	raw, err := buildRawMessage("a@example.test", "b@example.test", "s",
		strings.Repeat("<p>a long html body</p>", 200), "text",
		[]Attachment{{
			Filename:    "invite.ics",
			ContentType: "text/calendar; charset=utf-8",
			Content:     []byte(strings.Repeat("BEGIN:VEVENT\r\nEND:VEVENT\r\n", 100)),
		}},
	)
	if err != nil {
		t.Fatalf("buildRawMessage: %v", err)
	}

	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("a line is %d octets, over RFC 5322's hard limit", len(line))
		}
	}
}

// A message with no attachments still parses, so the same path serves both.
func TestAMessageWithNoAttachmentsIsStillValid(t *testing.T) {
	raw, err := buildRawMessage("a@example.test", "b@example.test", "s", "<p>h</p>", "t", nil)
	if err != nil {
		t.Fatalf("buildRawMessage: %v", err)
	}
	if _, err := mail.ReadMessage(strings.NewReader(string(raw))); err != nil {
		t.Errorf("the message does not parse: %v", err)
	}
}
