package mentorship

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func sampleBooking(t *testing.T) Booking {
	t.Helper()
	id, err := uuid.Parse("6f1e3d7a-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatalf("uuid.Parse: %v", err)
	}
	return Booking{
		ID:             id,
		MentorSlug:     "jane-doe",
		MentorTimezone: "Europe/Berlin",
		SeekerTimezone: "Asia/Tokyo",
		StartsAt:       time.Date(2026, time.September, 8, 16, 0, 0, 0, time.UTC),
		EndsAt:         time.Date(2026, time.September, 8, 17, 0, 0, 0, time.UTC),
		Status:         BookingConfirmed,
		MeetingURL:     "https://meet.example.test/jane",
	}
}

// Line endings are the one thing a calendar client will not forgive: RFC 5545 requires
// CRLF, and a file with bare newlines is rejected outright by some clients and silently
// mis-parsed by others.
func TestTheInviteUsesCRLFThroughout(t *testing.T) {
	ics := Invite(sampleBooking(t), InviteOptions{Organizer: "mentors@example.test"})

	if strings.Contains(strings.ReplaceAll(ics, "\r\n", ""), "\n") {
		t.Error("the invitation contains a bare newline")
	}
	if !strings.HasSuffix(ics, "END:VCALENDAR\r\n") {
		t.Errorf("the invitation does not end with a CRLF-terminated END:VCALENDAR: %q",
			ics[max(0, len(ics)-40):])
	}
}

func TestTheInviteCarriesTheSessionAndTheLink(t *testing.T) {
	booking := sampleBooking(t)
	ics := Invite(booking, InviteOptions{Organizer: "mentors@example.test", Summary: "Mentorship session"})

	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"METHOD:REQUEST",
		"BEGIN:VEVENT",
		"DTSTART:20260908T160000Z",
		"DTEND:20260908T170000Z",
		"SUMMARY:Mentorship session",
		"URL:https://meet.example.test/jane",
		"STATUS:CONFIRMED",
		"END:VEVENT",
		"END:VCALENDAR",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("the invitation is missing %q", want)
		}
	}
}

// A booking with no meeting link — the deliberate outcome of a failed calendar-event
// creation, see booking.go's attachMeetEvent — renders an invitation with no LOCATION or
// URL line at all, rather than one pointing nowhere.
func TestAnEmptyMeetingLinkOmitsLocationAndURL(t *testing.T) {
	booking := sampleBooking(t)
	booking.MeetingURL = ""
	ics := Invite(booking, InviteOptions{Organizer: "mentors@example.test", Summary: "Mentorship session"})

	if strings.Contains(ics, "LOCATION:") {
		t.Error("an empty meeting link still produced a LOCATION line")
	}
	if strings.Contains(ics, "URL:") {
		t.Error("an empty meeting link still produced a URL line")
	}
}

// The UID must be stable and derived from the booking: a cancellation carrying a
// different one does not cancel anything — it adds a second event to the calendar and
// leaves the first in place.
func TestTheUIDIsStableAndDerivedFromTheBooking(t *testing.T) {
	booking := sampleBooking(t)
	opts := InviteOptions{Organizer: "mentors@example.test"}

	first := Invite(booking, opts)
	second := Invite(booking, opts)
	if uidOf(t, first) != uidOf(t, second) {
		t.Error("two invitations for one booking carry different UIDs")
	}
	if !strings.Contains(uidOf(t, first), booking.ID.String()) {
		t.Errorf("UID %q does not name the booking", uidOf(t, first))
	}

	cancelled := booking
	cancelled.Status = BookingCancelled
	if uidOf(t, Invite(cancelled, opts)) != uidOf(t, first) {
		t.Error("the cancellation carries a different UID — it would add an event rather than remove one")
	}
}

