package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/skilltag"
)

// textMatch derives job skills from the posting text (the same skilltag pass
// ingest uses) and scores coverage by the profile. The test drives the skill
// vocabulary off skilltag itself so it does not hardcode dictionary contents.
func TestTextMatch(t *testing.T) {
	const title = "Backend Engineer"
	const text = "We build backends with Go, PostgreSQL, Docker and Kubernetes."

	skills := skilltag.Parse(title + "\n" + text)
	if len(skills) == 0 {
		t.Skip("skilltag recognised no skills in the fixture; nothing to assert")
	}

	// A profile holding exactly the posting's skills → full coverage, none missing.
	full := textMatch(title, text, skills)
	if full.CoveragePercent != 100 || len(full.Missing) != 0 {
		t.Errorf("full profile: coverage=%d missing=%v, want 100 / none", full.CoveragePercent, full.Missing)
	}

	// An empty profile → zero coverage, everything missing.
	none := textMatch(title, text, nil)
	if none.CoveragePercent != 0 || len(none.Missing) != len(skills) {
		t.Errorf("empty profile: coverage=%d missing=%d, want 0 / %d", none.CoveragePercent, len(none.Missing), len(skills))
	}
	if none.Total != len(skills) {
		t.Errorf("total=%d, want %d", none.Total, len(skills))
	}
}
