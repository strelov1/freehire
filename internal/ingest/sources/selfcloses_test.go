package sources

import "testing"

// SelfCloses is what keeps the quiet-board backoff off a change-stream source: jobtech emits its
// feed's changes, not its listing, while an ordinary ATS adapter does not self-close.
func TestSelfCloses(t *testing.T) {
	tax := Taxonomy()
	if !SelfCloses(tax["jobtech"]) {
		t.Error("SelfCloses(jobtech) = false, want true")
	}
	if SelfCloses(tax["greenhouse"]) {
		t.Error("SelfCloses(greenhouse) = true, want false")
	}
}
