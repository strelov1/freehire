package ical

import (
	"bytes"
	"strings"
	"testing"
)

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

// The sender of a text/calendar part is whoever mailed the hosted address, and
// cmd/mail-ingest parses before it resolves the recipient and before it acks — one
// message at a time, in the one Restart=always daemon on the host with no MemoryMax. So
// the work one body may cost is a property worth pinning, not a detail.
//
// Over the cap reads as absence, which is what every other unparseable invitation already
// reads as: internal/application/calmatch simply makes no automatic link.
func TestUIDRefusesABodyTooLargeToBeAnInvitation(t *testing.T) {
	head := crlf("BEGIN:VCALENDAR", "BEGIN:VEVENT", "UID:oversized@example.test")
	pad := bytes.Repeat([]byte("X-PADDING:xxxxxxxxxxxxxxxxxxxx\r\n"), maxBody/32+1)
	body := append(head, pad...)
	if len(body) <= maxBody {
		t.Fatalf("test body is %d bytes, want more than the %d-byte cap", len(body), maxBody)
	}

	if got := UID(body); got != "" {
		t.Errorf("UID = %q, want empty for a body over the cap", got)
	}
}

// The cap must not be reachable by an invitation anyone actually sends. A calendar part
// with a long description and a full attendee list is kilobytes, so a body just under the
// bound is still read normally.
func TestUIDReadsABodyJustUnderTheCap(t *testing.T) {
	head := crlf("BEGIN:VCALENDAR", "BEGIN:VEVENT", "UID:roomy@example.test")
	pad := bytes.Repeat([]byte("X-PADDING:xxxxxxxxxxxxxxxxxxxx\r\n"), (maxBody-len(head))/32-1)
	body := append(head, pad...)
	if len(body) > maxBody {
		t.Fatalf("test body is %d bytes, want at most the %d-byte cap", len(body), maxBody)
	}

	if want := "roomy@example.test"; UID(body) != want {
		t.Errorf("UID = %q, want %q", UID(body), want)
	}
}

// unfold used to join continuations with `lines[len(lines)-1] += r[1:]`, which reallocates
// and copies everything accumulated so far on every continuation line — quadratic, and
// measured at 70s on a 6.4 MB body. The cap above bounds the damage; this pins the shape,
// because a cap alone would still leave minutes of CPU inside it.
//
// Doubling the input must roughly double the work, so the assertion is on ALLOCATED BYTES
// rather than on wall time: it is the copying that grew, and a byte count is the same
// number on a busy CI runner as on an idle laptop. Quadratic growth here was ~4x per
// doubling; the ceiling of 3x leaves ample room for a linear implementation's slack.
func TestUnfoldCostGrowsWithTheBodyNotItsSquare(t *testing.T) {
	folded := func(continuations int) []byte {
		var b strings.Builder
		b.WriteString("BEGIN:VEVENT\r\nUID:long@example.test\r\n")
		for i := 0; i < continuations; i++ {
			b.WriteString(" xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\r\n")
		}
		b.WriteString("END:VEVENT\r\n")
		return []byte(b.String())
	}
	small := testing.Benchmark(func(b *testing.B) {
		body := folded(4000)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = unfold(body)
		}
	})
	large := testing.Benchmark(func(b *testing.B) {
		body := folded(8000)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = unfold(body)
		}
	})

	smallBytes := small.AllocedBytesPerOp()
	largeBytes := large.AllocedBytesPerOp()
	if smallBytes == 0 {
		t.Fatal("measured no allocation for the smaller body; the benchmark did not run")
	}
	if ratio := float64(largeBytes) / float64(smallBytes); ratio > 3 {
		t.Errorf("doubling the body multiplied allocated bytes by %.1f (%d → %d); want linear growth, at most 3x",
			ratio, smallBytes, largeBytes)
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
