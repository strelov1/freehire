package ical

import "testing"

// crlf builds a calendar body with the line endings the format actually uses. Writing
// "\n" in a literal would test a document no mail server sends.
func crlf(lines ...string) []byte {
	out := ""
	for _, l := range lines {
		out += l + "\r\n"
	}
	return []byte(out)
}

func TestUIDReadsTheEventIdentifier(t *testing.T) {
	got := UID(crlf(
		"BEGIN:VCALENDAR",
		"METHOD:REQUEST",
		"BEGIN:VEVENT",
		"DTSTART:20260813T090000Z",
		"UID:abc123@google.com",
		"SUMMARY:Interview",
		"END:VEVENT",
		"END:VCALENDAR",
	))
	if want := "abc123@google.com"; got != want {
		t.Errorf("UID = %q, want %q", got, want)
	}
}

// RFC 5545 folds a line longer than 75 octets onto continuations beginning with a space
// or a tab, and real UIDs from Google and the ATS platforms are long enough to be folded
// every time. A parser that reads line-by-line returns a truncated identifier, which is
// the worst possible failure: it looks like a UID, it is not empty, and it matches no
// calendar event ever.
func TestUIDUnfoldsBeforeReading(t *testing.T) {
	got := UID(crlf(
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"UID:040000008200E00074C5B7101A82E00800000000",
		" 90D9C4A1BFDC01000000000000000010000000",
		"\t7A2F1E4B9C8D",
		"END:VEVENT",
		"END:VCALENDAR",
	))
	want := "04000000820" + "0E00074C5B7101A82E0080000000090D9C4A1BFDC010000000000000000100000007A2F1E4B9C8D"
	if got != want {
		t.Errorf("UID = %q, want the unfolded %q", got, want)
	}
}

func TestUIDToleratesTheShapesSendersActuallyUse(t *testing.T) {
	cases := map[string][]byte{
		"lowercase property": crlf("BEGIN:VEVENT", "uid:lower@example.test", "END:VEVENT"),
		"leading whitespace before the value": crlf(
			"BEGIN:VEVENT", "UID: spaced@example.test", "END:VEVENT"),
		"parameters on the property": crlf(
			"BEGIN:VEVENT", "UID;X-VENDOR=ashby:param@example.test", "END:VEVENT"),
	}
	want := map[string]string{
		"lowercase property":                  "lower@example.test",
		"leading whitespace before the value": "spaced@example.test",
		"parameters on the property":          "param@example.test",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if got := UID(body); got != want[name] {
				t.Errorf("UID = %q, want %q", got, want[name])
			}
		})
	}
}

// A missing or unreadable identifier must read as absent rather than as something. The
// only automatic link this feature makes is UID equality, and a wrong non-empty answer
// would attach a meeting to an application nobody's invitation named.
func TestUIDReportsAbsenceRatherThanGuessing(t *testing.T) {
	cases := map[string][]byte{
		"no UID property": crlf("BEGIN:VEVENT", "SUMMARY:Interview", "END:VEVENT"),
		"empty value":     crlf("BEGIN:VEVENT", "UID:", "END:VEVENT"),
		"not a calendar":  []byte("Dear candidate,\r\n\r\nWe would like to invite you.\r\n"),
		"empty input":     nil,
		"UID-like text in a summary": crlf(
			"BEGIN:VEVENT", "SUMMARY:Please quote UID:not-a-real-id", "END:VEVENT"),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if got := UID(body); got != "" {
				t.Errorf("UID = %q, want empty", got)
			}
		})
	}
}

// A series carries one VEVENT per exception, all sharing the identifier. Reading the
// first is right; reading the last would drift when a single occurrence is rescheduled.
func TestUIDTakesTheFirstEventOfASeries(t *testing.T) {
	got := UID(crlf(
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"UID:series@example.test",
		"RECURRENCE-ID:20260813T090000Z",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:series@example.test",
		"END:VEVENT",
		"END:VCALENDAR",
	))
	if want := "series@example.test"; got != want {
		t.Errorf("UID = %q, want %q", got, want)
	}
}
