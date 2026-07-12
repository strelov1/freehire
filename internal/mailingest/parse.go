// Package mailingest drains inbound mail received at hosted mailboxes: it parses
// raw MIME, resolves the recipient to the owning user, and stores the message in
// the unified mail store. Parse is pure and unit-tested; the SES transport and
// the DB store sit behind interfaces so the worker runs against fakes.
package mailingest

import (
	"bytes"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jhillyerd/enmime"
)

// Parsed is the display-ready view of a received email the worker stores. The
// full raw MIME is kept separately in S3, so this carries only what the inbox
// list and reading pane need.
type Parsed struct {
	MessageID  string
	FromAddr   string
	FromName   string
	Subject    string
	TextBody   string
	HTMLBody   string
	ReceivedAt time.Time
}

// Parse turns raw MIME into a Parsed message. A missing Message-ID yields an empty
// MessageID (the worker synthesizes a dedup key from the S3 object key); an
// unparseable/absent Date yields a zero ReceivedAt (the worker falls back to now).
func Parse(raw []byte) (Parsed, error) {
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		return Parsed{}, fmt.Errorf("parse mime: %w", err)
	}

	p := Parsed{
		MessageID: trimAngles(env.GetHeader("Message-ID")),
		Subject:   env.GetHeader("Subject"),
		TextBody:  env.Text,
		HTMLBody:  env.HTML,
	}

	if addr, err := mail.ParseAddress(env.GetHeader("From")); err == nil {
		p.FromAddr = addr.Address
		p.FromName = addr.Name
	} else {
		// Keep the raw header rather than dropping the sender entirely.
		p.FromAddr = env.GetHeader("From")
	}

	if t, err := mail.ParseDate(env.GetHeader("Date")); err == nil {
		p.ReceivedAt = t
	}

	return p, nil
}

// trimAngles strips the surrounding <...> from a message-id style header.
func trimAngles(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}
