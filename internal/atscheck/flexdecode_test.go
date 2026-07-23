package atscheck

import (
	"encoding/json"
	"testing"
)

// The review is prompted for an integer content-quality score, but the model can return
// it as "85" or "85/100". A string there would abort the whole review (score + all
// suggestions), silently dropping the qualitative CV review.
func TestReview_StringContentQualityDecodes(t *testing.T) {
	raw := `{"content_quality": "85", "suggestions": ["tighten the summary"]}`
	var r Review
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal review with string score failed: %v", err)
	}
	if r.ContentQuality != 85 {
		t.Errorf("ContentQuality = %d, want 85", r.ContentQuality)
	}
	if len(r.Suggestions) != 1 || r.Suggestions[0] != "tighten the summary" {
		t.Errorf("Suggestions = %v, want [tighten the summary]", r.Suggestions)
	}
}
