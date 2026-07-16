package sources

import "testing"

// TestNextFlightTextRows pins the shared RSC text-row parser: a flight carries rows of the
// form "<id>:T<hexlen>,<bytes>" and a posting's "$<id>" reference resolves to that row's
// exact byte content (the length is HEX and content may itself contain commas/newlines).
func TestNextFlightTextRows(t *testing.T) {
	// Two rows: id "15" (hexlen 0x b = 11 bytes "hello world") and id "1a"
	// (hexlen 0x5 = 5 bytes "a,b,c"). A non-hex byte precedes each id.
	flight := "0:something\n15:Tb,hello world\n1a:T5,a,b,c\n"
	rows := nextFlightTextRows(flight)
	if got, want := rows["15"], "hello world"; got != want {
		t.Errorf("rows[15] = %q, want %q", got, want)
	}
	if got, want := rows["1a"], "a,b,c"; got != want {
		t.Errorf("rows[1a] = %q, want %q (byte-length slice keeps embedded commas)", got, want)
	}
}
