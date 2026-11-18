// Package ical reads the few facts freehire needs out of an iCalendar body — today,
// the event identifier an ATS invitation carries in its text/calendar part.
//
// It is a reader, not an implementation of RFC 5545: nothing here builds calendars, and
// the only property it understands is UID. That identifier makes the calendar meeting and the
// mailbox invitation provably the same meeting, the one link internal/application/calmatch may
// make without asking. No database access and no I/O.
package ical

import (
	"bytes"
	"strings"
)

// maxBody caps the body UID will parse: a bound on work done for an unauthenticated sender, not
// a format rule. cmd/mail-ingest parses a message before resolving the recipient, one at a time
// in one daemon, so a slow part delays every hosted mailbox. A megabyte is far above any real
// invitation. Over the cap reads as absence.
const maxBody = 1 << 20

// UID returns the first UID property in the body, or "" when it carries none.
//
// The first PROPERTY, not the first VEVENT's: a body whose VTODO precedes its VEVENT would yield
// the wrong one. ATS invitations are a single VEVENT, so this is documented, not implemented.
//
// Absence is reported as absence: the only automatic link built on UID equality must never
// attach a meeting to an application no invitation named.
func UID(body []byte) string {
	if len(body) > maxBody {
		return ""
	}
	for _, line := range unfold(body) {
		name, value, ok := splitProperty(line)
		if !ok || !strings.EqualFold(name, "UID") {
			continue
		}
		if value != "" {
			return value
		}
	}
	return ""
}

// unfold joins RFC 5545 continuation lines back into whole ones.
//
// The format wraps lines over 75 octets with a leading space or tab. Real UIDs are long enough
// that a line-by-line reader would return a truncated, well-formed, never-matching UID.
//
// The accumulator is a strings.Builder, not `+=` on the last line, which is quadratic in the
// number of continuations; maxBody alone would still leave minutes of CPU inside the cap.
func unfold(body []byte) []string {
	// Tolerate bare LF as well as the CRLF the format specifies: a message that has been
	// through a gateway is not always still canonical.
	raw := strings.Split(strings.ReplaceAll(string(bytes.TrimSpace(body)), "\r\n", "\n"), "\n")

	var (
		lines   []string
		current strings.Builder
		open    bool
	)
	for _, r := range raw {
		// A continuation before any content line is not one — nothing precedes it to
		// fold into, so it stands as its own line, leading whitespace and all.
		if open && (strings.HasPrefix(r, " ") || strings.HasPrefix(r, "\t")) {
			current.WriteString(r[1:])
			continue
		}
		if open {
			lines = append(lines, current.String())
			current.Reset()
		}
		current.WriteString(r)
		open = true
	}
	if open {
		lines = append(lines, current.String())
	}
	return lines
}

// splitProperty separates a content line into its property name and value.
//
// The name may carry parameters after a semicolon (`UID;X-VENDOR=ashby:value`), which are
// discarded. A line with no colon is not a property, which keeps "UID:" inside a SUMMARY from
// being read as one.
func splitProperty(line string) (name, value string, ok bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", "", false
	}
	name = line[:colon]
	if semi := strings.IndexByte(name, ';'); semi >= 0 {
		name = name[:semi]
	}
	return strings.TrimSpace(name), strings.TrimSpace(line[colon+1:]), true
}
