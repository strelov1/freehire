package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobfit"
	"github.com/strelov1/freehire/internal/jobmatch"
)

// fitAnalysisLimit caps how many distinct jobs a user may run the AI fit analysis on
// within fitAnalysisWindow (a rolling window). A recompute of an already-analyzed job is
// free — the meter counts distinct jobs, not LLM calls (see enforceFitQuota). Kept as
// package constants; an env-tunable limit is a trivial future seam.
const (
	fitAnalysisLimit  int64 = 10
	fitAnalysisWindow       = 30 * 24 * time.Hour
)

// fitLimitMessage is the 429 body when a new-job analysis would exceed the quota,
// derived from the constants so the numbers can't drift from what's enforced.
var fitLimitMessage = fmt.Sprintf("Monthly AI fit-analysis limit reached (%d per %d days).",
	fitAnalysisLimit, int64(fitAnalysisWindow/(24*time.Hour)))

// fitQuota is the caller's fit-analysis usage over the rolling window, surfaced on the
// read endpoint so the client can show "N/limit" and pre-block a new-job analysis.
type fitQuota struct {
	Used      int64 `json:"used"`
	Limit     int64 `json:"limit"`
	Remaining int64 `json:"remaining"`
}

// newFitQuota builds the quota view from the used count, flooring remaining at zero (a
// user over the cap reports 0 remaining, never a negative).
func newFitQuota(used int64) fitQuota {
	remaining := fitAnalysisLimit - used
	if remaining < 0 {
		remaining = 0
	}
	return fitQuota{Used: used, Limit: fitAnalysisLimit, Remaining: remaining}
}

// exhausted reports whether the caller has no quota left to start a NEW job's analysis.
func (q fitQuota) exhausted() bool { return q.Remaining <= 0 }

// jobFitStore reads/writes the per-(user, job) cached fit analysis and meters usage.
// *db.Queries satisfies it; a fake backs the DB-less handler tests.
type jobFitStore interface {
	GetUserJobAnalysis(ctx context.Context, arg db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error)
	UpsertUserJobAnalysis(ctx context.Context, arg db.UpsertUserJobAnalysisParams) error
	CountRecentUserJobAnalyses(ctx context.Context, arg db.CountRecentUserJobAnalysesParams) (int64, error)
	ListUserJobAnalyses(ctx context.Context, userID int64) ([]db.ListUserJobAnalysesRow, error)
}

// jobFitResponse is the wire shape for the LLM fit analysis. HasCV is false when the
// caller has no stored CV — the SPA then prompts an upload instead of an empty report.
// Stale marks a cached analysis whose CV or job changed since (the SPA offers a
// recompute); Analysis is nil when none is cached or the LLM is unconfigured. Quota is
// set on reads (GET) so the SPA can show usage and pre-block a new-job analysis; it is
// omitted on the compute responses.
type jobFitResponse struct {
	HasCV    bool             `json:"has_cv"`
	Stale    bool             `json:"stale"`
	Analysis *jobfit.Analysis `json:"analysis"`
	Quota    *fitQuota        `json:"quota,omitempty"`
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
		// No CV means no analysis is possible, so usage is moot — skip the count query.
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: false}})
	}
	quota := a.fitQuotaFor(c.Context(), userID)
	row, err := a.jobFitCache.GetUserJobAnalysis(c.Context(), db.GetUserJobAnalysisParams{UserID: userID, JobID: job.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true, Quota: &quota}})
	}
	if err != nil {
		return err
	}
	analysis := decodeAnalysis(row.Analysis)
	if analysis == nil {
		return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true, Quota: &quota}})
	}
	stale := !stampsFresh(row, cvUploadedAt, job.ContentHash, a.jobFit.ModelID())
	return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true, Stale: stale, Analysis: analysis, Quota: &quota}})
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
	// Enforce the monthly quota before touching the LLM: a new job over the cap is a 429,
	// a recompute of an already-analyzed job is always allowed.
	if err := a.enforceFitQuota(c.Context(), userID, job.ID); err != nil {
		return err
	}
	// Capture the CV upload time up front, so the cache is stamped with the CV that was
	// actually analyzed even if the user re-uploads mid-analysis (the three-stage chain
	// takes seconds); re-reading it afterwards would risk stamping a newer CV's time on
	// an older CV's analysis.
	cvUploadedAt, _ := a.cvUploadedAt(c, userID)

	// The caller's profile drives both the deterministic skills anchor and the location
	// dimension; a missing profile is tolerated (zero value → empty skills/preferences).
	profile, _ := a.userProfile.Get(c.Context(), userID)

	analysis, err := a.jobFit.Analyze(c.Context(), jobfit.Input{
		JobTitle:            job.Title,
		JobDescription:      job.Description,
		CompanyInfo:         a.companyInfo(c, job.CompanySlug),
		CVText:              cvText,
		StructuredResume:    a.structuredResumeJSON(c, userID),
		Match:               jobmatch.Compute(job.Skills, profile.Skills),
		JobWorkMode:         job.WorkMode,
		JobRemote:           job.Remote,
		JobLocation:         job.Location,
		JobRegions:          job.Regions,
		JobCountries:        job.Countries,
		LocationPreferences: string(profile.LocationPreferences),
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
	a.cacheAnalysis(c.Context(), userID, job, cvUploadedAt, analysis)
	return c.JSON(fiber.Map{"data": jobFitResponse{HasCV: true, Stale: false, Analysis: analysis}})
}

// fitQuotaFor reports the caller's fit-analysis usage over the rolling window. A count
// error degrades to zero usage (best-effort: a transient DB hiccup must not block a
// legitimate analysis, and the persisted rows remain the real ceiling).
func (a *API) fitQuotaFor(ctx context.Context, userID int64) fitQuota {
	used, err := a.jobFitCache.CountRecentUserJobAnalyses(ctx, db.CountRecentUserJobAnalysesParams{
		UserID:    userID,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-fitAnalysisWindow), Valid: true},
	})
	if err != nil {
		log.Printf("jobfit: count recent analyses for user %d: %v", userID, err)
		return newFitQuota(0)
	}
	return newFitQuota(used)
}

