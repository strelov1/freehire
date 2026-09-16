package ojcp

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/ghost"
)

// These tests walk the REAL vocabularies rather than a list written here. A test that
// enumerates the same values the map does proves only that someone typed them twice: add a
// fifth work mode and every posting carrying it would silently omit remote_policy, with
// every hand-written case still green.
//
// Each one asserts only that the vocabulary value has a DECIDED answer — translated, or
// deliberately not — never what that answer is. Deciding is the thing that gets forgotten.

func TestEveryWorkModeHasADecision(t *testing.T) {
	for _, value := range vocab.WorkModeValues {
		if _, decided := remotePolicy[value]; !decided {
			t.Errorf("work mode %q has no entry in remotePolicy: every posting carrying it "+
				"omits remote_policy silently. Translate it, or add it with an empty value "+
				"to record that OJCP has no word for it.", value)
		}
	}
}

func TestEverySalaryPeriodHasADecision(t *testing.T) {
	for _, value := range vocab.SalaryPeriodValues {
		if _, decided := salaryUnit[value]; !decided {
			t.Errorf("salary period %q has no entry in salaryUnit: a posting carrying it "+
				"drops its whole baseSalary block. Translate it, or add it with an empty "+
				"value to record the deliberate gap.", value)
		}
	}
}

func TestEveryGhostLevelHasADecision(t *testing.T) {
	for _, level := range []string{ghost.LevelNone, ghost.LevelPossible, ghost.LevelLikely} {
		if level == ghost.LevelNone {
			// Silence is the decision for `none`, and it is asserted in agentnotes_test.go.
			continue
		}
		if _, decided := realityVerdict[level]; !decided {
			t.Errorf("ghost level %q has no wording: a posting at that level publishes no "+
				"agent_notes at all", level)
		}
	}
}

func TestEveryGhostCriterionHasAReadableSentence(t *testing.T) {
	for _, code := range ghost.CriterionCodes {
		if _, named := criterionSentence[code]; !named {
			t.Errorf("ghost criterion %q has no readable sentence: an agent would relay the "+
				"bare code to a candidate as jargon", code)
		}
	}
}
