package fitanalysis

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// Stamps is what a cached analysis is judged fresh against: the CV it was computed from,
// the job text it read, the model that wrote it, and the language it was written in.
//
// It is one type rather than four arguments because the four move together — every caller
// that has one has all of them, and a freshness check that silently dropped one would keep
// serving an analysis the candidate can see is out of date.
type Stamps struct {
	CVUploadedAt   *time.Time
	JobContentHash pgtype.Text
	Model          string
	Language       string
}

// Fresh reports whether a cached row still matches these live stamps. A model change
// (LLM_MODEL upgrade) invalidates the cache so the improved model re-analyzes — the analogue
// of the enrichment version and semantic-embedder staleness guards. A language change
// (freehire#1837) invalidates it the same way, so switching the profile language re-runs the
// free-text commentary rather than leaving it in the old language until the CV or job text
// happens to change too. Absent-on-both-sides hash stamps count as unchanged (a non-board job
// with no content_hash is never re-crawled, so its text is stable and a NULL stamp must not
// force an endless recompute); a stamp appearing on one side only is a change.
func (live Stamps) Fresh(stored Stamps) bool {
	return stored.Model == live.Model &&
		stored.Language == live.Language &&
		sameTime(stored.CVUploadedAt, live.CVUploadedAt) &&
		sameText(stored.JobContentHash, live.JobContentHash)
}

// StoredStamps reads the stamps off a stored row's raw columns, so every caller holding a
// different generated row type judges freshness by one rule.
func StoredStamps(model string, cvUploadedAt pgtype.Timestamptz, jobHash pgtype.Text, language string) Stamps {
	return Stamps{
		CVUploadedAt:   timePtr(cvUploadedAt),
		JobContentHash: jobHash,
		Model:          model,
		Language:       language,
	}
}

func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func sameTime(stored, live *time.Time) bool {
	if (stored != nil) != (live != nil) {
		return false
	}
	return stored == nil || stored.Equal(*live)
}

func sameText(stored, live pgtype.Text) bool {
	if stored.Valid != live.Valid {
		return false
	}
	return !stored.Valid || stored.String == live.String
}

// DecodeAnalysis unmarshals a cached analysis blob, returning nil on empty/corrupt data
// (treated as "no analysis" — the caller re-offers a compute).
func DecodeAnalysis(blob []byte) *matchanalysis.Analysis {
	if len(blob) == 0 {
		return nil
	}
	var a matchanalysis.Analysis
	if err := json.Unmarshal(blob, &a); err != nil {
		return nil
	}
	return &a
}
