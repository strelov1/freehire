package mentorship

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Invite renders a booking as an iCalendar object (RFC 5545), the thing a mail client
// turns into an "add to calendar" button.
//
// internal/application/ical does NOT do this — it PARSES incoming invitations, for the
// calendar sync and the mail reader. Writing one is the opposite direction, shares no
// code with reading one, and has exactly one caller, so it lives here. If a second
// feature ever needs to write invitations, this file is what moves.
//
// Four rules decide almost everything below, and each is a way a calendar client
// silently does the wrong thing rather than reporting an error:
//
//   - Lines end with CRLF. Bare newlines are rejected by some clients and mis-parsed by
//     others.
//   - The UID is stable per booking. A cancellation with a different UID does not cancel
//     anything — it adds a second event and leaves the first in the calendar.
//   - A cancellation raises SEQUENCE. A client may ignore a same-sequence update as a
//     replay of what it already holds.
//   - TEXT values escape backslash, semicolon, comma and newline. A note containing a
//     comma would otherwise split one property into two.
func Invite(b Booking, opts InviteOptions) string {
	method, status, sequence := "REQUEST", "CONFIRMED", 0
	if b.Status == BookingCancelled {
		method, status, sequence = "CANCEL", "CANCELLED", 1
	}

	summary := opts.Summary
	if summary == "" {
		summary = defaultInviteSummary
	}

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//freehire//mentorship//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:" + method,
		"BEGIN:VEVENT",
		"UID:" + inviteUID(b),
		"DTSTAMP:" + icalTime(opts.Stamp),
		"DTSTART:" + icalTime(b.StartsAt),
		"DTEND:" + icalTime(b.EndsAt),
		"SUMMARY:" + escapeText(summary),
		"STATUS:" + status,
		fmt.Sprintf("SEQUENCE:%d", sequence),
	}

	if b.MeetingURL != "" {
		// Both, deliberately: LOCATION is what a calendar shows on the event, URL is what
		// some clients turn into a join button, and no single property covers both.
		lines = append(lines,
			"LOCATION:"+escapeText(b.MeetingURL),
			"URL:"+escapeText(b.MeetingURL),
		)
	}
	if description := inviteDescription(b); description != "" {
		lines = append(lines, "DESCRIPTION:"+escapeText(description))
	}
	if opts.Organizer != "" {
		organizer := "ORGANIZER"
		if opts.OrganizerName != "" {
			organizer += ";CN=" + opts.OrganizerName
		}
		lines = append(lines, organizer+":mailto:"+opts.Organizer)
	}
	if b.SeekerEmail != "" {
		lines = append(lines, "ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED:mailto:"+b.SeekerEmail)
	}

	lines = append(lines, "END:VEVENT", "END:VCALENDAR")

	var out strings.Builder
	for _, line := range lines {
		out.WriteString(foldLine(line))
		out.WriteString("\r\n")
	}
	return out.String()
}

// defaultInviteSummary is what the event is called when the caller says nothing.
const defaultInviteSummary = "Mentorship session"

// InviteOptions is what the booking itself does not carry.
type InviteOptions struct {
	// Organizer is the address the invitation comes from, and OrganizerName the display
	// name beside it.
	Organizer     string
	OrganizerName string
	// Summary is the event's title; empty falls back to defaultInviteSummary.
	Summary string
	// Stamp is DTSTAMP — when this object was produced. Zero means now. It is injectable
	// so a test can assert on a whole rendered invitation.
	Stamp time.Time
}

// inviteUID is stable per booking and globally unique, which is what lets a later
// cancellation refer to the event this one created.
func inviteUID(b Booking) string {
	return b.ID.String() + "@mentorship.freehire.me"
}

// inviteDescription is what the event body says: the seeker's note, then the link.
func inviteDescription(b Booking) string {
	var parts []string
	if b.Note != "" {
		parts = append(parts, b.Note)
	}
	if b.MeetingURL != "" {
		parts = append(parts, "Join: "+b.MeetingURL)
	}
	return strings.Join(parts, "\n\n")
}

// icalTime renders an instant as UTC in iCalendar's basic format. UTC always, never a
// local time with a TZID: a floating or zone-qualified time means shipping the zone
// definition too, and the instant is the only thing both parties agree on anyway.
func icalTime(at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	return at.UTC().Format("20060102T150405Z")
}

// escapeText escapes the four characters RFC 5545 gives special meaning inside a TEXT
// value. The backslash goes first, or it would escape the escapes.
func escapeText(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
	).Replace(s)
}

// maxLineOctets is RFC 5545's content-line limit, counted in OCTETS and excluding the
// CRLF.
const maxLineOctets = 75

// foldLine breaks a long content line the way RFC 5545 requires: at most 75 octets, with
// each continuation beginning with one space.
//
// It counts octets and steps by RUNE, which is the whole subtlety. The limit is a byte
// limit, but splitting between the bytes of a multi-byte character corrupts it — a
// description in Cyrillic is half the characters per line and twice as likely to land on
// a boundary, so this is not a theoretical case.
func foldLine(line string) string {
	if len(line) <= maxLineOctets {
		return line
	}

	var out strings.Builder
	written, limit := 0, maxLineOctets
	for _, r := range line {
		size := utf8.RuneLen(r)
		if written+size > limit {
			out.WriteString("\r\n ")
			// A continuation line's leading space counts towards its own 75 octets.
			written, limit = 0, maxLineOctets-1
		}
		out.WriteRune(r)
		written += size
	}
	return out.String()
}