// enforceFitQuota returns a 429 error when starting a NEW analysis for (userID, jobID)
// would exceed the caller's quota; it returns nil (allow) otherwise. A recompute — a row
// already exists for the pair — is always allowed and never metered, so the meter counts
// distinct jobs rather than LLM calls.
func (a *API) enforceFitQuota(ctx context.Context, userID, jobID int64) error {
	_, err := a.jobFitCache.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err == nil {
		return nil // recompute of an already-analyzed pair — free
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err // genuine DB error, not "no row"
	}
	if a.fitQuotaFor(ctx, userID).exhausted() {
		return fiber.NewError(fiber.StatusTooManyRequests, fitLimitMessage)
	}
	return nil
}

// cacheAnalysis upserts the analysis stamped with the analyzed CV's upload time, the job
// content hash, and the model that produced it. It takes a plain context (not the fiber
// ctx) so the SSE stream can cache after the request handler has returned. Best-effort:
// a cache failure is logged, not surfaced.
func (a *API) cacheAnalysis(ctx context.Context, userID int64, job db.Job, cvUploadedAt *time.Time, analysis *jobfit.Analysis) {
	blob, err := json.Marshal(analysis)
	if err != nil {
		return
	}
	if err := a.jobFitCache.UpsertUserJobAnalysis(ctx, db.UpsertUserJobAnalysisParams{
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

// structuredResumeJSON returns the caller's current structured résumé as JSON for the
// fit input, or "" when the caller has none current (no résumé, unconfigured LLM, not
// yet extracted, or stale) — the fit chain then runs on the CV text alone. Best-effort:
// a read/marshal error degrades to "".
func (a *API) structuredResumeJSON(c *fiber.Ctx, userID int64) string {
	if !a.resume.Enabled() {
		return ""
	}
	st, ok, err := a.resume.Structured(c.Context(), userID)
	if err != nil || !ok {
		return ""
	}
	blob, err := json.Marshal(st)
	if err != nil {
		return ""
	}
	return string(blob)
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

// stampsFresh reports whether a cached row still matches the live CV upload time, job
// content hash, and current model. A model change (LLM_MODEL upgrade) invalidates the
// cache so the improved model re-analyzes — the analogue of the enrichment version and
// semantic-embedder staleness guards. Absent-on-both-sides stamps count as unchanged
// (a non-board job with no content_hash is never re-crawled, so its text is stable and
// a NULL stamp must not force an endless recompute); a stamp appearing on one side only
// is a change.
func stampsFresh(row db.GetUserJobAnalysisRow, cvUploadedAt *time.Time, jobHash pgtype.Text, model string) bool {
	return stampsMatch(row.Model, row.CvUploadedAt, row.JobContentHash, cvUploadedAt, jobHash, model)
}

// stampsMatch is stampsFresh over the raw stored stamps, so callers holding a different
// row type (e.g. the analysed-jobs list) can reuse the same freshness rule.
func stampsMatch(storedModel string, storedCV pgtype.Timestamptz, storedHash pgtype.Text, liveCV *time.Time, liveHash pgtype.Text, liveModel string) bool {
	return storedModel == liveModel &&
		sameTime(storedCV, liveCV) &&
		sameText(storedHash, liveHash)
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
