package telegram

import (
	"encoding/json"
	"testing"
)

// The extraction prompt says remote is a boolean, but the model can emit "true"/"yes" or
// 1. A string/number there would abort the whole post's extraction (every job in it).
func TestExtractedJob_RemoteFromStringOrNumber(t *testing.T) {
	raw := `{"jobs": [
		{"title": "Go dev", "description": "d1", "remote": "true"},
		{"title": "JS dev", "description": "d2", "remote": 1},
		{"title": "QA",     "description": "d3", "remote": false}
	]}`
	var ex Extraction
	if err := json.Unmarshal([]byte(raw), &ex); err != nil {
		t.Fatalf("unmarshal extraction with string/number remote failed: %v", err)
	}
	if len(ex.Jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(ex.Jobs))
	}
	if !ex.Jobs[0].Remote || !ex.Jobs[1].Remote {
		t.Errorf("remote flags = %v/%v, want true/true", ex.Jobs[0].Remote, ex.Jobs[1].Remote)
	}
	if ex.Jobs[2].Remote {
		t.Errorf("jobs[2].Remote = true, want false")
	}
	if ex.Jobs[0].Title != "Go dev" {
		t.Errorf("jobs[0].Title = %q, want %q", ex.Jobs[0].Title, "Go dev")
	}
}
