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

// TestEverySenioritySurvivesTheRoundTrip walks our seniority vocabulary out through the
// projection and back in through the filter, and asserts it arrives as itself.
//
// The two directions are written in different files and were allowed to drift: we published
// `intern`, `junior`, `staff` and `principal` verbatim — the standard has no word for them —
// while the inbound filter knew only the standard's five. An agent that read one of those
// levels off a posting of ours and sent it back as a filter was told we did not understand
// it, and its answer WIDENED to the whole catalogue. Publishing a value we then refuse to
// filter on is a silent widening one step removed, and nothing here caught it.
//
// Walking vocab.SeniorityValues is the point. A test that enumerated either map would pass
// while the other half of the pair went missing.
//
// It cannot reach `entry`, which is a standard word we never publish and so never round
// trip. That one is pinned in searchinput_test.go, through QueryValues rather than against
// the map — asserting it here as well would only be typing it twice.
func TestEverySenioritySurvivesTheRoundTrip(t *testing.T) {
	for _, ours := range vocab.SeniorityValues {
		published := seniorityFor(ours)
		back, known := seniorityFromStandard[published]
		if !known {
			t.Errorf("seniority %q is published as experienceLevel %q, but that value is not "+
				"accepted as filters.experience_level: an agent reading it off our own posting "+
				"and sending it back gets the filter dropped and the whole catalogue instead",
				ours, published)
			continue
		}
		if back != ours {
			t.Errorf("seniority %q is published as %q, which filters back to %q: the round trip "+
				"changes which jobs the agent asked for", ours, published, back)
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
