package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

// TestServedAnalysisCappedByUnmetHardConstraint drives the real serve-path chain a
// caller who misses a hard constraint goes through: build inputs from the job +
// résumé, evaluate blockers, and apply them to the LLM analysis (as GET/POST do).
// The over-optimistic score must be capped and the blocker surfaced.
func TestServedAnalysisCappedByUnmetHardConstraint(t *testing.T) {
	job := db.Job{Description: "Requires an active PMP certification."}
	cv := resumeextract.Structured{Certifications: []string{"AWS Certified Solutions Architect"}} // lists certs, not PMP
	jr, ev := buildHardConstraintInputs(job, cv, userprofile.LocationPreferences{})
	blockers := hardconstraint.Evaluate(jr, ev)

	analysis := &matchanalysis.Analysis{OverallScore: 88, Verdict: "Strong Fit"}
	applyBlockers(analysis, blockers)

	if analysis.OverallScore != 60 { // certification score-cap
		t.Errorf("served overall_score = %d, want 60 (capped by unmet PMP)", analysis.OverallScore)
	}
	var sawCert bool
	for _, b := range analysis.Blockers {
		if b.Category == hardconstraint.CategoryCertification && !b.Met {
			sawCert = true
		}
	}
	if !sawCert {
		t.Error("served analysis should surface the unmet certification blocker")
	}
}
