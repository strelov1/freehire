package matchanalysis

import "github.com/strelov1/freehire/internal/platform/llm"

// reasoningForStage says how much a stage of the chain should deliberate.
//
// Stage 1 (Extract & Match) is close to transcription, so it is asked not to deliberate; it
// otherwise hit its own deadline. Stages 2 and 3 are judgement and keep the model's default.
//
// The effort is honoured per provider (zai models cut reasoning tokens; gemini ignores it), so
// sending it is harmless where it does nothing. See llm.ReasoningEffort.
//
// An unrecognised stage keeps the model's own default.
func reasoningForStage(stage int) llm.ReasoningEffort {
	if stage == 1 {
		return llm.ReasoningNone
	}

	return llm.ReasoningDefault
}

// stageGenOptions renders a stage's call options. An empty effort produces NO option rather
// than one carrying an empty value, which would change if llm.WithReasoning learns to send
// something for the zero value.
func stageGenOptions(stage int) []llm.GenOption {
	effort := reasoningForStage(stage)
	if effort == llm.ReasoningDefault {
		return nil
	}

	return []llm.GenOption{llm.WithReasoning(effort)}
}
