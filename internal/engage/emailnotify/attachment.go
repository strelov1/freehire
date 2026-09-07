package emailnotify

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
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

// AttachmentSender is the transport for an email that carries files. It is separate from
// Sender rather than replacing it: almost every mail this product sends is body-only, and
// SES bills and validates a raw message differently from a simple one.
type AttachmentSender interface {
	SendWithAttachments(ctx context.Context, from, to, subject, htmlBody, textBody string, attachments []Attachment) error
}

// Compile-time guarantee that Client is an AttachmentSender.
var _ AttachmentSender = (*Client)(nil)

// SendWithAttachments delivers one email carrying files, via SES's raw-message path.
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
func (c *Client) SendWithAttachments(ctx context.Context, from, to, subject, htmlBody, textBody string, attachments []Attachment) error {
	raw, err := buildRawMessage(from, to, subject, htmlBody, textBody, attachments)
	if err != nil {
		return err
	}
	in := &sesv2.SendEmailInput{
		Content: &types.EmailContent{Raw: &types.RawMessage{Data: raw}},
	}
	if _, err := c.ses.SendEmail(ctx, in); err != nil {
		return fmt.Errorf("emailnotify: ses raw send to %s: %w", to, err)
	}
	return nil
}

// buildRawMessage assembles the RFC 5322 message SES sends verbatim.
func buildRawMessage(from, to, subject, htmlBody, textBody string, attachments []Attachment) ([]byte, error) {
	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)

	// The subject is encoded rather than written raw: a header is ASCII, and a non-ASCII
	// subject line reaching a server unencoded is either rejected or mangled.
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=" + mixed.Boundary(),
	}
	for _, h := range headers {
		buf.WriteString(h + "\r\n")
	}
	buf.WriteString("\r\n")

	if err := writeAlternativeBody(mixed, htmlBody, textBody); err != nil {
		return nil, err
	}
	for _, a := range attachments {
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
