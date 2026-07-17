package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobfit"
)

// tailoringKeyTTL bounds how long the minted CLI credential is valid. A tailoring session is
// interactive and short; a couple of hours covers it while limiting the blast radius of a key
// that leaks out of the agent's environment.
const tailoringKeyTTL = 2 * time.Hour

// apiKeyMinter is the slice of the query surface mintTailoringKey needs (*db.Queries satisfies
// it), kept narrow so the mint logic is unit-testable without a database.
type apiKeyMinter interface {
	CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) (db.CreateAPIKeyRow, error)
}

// mintTailoringKey issues a short-lived API key the tailoring agent's CLI uses to act as the
// user against the CV endpoints. It reuses the api_keys machinery; there is no per-endpoint
// scope, so the key is owner-scoped only — the CV endpoints' own owner checks confine it to
// this user's CVs. The plaintext token is returned once (to hand to the agent session) and
// only its hash is stored.
func mintTailoringKey(ctx context.Context, q apiKeyMinter, userID int64, now time.Time) (string, error) {
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		return "", err
	}
	if _, err := q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		UserID:      userID,
		Name:        "CV tailoring session",
		TokenHash:   hash,
		TokenPrefix: prefix,
		ExpiresAt:   pgtype.Timestamptz{Time: now.Add(tailoringKeyTTL), Valid: true},
	}); err != nil {
		return "", err
	}
	return token, nil
}

type tailorCVRequest struct {
	JobSlug string `json:"job_slug"`
}

// tailorCVResponse is what the fit-page CTA gets back: the ids of the new tailored CV and the
// base it was copied from, the cached analysis (so the client need not refetch), and the
// short-lived CLI token the agent session authenticates with.
type tailorCVResponse struct {
	TailorCVID int64            `json:"tailor_cv_id"`
	BaseCVID   int64            `json:"base_cv_id"`
	Analysis   *jobfit.Analysis `json:"analysis"`
	CLIToken   string           `json:"cli_token"`
}

// TailorCV bootstraps a tailoring session for a vacancy: it requires a cached fit analysis
// (409 otherwise), ensures the user has a base CV (seeding one from their résumé, 409 when
// they have none), creates a vacancy-bound tailored copy, mints the CLI credential, and
// returns the ids plus the analysis. Cookie-only (the browser starts tailoring); never calls
// the LLM.
func (a *API) TailorCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in tailorCVRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	slug := strings.TrimSpace(in.JobSlug)
	if slug == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job_slug is required")
	}
	job, err := a.queries.GetJobBySlug(c.Context(), slug)
	if err != nil {
		return err // unknown slug → pgx.ErrNoRows → 404 via RenderError
	}
	analysis, err := a.cachedAnalysis(c, userID, job.ID)
	if err != nil {
		return err
	}
	base, tailored, err := a.cvStore.Tailor(c.Context(), userID, job.ID, tailoredCVTitle(job.Title), a.resume)
	if errors.Is(err, cv.ErrNoResume) {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before tailoring")
	}
	if err != nil {
		return mapCVError(err)
	}
	token, err := mintTailoringKey(c.Context(), a.queries, userID, time.Now())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": tailorCVResponse{
		TailorCVID: tailored.ID, BaseCVID: base.ID, Analysis: analysis, CLIToken: token,
	}})
}

// PatchCV applies one field-level patch to an owned CV. Cookie or API key (the agent's CLI
// uses the key). Bad addressing is a 422; a foreign/missing id is a 404.
func (a *API) PatchCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var p cv.Patch
	if err := c.BodyParser(&p); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	meta, err := a.cvStore.Patch(c.Context(), int64(id), userID, p)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}

// tailorContextResponse is the reasoning context the agent reads (freehire cv context): the
// verdict and recommendation, per-dimension comments, and the requirement split the honest
// wall turns on — missing_have (reframe existing evidence) vs missing_gap (must ask first).
type tailorContextResponse struct {
	Verdict        string               `json:"verdict"`
	OverallScore   int                  `json:"overall_score"`
	Recommendation string               `json:"recommendation"`
	Dimensions     []jobfit.Dimension   `json:"dimensions"`
	MissingHave    []jobfit.Requirement `json:"missing_have"`
	MissingGap     []jobfit.Requirement `json:"missing_gap"`
	Strengths      []string             `json:"strengths"`
	Gaps           []string             `json:"gaps"`
}

// TailorContext serves the cached fit analysis for a tailored CV, projected to the tailoring
// reasoning context. Cookie or API key. 409 when the CV is not a tailored copy (no bound
// vacancy) or has no cached analysis; never calls the LLM.
func (a *API) TailorContext(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := a.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	if rec.JobID == 0 {
		return fiber.NewError(fiber.StatusConflict, "not a tailored CV")
	}
	analysis, err := a.cachedAnalysis(c, userID, rec.JobID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": tailorContext(analysis)})
}

// cachedAnalysis loads the cached fit analysis for (user, job), or a 409 telling the caller to
// run the fit analysis first when none is cached (or the cached blob is empty/corrupt). It
// never recomputes.
func (a *API) cachedAnalysis(c *fiber.Ctx, userID, jobID int64) (*jobfit.Analysis, error) {
	row, err := a.jobFitCache.GetUserJobAnalysis(c.Context(), db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fiber.NewError(fiber.StatusConflict, "run the fit analysis first")
	}
	if err != nil {
		return nil, err
	}
	analysis := decodeAnalysis(row.Analysis)
	if analysis == nil {
		return nil, fiber.NewError(fiber.StatusConflict, "run the fit analysis first")
	}
	return analysis, nil
}

// tailorContext projects an analysis to the agent's reasoning context, splitting requirements
// into the reframe-able (missing-have) and the genuine gaps (missing-gap).
func tailorContext(a *jobfit.Analysis) tailorContextResponse {
	var have, gap []jobfit.Requirement
	for _, r := range a.RequirementMatch {
		switch r.Status {
		case jobfit.StatusMissingHave:
			have = append(have, r)
		case jobfit.StatusMissingGap:
			gap = append(gap, r)
		}
	}
	return tailorContextResponse{
		Verdict:        a.Verdict,
		OverallScore:   a.OverallScore,
		Recommendation: a.Recommendation,
		Dimensions:     a.Dimensions,
		MissingHave:    have,
		MissingGap:     gap,
		Strengths:      a.Strengths,
		Gaps:           a.Gaps,
	}
}

// tailoredCVTitle names a tailored copy from the vacancy title (bounded/defaulted like any CV
// title).
func tailoredCVTitle(jobTitle string) string {
	jobTitle = strings.TrimSpace(jobTitle)
	if jobTitle == "" {
		return "Tailored CV"
	}
	return cvTitle("Tailored — " + jobTitle)
}
