package mailclassify

import (
	"encoding/json"
	"testing"
)

// The classifier is prompted for a numeric confidence and a matched job id, but the model
// can quote them ("0.8") or answer "none" for the id. Either would abort the whole
// classification, silently leaving the email unclassified/unlinked.
func TestClassification_StringConfidenceAndIDDecode(t *testing.T) {
	raw := `{"signal": "applied", "confidence": "0.8", "matched_job_id": "none"}`
	var c Classification
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal classification with string confidence/id failed: %v", err)
	}
	if c.Confidence != 0.8 {
		t.Errorf("Confidence = %v, want 0.8", c.Confidence)
	}
	if c.MatchedJobID != 0 {
		t.Errorf("MatchedJobID = %d, want 0", c.MatchedJobID)
	}
	if c.Signal != "applied" {
		t.Errorf("Signal = %q, want applied", c.Signal)
	}
}
