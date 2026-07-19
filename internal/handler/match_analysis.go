package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/jobmatch"
)

// creditsError writes the 402 Payment Required body when a metered action can't be
// afforded: a message plus the caller's remaining points and the date the monthly grant
// resets, so the SPA can render an out-of-credits state.
func creditsError(c *fiber.Ctx, bal credits.Balance) error {
	return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
		"error":     "You're out of AI credits for this month.",
		"remaining": bal.Remaining,
		"resets_at": bal.ResetsAt,
	})
}

// matchAnalysisStore reads/writes the per-(user, job) cached fit analysis. *db.Queries satisfies
// it; a fake backs the DB-less handler tests.
type matchAnalysisStore interface {
	GetUserJobAnalysis(ctx context.Context, arg db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error)
	UpsertUserJobAnalysis(ctx context.Context, arg db.UpsertUserJobAnalysisParams) error
	ListUserJobAnalyses(ctx context.Context, userID int64) ([]db.ListUserJobAnalysesRow, error)
}

// matchAnalysisResponse is the wire shape for the LLM fit analysis. HasCV is false when the
// caller has no stored CV — the SPA then prompts an upload instead of an empty report.
// Stale marks a cached analysis whose CV or job changed since (the SPA offers a
// recompute); Analysis is nil when none is cached or the LLM is unconfigured. Credits is
// set on reads (GET) so the SPA can show the points balance and pre-block a new-job
// analysis; it is omitted on the compute responses.
type matchAnalysisResponse struct {
	HasCV    bool             `json:"has_cv"`
	Stale    bool             `json:"stale"`
	Analysis *matchanalysis.Analysis `json:"analysis"`
	Credits  *credits.Balance `json:"credits,omitempty"`
}

// GetMatchAnalysis serves the cached fit analysis for one of the caller's jobs, never calling
// the LLM. It returns the cached analysis (flagged stale when the CV or job changed
// since it was computed), or a null analysis when none is cached. Cookie or API key;
// an unknown slug is a 404. has_cv=false (no LLM ever) when no CV is stored.
func (a *API) GetMatchAnalysis(c *fiber.Ctx) error {
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
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: false}})
	}
	bal := a.creditsBalance(c.Context(), userID)
	row, err := a.matchAnalysisCache.GetUserJobAnalysis(c.Context(), db.GetUserJobAnalysisParams{UserID: userID, JobID: job.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Credits: bal}})
	}
	if err != nil {
		return err
	}
	analysis := decodeAnalysis(row.Analysis)
	if analysis == nil {
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Credits: bal}})
	}
	stale := !stampsFresh(row, cvUploadedAt, job.ContentHash, a.matchAnalysis.ModelID())
	return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Stale: stale, Analysis: analysis, Credits: bal}})
}

// PostMatchAnalysis runs the three-stage fit prompt-chain over the caller's stored CV and the
// job, caches the result per (user, job), and returns it fresh. Best-effort: an
// unconfigured or failing LLM returns has_cv with a null analysis (200) and caches
// nothing. Cookie or API key; unknown slug 404; has_cv=false when no CV is stored.
func (a *API) PostMatchAnalysis(c *fiber.Ctx) error {
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
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: false}})
	}
	// Gate on points before touching the LLM: a new job needs at least the match cost, a
	// recompute of an already-analyzed job is always free. Only new analyses are charged,
	// and only after they persist (below), so a legacy cached job re-runs for free.
	isNew, err := a.matchIsNew(c.Context(), userID, job.ID)
	if err != nil {
		return err
	}
	if isNew {
		bal := a.creditsBalance(c.Context(), userID)
		if bal != nil && bal.Remaining < a.credits.Cost(credits.FeatureMatch) {
			return creditsError(c, *bal)
		}
	}
	// Capture the CV upload time up front, so the cache is stamped with the CV that was
	// actually analyzed even if the user re-uploads mid-analysis (the three-stage chain
	// takes seconds); re-reading it afterwards would risk stamping a newer CV's time on
	// an older CV's analysis.
	cvUploadedAt, _ := a.cvUploadedAt(c, userID)

	// The caller's profile drives both the deterministic skills anchor and the location
	// dimension; a missing profile is tolerated (zero value → empty skills/preferences).
	profile, _ := a.userProfile.Get(c.Context(), userID)

	analysis, err := a.matchAnalysis.Analyze(c.Context(), matchanalysis.Input{
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
		log.Printf("matchanalysis: analyze failed for user %d job %d: %v", userID, job.ID, err)
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true}})
	}
	if analysis == nil {
		// LLM unconfigured — nothing to cache.
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true}})
	}
	a.cacheAnalysis(c.Context(), userID, job, cvUploadedAt, analysis)
	if isNew {
		a.debitMatch(c.Context(), userID, job.ID)
	}
	return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Stale: false, Analysis: analysis}})
}

// creditsBalance reports the caller's current points, or nil on a DB error (logged).
// Best-effort: a transient hiccup must neither block a legitimate analysis nor 402 the
// caller — the atomic Debit remains the real ceiling.
func (a *API) creditsBalance(ctx context.Context, userID int64) *credits.Balance {
	bal, err := a.credits.Balance(ctx, userID)
	if err != nil {
		log.Printf("credits: balance for user %d: %v", userID, err)
		return nil
	}
	return &bal
}

// matchIsNew reports whether analysing (userID, jobID) would be the caller's FIRST
// analysis of that job — i.e. no cached row exists. A recompute (row present) is free and
// never charged, so a legacy analysis cached before credits shipped re-runs for free.
func (a *API) matchIsNew(ctx context.Context, userID, jobID int64) (bool, error) {
	_, err := a.matchAnalysisCache.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err == nil {
		return false, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return false, err
}

// debitMatch charges one match against the caller's points after a fresh analysis has
// persisted, idempotent by job id. Best-effort: the analysis is already computed and
// cached, so a debit error (including a rare insufficient-balance race the pre-check let
// through) is logged, not surfaced.
func (a *API) debitMatch(ctx context.Context, userID, jobID int64) {
	if _, err := a.credits.Debit(ctx, userID, credits.FeatureMatch, strconv.FormatInt(jobID, 10)); err != nil {
		log.Printf("credits: match debit user=%d job=%d: %v", userID, jobID, err)
	}
}

// cacheAnalysis upserts the analysis stamped with the analyzed CV's upload time, the job
// content hash, and the model that produced it. It takes a plain context (not the fiber
// ctx) so the SSE stream can cache after the request handler has returned. Best-effort:
// a cache failure is logged, not surfaced.
func (a *API) cacheAnalysis(ctx context.Context, userID int64, job db.Job, cvUploadedAt *time.Time, analysis *matchanalysis.Analysis) {
	blob, err := json.Marshal(analysis)
	if err != nil {
		return
	}
	if err := a.matchAnalysisCache.UpsertUserJobAnalysis(ctx, db.UpsertUserJobAnalysisParams{
		UserID:         userID,
		JobID:          job.ID,
		Analysis:       blob,
		Model:          a.matchAnalysis.ModelID(),
		CvUploadedAt:   tsFromPtr(cvUploadedAt),
		JobContentHash: job.ContentHash,
	}); err != nil {
		log.Printf("matchanalysis: cache analysis for user %d job %d: %v", userID, job.ID, err)
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
func decodeAnalysis(blob []byte) *matchanalysis.Analysis {
	if len(blob) == 0 {
		return nil
	}
	var a matchanalysis.Analysis
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