// A cancellation is METHOD:CANCEL with a higher SEQUENCE. Without the sequence bump a
// client is entitled to ignore it as a replay of something it already has.
func TestACancellationIsAMethodCancelWithAHigherSequence(t *testing.T) {
	booking := sampleBooking(t)
	booking.Status = BookingCancelled
	ics := Invite(booking, InviteOptions{Organizer: "mentors@example.test"})

	for _, want := range []string{"METHOD:CANCEL", "STATUS:CANCELLED", "SEQUENCE:1"} {
		if !strings.Contains(ics, want) {
			t.Errorf("the cancellation is missing %q", want)
		}
	}

	confirmed := sampleBooking(t)
	if !strings.Contains(Invite(confirmed, InviteOptions{Organizer: "x@example.test"}), "SEQUENCE:0") {
		t.Error("a confirmation does not carry SEQUENCE:0")
	}
}

// RFC 5545 gives four characters special meaning inside a TEXT value. A mentor whose
// note contains a comma would otherwise split one property into two.
func TestTextValuesAreEscaped(t *testing.T) {
	booking := sampleBooking(t)
	booking.Note = "Bring: a CV, a laptop; and questions\nSee you\\then"
	ics := Invite(booking, InviteOptions{
		Organizer: "mentors@example.test",
		Summary:   "Session with Jane, Senior Engineer",
	})

	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	for _, want := range []string{
		`SUMMARY:Session with Jane\, Senior Engineer`,
		`a CV\, a laptop\; and questions\nSee you\\then`,
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("the invitation does not escape as expected; missing %q\ngot:\n%s", want, unfolded)
		}
	}
}

// Lines longer than 75 octets must be folded, and a folded line continues with a single
// leading space. Unfolding must give the original back.
func TestLongLinesAreFoldedAndUnfoldBackToThemselves(t *testing.T) {
	booking := sampleBooking(t)
	booking.Note = strings.Repeat("a very long line of description text ", 8)
	ics := Invite(booking, InviteOptions{Organizer: "mentors@example.test"})

	for _, line := range strings.Split(ics, "\r\n") {
		if len(line) > 75 {
			t.Errorf("a line is %d octets, over the 75-octet limit: %q", len(line), line[:40])
		}
	}

	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	if !strings.Contains(unfolded, `DESCRIPTION:`+strings.Repeat("a very long line of description text ", 8)) {
		t.Error("unfolding the description does not give the original text back")
	}
}

// Folding must not split a multi-byte character across two lines, which would corrupt it.
func TestFoldingDoesNotSplitAMultiByteCharacter(t *testing.T) {
	booking := sampleBooking(t)
	booking.Note = strings.Repeat("привет мир ", 12)
	ics := Invite(booking, InviteOptions{Organizer: "mentors@example.test"})

	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	if !strings.Contains(unfolded, strings.Repeat("привет мир ", 12)) {
		t.Error("a multi-byte description did not survive folding")
	}
	for _, line := range strings.Split(ics, "\r\n") {
		if !isValidUTF8(line) {
			t.Errorf("folding split a character: %q", line)
		}
	}
}

func TestTheInviteNamesBothParties(t *testing.T) {
	booking := sampleBooking(t)
	booking.SeekerEmail = "seeker@example.test"
	ics := Invite(booking, InviteOptions{
		Organizer:     "mentors@example.test",
		OrganizerName: "freehire mentorship",
	})

	if !strings.Contains(strings.ReplaceAll(ics, "\r\n ", ""), "ORGANIZER;CN=freehire mentorship:mailto:mentors@example.test") {
		t.Error("the invitation does not name its organizer")
	}
	if !strings.Contains(strings.ReplaceAll(ics, "\r\n ", ""), "ATTENDEE") {
		t.Error("the invitation does not name the attendee")
	}
}

func uidOf(t *testing.T, ics string) string {
	t.Helper()
	for _, line := range strings.Split(strings.ReplaceAll(ics, "\r\n ", ""), "\r\n") {
		if after, ok := strings.CutPrefix(line, "UID:"); ok {
			return after
		}
	}
	t.Fatal("the invitation carries no UID")
	return ""
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
