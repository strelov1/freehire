package handler

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// postingRequirements is the seam that carries a posting's stated requirements into the
// tailoring context, and it deliberately does NOT decide which of the two producers wins —
// jobview does. These pin that it reads the merged answer rather than one side of it, which
// is the thing a second, drifting copy of the rule would break.

func jobWithEnrichment(t *testing.T, payload string, derived string) db.Job {
	t.Helper()
	j := db.Job{ID: 7}
	if payload != "" {
		j.Enrichment = json.RawMessage(payload)
	}
	if derived != "" {
		j.RequirementsDerived = json.RawMessage(derived)
	}
	return j
}

func TestPostingRequirementsPrefersWhatTheModelStated(t *testing.T) {
	job := jobWithEnrichment(t,
		`{"requirements":[{"text":"Model said Go","priority":"required"}]}`,
		`[{"text":"Parser said Kafka","priority":"preferred"}]`)

	got := postingRequirements(job)

	if len(got) != 1 || got[0].Text != "Model said Go" {
		t.Errorf("postingRequirements = %+v, want the model's list to win", got)
	}
}

// The case the derived column exists for: the model has not reached this posting, so the
// parser's read of its own markup is the only list there is.
func TestPostingRequirementsFallsBackToTheDerivedColumn(t *testing.T) {
	job := jobWithEnrichment(t, `{}`, `[{"text":"Parser said Kafka","priority":"preferred"}]`)

	got := postingRequirements(job)

	if len(got) != 1 || got[0].Text != "Parser said Kafka" {
		t.Errorf("postingRequirements = %+v, want the derived list when the model stated none", got)
	}
}

// A corrupt enrichment blob must cost the caller its requirements, not its whole context.
func TestPostingRequirementsDegradesOnAnUnreadableRow(t *testing.T) {
	job := jobWithEnrichment(t, `{"requirements": not json`, "")

	if got := postingRequirements(job); got != nil {
		t.Errorf("postingRequirements = %+v, want nil for a row the projection cannot read", got)
	}
}
