package matchanalysis

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// The predicate a caller needs to say WHY an analysis came back empty.
//
// The chain scores the fit from banked work history and never falls back to the raw CV, so
// a candidate whose CV failed to parse has a stored file and nothing to reason over. That
// used to reach them as "analysis unavailable" on a screen that had just told them their CV
// was present — a fault that looked like ours and was theirs to fix.
func TestHasCandidateContext(t *testing.T) {
	t.Run("false with no banked experience, whatever else the résumé carries", func(t *testing.T) {
		// Everything but the one field the chain reads. A summary and a skill list are not
		// evidence of having done anything, which is why they do not count.
		full := resumeextract.Professional{
			Headline: "Senior Salesforce Developer",
			Summary:  "Ten years of Salesforce delivery.",
			Skills:   []string{"salesforce", "apex"},
		}
		if HasCandidateContext(full) {
			t.Error("HasCandidateContext = true with no experience; the chain would decline and the caller would say the wrong thing")
		}
	})

	t.Run("true with one employment", func(t *testing.T) {
		one := resumeextract.Professional{
			Experience: []resumeextract.Experience{{Company: "Acme", Title: "Backend Engineer"}},
		}
		if !HasCandidateContext(one) {
			t.Error("HasCandidateContext = false with a banked employment; the chain would have run")
		}
	})

	t.Run("agrees with what the chain actually does", func(t *testing.T) {
		// The predicate exists so a caller need not re-derive the rule. If it ever stopped
		// answering the same question as the renderer the chain gates on, a caller would
		// explain an outcome that did not happen.
		for _, p := range []resumeextract.Professional{
			{},
			{Summary: "words"},
			{Experience: []resumeextract.Experience{{Company: "Acme"}}},
		} {
			if got, want := HasCandidateContext(p), candidateContext(p) != ""; got != want {
				t.Errorf("HasCandidateContext(%+v) = %v, want %v", p, got, want)
			}
		}
	})
}
