package matchanalysis

import (
	"strings"
	"testing"
)

// The rule that weak evidence cannot sustain a high skills_coverage lived in the audit's
// prompt and nowhere else, and production shows exactly what that cost: measured over three
// vacancies on 2026-09-08, Stage 2 scored skills_coverage 95, 100 and 93, and the audit
// pulled each down to 62, 70 and 74. Same direction, same magnitude, every time — the
// signature of a rule one stage knows and the other has to rediscover.
//
// Both stages must carry it, or the second goes on paying to re-derive it.
func TestBothScoringStagesCarryTheEvidenceStrengthRule(t *testing.T) {
	rule := evidenceStrengthRule()
	if strings.TrimSpace(rule) == "" {
		t.Fatal("the evidence-strength rule is empty")
	}
	for _, tc := range []struct {
		name, prompt string
	}{
		{"stage 2 (recruiter verdict)", stage2SystemPrompt("en")},
		{"stage 3 (adversarial audit)", stage3SystemPrompt("en")},
	} {
		if !strings.Contains(tc.prompt, rule) {
			t.Errorf("%s does not carry the evidence-strength rule", tc.name)
		}
	}
}

// The rule is worth nothing if it does not name the two weak shapes by the words Stage 1
// actually stamps on a requirement. A model told "weak evidence counts for less" without
// being told what weak LOOKS like here is being asked to guess the vocabulary.
func TestTheEvidenceRuleNamesBothWeakShapesByTheirStampedNames(t *testing.T) {
	rule := evidenceStrengthRule()
	for _, want := range []string{"synonym-only", "keyword"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the rule does not name %q, which is how Stage 1 stamps it", want)
		}
	}
	if !strings.Contains(rule, "skills_coverage") {
		t.Error("the rule does not say which dimension it bounds")
	}
}

// Stage 2 scores and Stage 3 audits: the shared rule must not turn the recruiter into a
// second auditor. Only the audit is told to lower what somebody else scored.
func TestOnlyTheAuditIsToldToLowerAnAlreadyScoredVerdict(t *testing.T) {
	if strings.Contains(stage2SystemPrompt("en"), "lower any inflated dimension score") {
		t.Error("stage 2 was handed the audit's own instruction; it has nothing to audit yet")
	}
	if !strings.Contains(stage3SystemPrompt("en"), "lower any inflated dimension score") {
		t.Error("stage 3 lost its audit instruction")
	}
}
