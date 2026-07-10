package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobfit"
	"github.com/strelov1/freehire/internal/jobmatch"
)

// jobFitStore reads/writes the per-(user, job) cached fit analysis. *db.Queries
// satisfies it; a fake backs the DB-less handler tests.
type jobFitStore interface {
	GetUserJobAnalysis(ctx context.Context, arg db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error)
	UpsertUserJobAnalysis(ctx context.Context, arg db.UpsertUserJobAnalysisParams) error
}

// jobFitResponse is the wire shape for the LLM fit analysis. HasCV is false when the
// caller has no stored CV — the SPA then prompts an upload instead of an empty report.
// Stale marks a cached analysis whose CV or job changed since (the SPA offers a
// recompute); Analysis is nil when none is cached or the LLM is unconfigured.
type jobFitResponse struct {
	HasCV    bool             `json:"has_cv"`
	Stale    bool             `json:"stale"`
	Analysis *jobfit.Analysis `json:"analysis"`
}

// GetJobFit serves the cached fit analysis for one of the caller's jobs, never calling
// the LLM. It returns the cached analysis (flagged stale when the CV or job changed
// since it was computed), or a null analysis when none is cached. Cookie or API key;
// an unknown slug is a 404. has_cv=false (no LLM ever) when no CV is stored.
func (a *API) GetJobFit(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := a.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	cvUploadedAt, hasCV := a.cvUploadedAt(c, userID)
	if !hasCV {
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: false}})
	}
	row, err := a.jobFitCache.GetUserJobAnalysis(c.Context(), db.GetUserJobAnalysisParams{UserID: userID, JobID: job.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true}})
	}
	if err != nil {
		return err
	}
	analysis := decodeAnalysis(row.Analysis)
	if analysis == nil {
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true}})
	}
	stale := !stampsFresh(row, cvUploadedAt, job.ContentHash, a.jobFit.ModelID())
	return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true, Stale: stale, Analysis: analysis}})
}

// PostJobFit runs the three-stage fit prompt-chain over the caller's stored CV and the
// job, caches the result per (user, job), and returns it fresh. Best-effort: an
// unconfigured or failing LLM returns has_cv with a null analysis (200) and caches
// nothing. Cookie or API key; unknown slug 404; has_cv=false when no CV is stored.
func (a *API) PostJobFit(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := a.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	cvText, hasCV, err := a.storedCVText(c, userID)
	if err != nil {
		return err
	}
	if !hasCV {
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: false}})
	}
	// Capture the CV upload time up front, so the cache is stamped with the CV that was
	// actually analyzed even if the user re-uploads mid-analysis (the three-stage chain
	// takes seconds); re-reading it afterwards would risk stamping a newer CV's time on
	// an older CV's analysis.
	cvUploadedAt, _ := a.cvUploadedAt(c, userID)

	analysis, err := a.jobFit.Analyze(c.Context(), jobfit.Input{
		JobTitle:       job.Title,
		JobDescription: job.Description,
		CompanyInfo:    a.companyInfo(c, job.CompanySlug),
		CVText:         cvText,
		Match:          jobmatch.Compute(job.Skills, a.profileSkills(c, userID)),
	})
	if err != nil {
		// Best-effort: log (never the CV/job text) and serve no analysis.
		log.Printf("jobfit: analyze failed for user %d job %d: %v", userID, job.ID, err)
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true}})
	}
	if analysis == nil {
		// LLM unconfigured — nothing to cache.
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true}})
	}
	a.cacheAnalysis(c, userID, job, cvUploadedAt, analysis)
	return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true, Stale: false, Analysis: analysis}})
}

// cacheAnalysis upserts the analysis stamped with the analyzed CV's upload time, the job
// content hash, and the model that produced it. Best-effort: a cache failure is logged,
// not surfaced.
func (a *API) cacheAnalysis(c *fiber.Ctx, userID int64, job db.Job, cvUploadedAt *time.Time, analysis *jobfit.Analysis) {
	blob, err := json.Marshal(analysis)
	if err != nil {
		return
	}
	if err := a.jobFitCache.UpsertUserJobAnalysis(c.Context(), db.UpsertUserJobAnalysisParams{
		UserID:         userID,
		JobID:          job.ID,
		Analysis:       blob,
		Model:          a.jobFit.ModelID(),
		CvUploadedAt:   tsFromPtr(cvUploadedAt),
		JobContentHash: job.ContentHash,
	}); err != nil {
		log.Printf("jobfit: cache analysis for user %d job %d: %v", userID, job.ID, err)
	}
}

// cvUploadedAt reports the caller's stored-CV upload time; ok=false (no error) when CV
// storage is disabled or the caller has none stored.
func (a *API) cvUploadedAt(c *fiber.Ctx, userID int64) (*time.Time, bool) {
	if !a.resume.Enabled() {
		return nil, false
	}
	meta, err := a.resume.Status(c.Context(), userID)
	if err != nil || !meta.Present {
		return nil, false
	}
	return meta.UploadedAt, true
}

// companyInfo returns the raw company_info JSON for the job's company, or "" when the
// company is unknown or has none — the analysis then grounds on the job text alone.
func (a *API) companyInfo(c *fiber.Ctx, companySlug string) string {
	if companySlug == "" {
		return ""
	}
	company, err := a.queries.GetCompany(c.Context(), companySlug)
	if err != nil {
		return ""
	}
	return string(company.CompanyInfo)
}

// profileSkills returns the caller's profile skills for the deterministic anchor, or
// nil when they have no profile (the anchor is then all-missing — still valid).
func (a *API) profileSkills(c *fiber.Ctx, userID int64) []string {
	profile, err := a.userProfile.Get(c.Context(), userID)
	if err != nil {
		return nil
	}
	return profile.Skills
}

// stampsFresh reports whether a cached row still matches the live CV upload time, job
// content hash, and current model. A model change (LLM_MODEL upgrade) invalidates the
// cache so the improved model re-analyzes — the analogue of the enrichment version and
// semantic-embedder staleness guards. Absent-on-both-sides stamps count as unchanged
// (a non-board job with no content_hash is never re-crawled, so its text is stable and
// a NULL stamp must not force an endless recompute); a stamp appearing on one side only
// is a change.
func stampsFresh(row db.GetUserJobAnalysisRow, cvUploadedAt *time.Time, jobHash pgtype.Text, model string) bool {
	return row.Model == model &&
		sameTime(row.CvUploadedAt, cvUploadedAt) &&
		sameText(row.JobContentHash, jobHash)
}

func sameTime(stored pgtype.Timestamptz, live *time.Time) bool {
	if stored.Valid != (live != nil) {
		return false
	}
	return !stored.Valid || stored.Time.Equal(*live)
}

func sameText(stored, live pgtype.Text) bool {
	if stored.Valid != live.Valid {
		return false
	}
	return !stored.Valid || stored.String == live.String
}

// decodeAnalysis unmarshals a cached analysis blob, returning nil on empty/corrupt data
// (treated as "no analysis" — the caller re-offers a compute).
func decodeAnalysis(blob []byte) *jobfit.Analysis {
	if len(blob) == 0 {
		return nil
	}
	var a jobfit.Analysis
	if err := json.Unmarshal(blob, &a); err != nil {
		return nil
	}
	return &a
}

func tsFromPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
