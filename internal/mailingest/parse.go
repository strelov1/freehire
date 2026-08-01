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

	"github.com/strelov1/freehire/internal/ical"
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
	// CalendarUID is the identifier of the meeting an invitation attaches as its
	// text/calendar part, and "" for the mail that carries none — which is most of it.
	// It is the only thing that later proves a calendar entry and this invitation are
	// the same meeting, which is the one link calmatch may make without asking.
	CalendarUID string
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

	p.CalendarUID = calendarUID(env)

	return p, nil
}

// calendarUID reads the meeting identifier out of a text/calendar part.
//
// The part arrives as an attachment from some senders and as a plain sibling part from
// others, so every list enmime splits the message into is searched. A body that does not
// parse yields "" and nothing else: an invitation whose meeting we cannot identify is
// still an invitation, and failing the message over it would lose the mail as well.
func calendarUID(env *enmime.Envelope) string {
	for _, parts := range [][]*enmime.Part{env.Attachments, env.Inlines, env.OtherParts} {
		for _, part := range parts {
			if !strings.EqualFold(part.ContentType, "text/calendar") {
				continue
			}
			if uid := ical.UID(part.Content); uid != "" {
				return uid
			}
		}
	}
	return ""
}

// trimAngles strips the surrounding <...> from a message-id style header.
func trimAngles(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}
