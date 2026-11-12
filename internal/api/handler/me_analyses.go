package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
)

// myAnalysisItem is one row of the Tracking → AI fit tab: a compact projection of a
// cached fit analysis for listing (not the full matchanalysis.Analysis). Stale is true when the
// caller's CV, the job content, or the model changed since the analysis was computed.
type myAnalysisItem struct {
	Slug         string    `json:"slug"`
	Title        string    `json:"title"`
	Company      string    `json:"company"`
	Closed       bool      `json:"closed"`
	OverallScore int       `json:"overall_score"`
	Verdict      string    `json:"verdict"`
	AnalysedAt   time.Time `json:"analysed_at"`
	Stale        bool      `json:"stale"`
}

// buildAnalysisItems projects the caller's analysed-job rows into list items, flagging
// staleness against the caller's live CV upload time, current model, and profile
// language. Rows whose stored analysis blob is empty or corrupt are skipped (nothing to
// show). Order is preserved (the query returns newest-first).
//
// live carries the caller-wide stamps; the job content hash is the one thing that varies per
// row (each row joins its own job's current hash), so it is filled in per item rather than
// passed in. The rule those stamps are compared by is fitanalysis's, the same one the single-
// job read uses — this listing must not be able to disagree with the page it links to.
func buildAnalysisItems(rows []db.ListUserJobAnalysesRow, live fitanalysis.Stamps) []myAnalysisItem {
	items := make([]myAnalysisItem, 0, len(rows))
	for _, r := range rows {
		analysis := fitanalysis.DecodeAnalysis(r.Analysis)
		if analysis == nil {
			continue
		}
		rowLive := live
		rowLive.JobContentHash = r.ContentHash
		items = append(items, myAnalysisItem{
			Slug:         r.PublicSlug,
			Title:        r.Title,
			Company:      r.Company,
			Closed:       r.ClosedAt.Valid,
			OverallScore: analysis.OverallScore,
			Verdict:      analysis.Verdict,
			AnalysedAt:   r.CreatedAt.Time,
			Stale:        !rowLive.Fresh(fitanalysis.StoredStamps(r.Model, r.CvUploadedAt, r.JobContentHash, r.Language)),
		})
	}
	return items
}

// ListMyAnalyses lists the jobs the caller has run the AI fit analysis on (newest first),
// with the caller's points balance in meta. Never calls the LLM. Cookie or API key.
func (h *matchHandlers) ListMyAnalyses(ctx context.Context) (*apigen.FitAnalysedJobs, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.fit.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	cvUploadedAt, _ := h.cvUploadedAt(ctx, userID)
	// job is the zero value: liveStamps only reads its content hash, and this listing fills
	// that per row from the job each one joins.
	items := buildAnalysisItems(rows, h.liveStamps(ctx, userID, db.Job{}, cvUploadedAt))
	out := &apigen.FitAnalysedJobs{Data: make([]apigen.FitAnalysedJob, len(items))}
	for i, it := range items {
		out.Data[i] = apigen.FitAnalysedJob{Slug: it.Slug, Title: it.Title, Company: it.Company, Closed: it.Closed,
			OverallScore: int64(it.OverallScore), Verdict: it.Verdict, AnalysedAt: it.AnalysedAt, Stale: it.Stale}
	}
	// Null when the standing cannot be read, as the nil pointer always marshalled.
	if out.Meta.Allowance, err = json.Marshal(h.allowanceView(ctx, userID)); err != nil {
		return nil, err
	}
	return out, nil
}
