package emailnotify

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"time"
)

// Attachment is one file carried by an email.
type Attachment struct {
	// Filename is what the recipient's client shows and saves it as.
	Filename string
	// ContentType is the full MIME type INCLUDING its parameters — for a calendar
	// invitation that means `text/calendar; charset=utf-8; method=REQUEST`, because a
	// client decides whether to render an "add to calendar" button from the method
	// parameter and treats the part as a plain file without it.
	ContentType string
	Content     []byte
}

// buildRawMessage assembles the RFC 5322 message SES sends verbatim, for a message
// carrying files.
//
// The structure is multipart/mixed wrapping a multipart/alternative body plus one part
// per attachment. That nesting is not decoration: putting the text and HTML alternatives
// directly under mixed makes a client show BOTH — the plain text and then the rendered
// HTML, one after the other — because parts of a mixed message are all displayed, while
// alternatives are a choice between equivalents.
//
// Content is base64-encoded regardless of type. A calendar invitation is text and would
// survive quoted-printable, but base64 is what keeps a long unfolded line, a CRLF, or a
// non-ASCII name from being rewritten in transit — and a rewritten `.ics` is one no
// client will parse.
func buildRawMessage(m Message) ([]byte, error) {
	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)

	// The subject is encoded rather than written raw: a header is ASCII, and a non-ASCII
	// subject line reaching a server unencoded is either rejected or mangled.
	headers := []string{
		"From: " + m.From,
		"To: " + m.To,
		"Subject: " + mime.QEncoding.Encode("utf-8", m.Subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=" + mixed.Boundary(),
	}
	if m.ReplyTo != "" {
		headers = append(headers, "Reply-To: "+m.ReplyTo)
	}
	// The same pair the simple path sends, spelled once in unsubscribeHeaders and
	// rendered into wire form here. A raw message is assembled by hand, so this is
	// the one place the two paths could have drifted apart.
	for _, h := range m.unsubscribeHeaders() {
		headers = append(headers, *h.Name+": "+*h.Value)
	}
	for _, h := range headers {
		buf.WriteString(h + "\r\n")
	}
	buf.WriteString("\r\n")

	if err := writeAlternativeBody(mixed, m.HTML, m.Text); err != nil {
		return nil, err
	}
	for _, a := range m.Attachments {
		if err := writeAttachment(mixed, a); err != nil {
			return nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, fmt.Errorf("emailnotify: closing the message: %w", err)
	}
	return buf.Bytes(), nil
}

// writeAlternativeBody writes the text-or-HTML choice as one part of the mixed message.
// Plain text goes FIRST: in multipart/alternative the last part a client understands
// wins, so the richest representation belongs last.
func writeAlternativeBody(mixed *multipart.Writer, htmlBody, textBody string) error {
	var body bytes.Buffer
	alternative := multipart.NewWriter(&body)

	for _, part := range []struct{ contentType, content string }{
		{"text/plain; charset=utf-8", textBody},
		{"text/html; charset=utf-8", htmlBody},
	} {
		w, err := alternative.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return fmt.Errorf("emailnotify: body part: %w", err)
		}
		if err := writeBase64(w, []byte(part.content)); err != nil {
			return err
		}
	}
	if err := alternative.Close(); err != nil {
		return fmt.Errorf("emailnotify: closing the body: %w", err)
	}

	w, err := mixed.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/alternative; boundary=" + alternative.Boundary()},
	})
	if err != nil {
		return fmt.Errorf("emailnotify: body: %w", err)
	}
	_, err = w.Write(body.Bytes())
	return err
}

func writeAttachment(mixed *multipart.Writer, a Attachment) error {
	w, err := mixed.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {a.ContentType},
		"Content-Transfer-Encoding": {"base64"},
		"Content-Disposition":       {`attachment; filename="` + a.Filename + `"`},
	})
	if err != nil {
		return fmt.Errorf("emailnotify: attachment %s: %w", a.Filename, err)
	}
	return writeBase64(w, a.Content)
}

// maxBase64Line is RFC 2045's limit on an encoded line. Longer lines are legal to
// produce and routinely mangled by intermediate servers, which for an attachment means a
// file that no longer decodes.
const maxBase64Line = 76

func writeBase64(w interface{ Write([]byte) (int, error) }, content []byte) error {
	encoded := base64.StdEncoding.EncodeToString(content)
	for len(encoded) > maxBase64Line {
		if _, err := w.Write([]byte(encoded[:maxBase64Line] + "\r\n")); err != nil {
			return err
		}
		encoded = encoded[maxBase64Line:]
	}
	_, err := w.Write([]byte(encoded + "\r\n"))
	return err
}
