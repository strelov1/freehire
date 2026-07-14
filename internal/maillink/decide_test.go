package maillink

import (
	"testing"

	"github.com/strelov1/freehire/internal/mailclassify"
	"github.com/strelov1/freehire/internal/mailmatch"
)

func TestResolveLink(t *testing.T) {
	cfg := thresholds{autoLink: 0.85, stage: 0.8}

	t.Run("deterministic name match auto-links", func(t *testing.T) {
		m := mailmatch.Match{JobID: 5, Confidence: 0.9, Tier: mailmatch.TierName}
		cls := mailclassify.Classification{Signal: mailclassify.SignalAcknowledgement}
		job, sug, src, _ := resolveLink(m, cls, cfg)
		if job != 5 || sug != 0 || src != "auto" {
			t.Fatalf("got (job=%d sug=%d src=%q), want (5,0,auto)", job, sug, src)
		}
	})

	t.Run("ambiguous match falls to LLM pick as a suggestion", func(t *testing.T) {
		m := mailmatch.Match{Tier: mailmatch.TierAmbiguous}
		cls := mailclassify.Classification{MatchedJobID: 7, Confidence: 0.7}
		job, sug, src, _ := resolveLink(m, cls, cfg)
		if job != 0 || sug != 7 || src != "" {
			t.Fatalf("got (job=%d sug=%d src=%q), want (0,7,\"\")", job, sug, src)
		}
	})

	t.Run("no match and no LLM pick stays unlinked", func(t *testing.T) {
		m := mailmatch.Match{Tier: mailmatch.TierNone}
		cls := mailclassify.Classification{Signal: mailclassify.SignalOther}
		job, sug, src, _ := resolveLink(m, cls, cfg)
		if job != 0 || sug != 0 || src != "" {
			t.Fatalf("got (job=%d sug=%d src=%q), want all empty", job, sug, src)
		}
	})
}

func TestStageAdvance(t *testing.T) {
	cfg := thresholds{autoLink: 0.85, stage: 0.8}

	t.Run("forward advance on linked high-confidence interview", func(t *testing.T) {
		got := stageAdvance(5, "applied", mailclassify.Classification{Signal: mailclassify.SignalInterviewInvitation, Confidence: 0.95}, cfg)
		if got != "interview" {
			t.Fatalf("got %q, want interview", got)
		}
	})

	t.Run("no advance when unlinked", func(t *testing.T) {
		got := stageAdvance(0, "applied", mailclassify.Classification{Signal: mailclassify.SignalInterviewInvitation, Confidence: 0.95}, cfg)
		if got != "" {
			t.Fatalf("got %q, want empty (unlinked)", got)
		}
	})

	t.Run("no advance below stage confidence threshold", func(t *testing.T) {
		got := stageAdvance(5, "applied", mailclassify.Classification{Signal: mailclassify.SignalInterviewInvitation, Confidence: 0.5}, cfg)
		if got != "" {
			t.Fatalf("got %q, want empty (low confidence)", got)
		}
	})

	t.Run("no advance on rejection", func(t *testing.T) {
		got := stageAdvance(5, "screening", mailclassify.Classification{Signal: mailclassify.SignalRejection, Confidence: 0.99}, cfg)
		if got != "" {
			t.Fatalf("got %q, want empty (rejection never auto)", got)
		}
	})
}
