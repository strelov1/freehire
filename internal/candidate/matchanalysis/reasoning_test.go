package matchanalysis

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// Stage 1 is the one that times out in production, and it is the one stage not asking for a
// judgement: "Extract & Match" restates the vacancy's requirements and says which the CV
// meets. Stage 2 is the recruiter's verdict and Stage 3 audits it — those ARE judgement, and
// they keep whatever deliberation the model would do.
func TestOnlyTheExtractionStageAsksForNoDeliberation(t *testing.T) {
	if got := reasoningForStage(1); got != llm.ReasoningNone {
		t.Errorf("stage 1 asks for %q, want %q", got, llm.ReasoningNone)
	}
	for _, stage := range []int{2, 3} {
		if got := reasoningForStage(stage); got != llm.ReasoningDefault {
			t.Errorf("stage %d asks for %q, want the model's own default — it is a judgement", stage, got)
		}
	}
}

// A stage nobody has an opinion about must send nothing rather than something. The default
// is the empty effort precisely so an unknown stage keeps whatever it sent before.
func TestAnUnknownStageKeepsTheModelsOwnDeliberation(t *testing.T) {
	if got := reasoningForStage(9); got != llm.ReasoningDefault {
		t.Errorf("stage 9 asks for %q, want the model's own default", got)
	}
}
