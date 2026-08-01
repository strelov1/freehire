// Package ical reads the few facts freehire needs out of an iCalendar body — today,
// the event identifier an ATS invitation carries in its text/calendar part.
//
// It is a reader, not an implementation of RFC 5545: nothing here builds calendars, and
// the only property it understands is UID. That identifier is what makes the meeting in
// the candidate's calendar and the invitation in their mailbox provably the same meeting,
// which is the one link internal/calmatch is allowed to make without asking.
//
// No database access and no I/O, so the parsing rules are testable on their own — the
// shape internal/appevent and internal/userjob already use for rules of this kind.
package ical

import (
	"bytes"
	"strings"
)

// UID returns the first VEVENT's identifier, or "" when the body carries none.
//
// Absence is reported as absence. The only automatic link built on this is UID equality,
// so a confident wrong answer would attach a meeting to an application that no
// invitation named — and the candidate would prepare for the wrong employer.
func UID(body []byte) string {
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
// The format wraps any line longer than 75 octets and marks the continuation with a
// leading space or tab. Real identifiers from Google and the ATS platforms are long
// enough that this happens every time, so a line-by-line reader returns a truncated UID
// — non-empty, well-formed, and matching no calendar event that will ever exist.
func unfold(body []byte) []string {
	// Tolerate bare LF as well as the CRLF the format specifies: a message that has been
	// through a gateway is not always still canonical.
	raw := strings.Split(strings.ReplaceAll(string(bytes.TrimSpace(body)), "\r\n", "\n"), "\n")

	var lines []string
	for _, r := range raw {
		if strings.HasPrefix(r, " ") || strings.HasPrefix(r, "\t") {
			if len(lines) > 0 {
				lines[len(lines)-1] += r[1:]
				continue
			}
		}
		lines = append(lines, r)
	}
	return lines
}

// splitProperty separates a content line into its property name and value.
//
// The name may carry parameters after a semicolon (`UID;X-VENDOR=ashby:value`), which
// are discarded — this package reads one property and has no use for their meaning. A
// line with no colon is not a property, which is what keeps the word "UID:" inside a
// SUMMARY from being read as one.
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
