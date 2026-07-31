package maillink

import (
	"github.com/strelov1/freehire/internal/mailclassify"
	"github.com/strelov1/freehire/internal/mailmatch"
)

// thresholds gates the two confidence-driven decisions: auto-link vs suggestion,
// and automatic forward stage advancement.
type thresholds struct {
	autoLink float64
	stage    float64
}

// autoLinks reports whether a deterministic match is strong enough to link on its
// own — a thread continuation or a unique company-name hit, at or above the
// auto-link threshold. Only a deterministic tier qualifies: the LLM reads
// attacker-controlled text, so its confident pick becomes a suggestion instead.
//
// It is ONE predicate because two callers ask the same question for different
// reasons — the runner decides whether to spend LLM disambiguation on the email,
// resolveLink decides what to persist. Written twice, a drift between them would
// either pay for candidates nobody reads or auto-link an email we had just asked
// the model to disambiguate.
func (t thresholds) autoLinks(m mailmatch.Match) bool {
	deterministic := m.Tier == mailmatch.TierThread || m.Tier == mailmatch.TierName
	return deterministic && m.Confidence >= t.autoLink
}

// resolveLink turns the deterministic match and the LLM classification into the
// persisted link. A confident deterministic match (thread or unique name)
// auto-links; otherwise a non-zero LLM pick becomes a suggestion the caller
// confirms; otherwise the email stays unlinked.
func resolveLink(m mailmatch.Match, cls mailclassify.Classification, cfg thresholds) (jobID, suggestedJobID int64, source string, confidence float64) {
	if cfg.autoLinks(m) {
		return m.JobID, 0, "auto", m.Confidence
	}
	if cls.MatchedJobID != 0 {
		return 0, cls.MatchedJobID, "", cls.Confidence
	}
	return 0, 0, "", 0
}

// stageAdvance returns the stage a linked application should move forward to, or
// "" when no automatic advancement should occur: only a linked email at or above
// the stage-confidence threshold whose signal maps strictly forward advances.
func stageAdvance(jobID int64, currentStage string, cls mailclassify.Classification, cfg thresholds) string {
	if jobID == 0 || cls.Confidence < cfg.stage {
		return ""
	}
	stage, ok := mailclassify.AdvanceStage(currentStage, cls.Signal)
	if !ok {
		return ""
	}
	return stage
}
