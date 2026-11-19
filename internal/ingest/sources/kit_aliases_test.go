package sources

import "testing"

// The exported wrappers keep sources' public helpers for callers outside ingest; each must
// answer exactly what kit does.
func TestKernelWrappersDelegate(t *testing.T) {
	if ParseDate("2026-06-30") == nil || ParseRFC3339("2026-06-30T00:00:00Z") == nil || ParseEpochMillis(1) == nil {
		t.Error("a date wrapper lost its value")
	}
	if NotFuture(nil) != nil || !IsRemote("Remote, EU") {
		t.Error("NotFuture or IsRemote wrapper disagrees with kit")
	}
	if UnescapeEncodedHTML("&lt;p&gt;x&lt;/p&gt;") != "<p>x</p>" || LenientPercentUnescape("a%20b%") != "a b%" {
		t.Error("an unescape wrapper disagrees with kit")
	}
}
