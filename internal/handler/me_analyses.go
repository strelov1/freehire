package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
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
func buildAnalysisItems(rows []db.ListUserJobAnalysesRow, cvUploadedAt *time.Time, model, language string) []myAnalysisItem {
	items := make([]myAnalysisItem, 0, len(rows))
	for _, r := range rows {
		analysis := decodeAnalysis(r.Analysis)
		if analysis == nil {
			continue
		}
		items = append(items, myAnalysisItem{
			Slug:         r.PublicSlug,
			Title:        r.Title,
			Company:      r.Company,
			Closed:       r.ClosedAt.Valid,
			OverallScore: analysis.OverallScore,
			Verdict:      analysis.Verdict,
			AnalysedAt:   r.CreatedAt.Time,
			Stale:        !stampsMatch(r.Model, r.CvUploadedAt, r.JobContentHash, r.Language, cvUploadedAt, r.ContentHash, model, language),
		})
	}
	return items
}

// ListMyAnalyses lists the jobs the caller has run the AI fit analysis on (newest first),
// with the caller's points balance in meta. Never calls the LLM. Cookie or API key.
func (h *matchHandlers) ListMyAnalyses(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.matchAnalysisCache.ListUserJobAnalyses(c.Context(), userID)
	if err != nil {
		return err
	}
	cvUploadedAt, _ := h.cvUploadedAt(c, userID)
	items := buildAnalysisItems(rows, cvUploadedAt, h.matchAnalysis.ModelID(), h.callerLanguage(c.Context(), userID))
	return c.JSON(fiber.Map{"data": items, "meta": fiber.Map{"credits": h.creditsBalance(c.Context(), userID)}})
}
