package matchanalysis

import "github.com/strelov1/freehire/internal/platform/llm"

// reasoningForStage says how much a stage of the chain should deliberate.
//
// Stage 1 is asked not to. "Extract & Match" restates the vacancy's requirements and says
// which the CV meets — it is the ATS lens, close to transcription, and the same argument
// #2636 made for the structured-résumé extraction applies to it. Stages 2 and 3 are the
// recruiter's verdict and the audit of that verdict; those are judgement, and they keep
// whatever deliberation the model would do.
//
// It is Stage 1 that fails in production, and it fails on its own deadline rather than on
// anything the gateway said: every log line reads `dur=1m30.000s`, twice per analysis (the
// stage retries once), for a total of three minutes spent to return nothing. A 6000-character
// prompt through the same alias answers in ~35s, so the stage is not slow because the
// gateway is — measured 2026-09-08, on postings from 2649 to 7938 characters, with length
// making no difference to whether it failed.
//
// The effort is honoured per provider, not universally: on the gateway's zai models it takes
// the reasoning tokens from ~1500 to under 30, while the gemini ones ignore it and answer
// fast regardless. Sending it is therefore right on both — it helps where the cost is, and
// is inert where it is not. See llm.ReasoningEffort.
//
// An unrecognised stage keeps the model's own default, so adding a fourth stage sends what
// it would have sent rather than silently inheriting Stage 1's answer.
func reasoningForStage(stage int) llm.ReasoningEffort {
	if stage == 1 {
		return llm.ReasoningNone
	}

	return llm.ReasoningDefault
}

// stageGenOptions renders a stage's call options. An empty effort produces NO option rather
// than an option carrying an empty value: the two are the same on the wire today, and only
// the first stays that way if llm.WithReasoning ever learns to send something for the zero
// value.
func stageGenOptions(stage int) []llm.GenOption {
	effort := reasoningForStage(stage)
	if effort == llm.ReasoningDefault {
		return nil
	}

	return []llm.GenOption{llm.WithReasoning(effort)}
}
