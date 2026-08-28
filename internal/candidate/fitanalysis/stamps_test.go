package fitanalysis_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
)

func TestStampsFresh(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	ts := func(tm time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: tm, Valid: true} }
	txt := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

	stored := fitanalysis.StoredStamps("model-a", ts(now), txt("hash-1"), "en")
	live := func(cv *time.Time, hash pgtype.Text, model, language string) fitanalysis.Stamps {
		return fitanalysis.Stamps{CVUploadedAt: cv, JobContentHash: hash, Model: model, Language: language}
	}

	t.Run("all stamps match → fresh", func(t *testing.T) {
		if !live(&now, txt("hash-1"), "model-a", "en").Fresh(stored) {
			t.Error("want fresh when CV time, job hash, model, and language all match")
		}
	})
	t.Run("CV re-uploaded → stale", func(t *testing.T) {
		later := now.Add(time.Hour)
		if live(&later, txt("hash-1"), "model-a", "en").Fresh(stored) {
			t.Error("want stale when the CV upload time changed")
		}
	})
	t.Run("job re-ingested → stale", func(t *testing.T) {
		if live(&now, txt("hash-2"), "model-a", "en").Fresh(stored) {
			t.Error("want stale when the job content hash changed")
		}
	})
	t.Run("model upgraded → stale", func(t *testing.T) {
		if live(&now, txt("hash-1"), "model-b", "en").Fresh(stored) {
			t.Error("want stale when LLM_MODEL changed since the analysis")
		}
	})
	// freehire#1837: switching the profile language must re-run the free-text
	// commentary rather than leave it in the language it was originally analyzed in.
	t.Run("profile language changed → stale", func(t *testing.T) {
		if live(&now, txt("hash-1"), "model-a", "ru").Fresh(stored) {
			t.Error("want stale when the caller's profile language changed since the analysis")
		}
	})
	t.Run("missing live CV time → stale (cannot confirm)", func(t *testing.T) {
		if live(nil, txt("hash-1"), "model-a", "en").Fresh(stored) {
			t.Error("want stale when the live CV upload time is unknown")
		}
	})
	t.Run("both job hashes null → fresh (non-board job, never re-crawled)", func(t *testing.T) {
		nullHash := fitanalysis.StoredStamps("model-a", ts(now), pgtype.Text{}, "en")
		if !live(&now, pgtype.Text{}, "model-a", "en").Fresh(nullHash) {
			t.Error("want fresh when neither the stored nor the live job hash exists")
		}
	})
	t.Run("job gains a hash later → stale", func(t *testing.T) {
		nullHash := fitanalysis.StoredStamps("model-a", ts(now), pgtype.Text{}, "en")
		if live(&now, txt("hash-1"), "model-a", "en").Fresh(nullHash) {
			t.Error("want stale when the job acquired a content hash after the analysis")
		}
	})
}

// TestDecodeAnalysis pins the "unreadable blob reads as no analysis" rule every caller leans
// on to re-offer a compute rather than surface an error.
func TestDecodeAnalysis(t *testing.T) {
	if fitanalysis.DecodeAnalysis(nil) != nil {
		t.Error("an empty blob must decode to no analysis")
	}
	if fitanalysis.DecodeAnalysis([]byte("{not json")) != nil {
		t.Error("a corrupt blob must decode to no analysis, not an error")
	}
	if got := fitanalysis.DecodeAnalysis([]byte(`{"overall_score":73}`)); got == nil || got.OverallScore != 73 {
		t.Errorf("DecodeAnalysis = %+v, want a decoded analysis scoring 73", got)
	}
}
