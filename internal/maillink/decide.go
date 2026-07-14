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

// resolveLink turns the deterministic match and the LLM classification into the
// persisted link. A confident deterministic match (thread or unique name)
// auto-links; otherwise a non-zero LLM pick becomes a suggestion the caller
// confirms; otherwise the email stays unlinked.
func resolveLink(m mailmatch.Match, cls mailclassify.Classification, cfg thresholds) (jobID, suggestedJobID int64, source string, confidence float64) {
	if (m.Tier == mailmatch.TierThread || m.Tier == mailmatch.TierName) && m.Confidence >= cfg.autoLink {
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
