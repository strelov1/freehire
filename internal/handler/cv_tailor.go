package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
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
	Analysis   *matchanalysis.Analysis `json:"analysis"`
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
	// Gate on points before creating anything: an out-of-credits caller is a 402 and no
	// tailored CV or session is minted. The debit itself lands after the CV exists (below).
	if bal := a.creditsBalance(c.Context(), userID); bal != nil && bal.Remaining < a.credits.Cost(credits.FeatureTailor) {
		return creditsError(c, *bal)
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
	// Charge the tailor cost only once the session is fully minted, so a mint failure never
	// leaves the caller charged for an unusable session (a retry would mint a new CV id and
	// charge again). Idempotent by the new CV id; resuming an existing CV (a different
	// endpoint) never debits. The session already exists, so a debit error — including a
	// rare insufficient-balance race the pre-check let through — is logged, not surfaced.
	if _, err := a.credits.Debit(c.Context(), userID, credits.FeatureTailor, strconv.FormatInt(tailored.ID, 10)); err != nil {
		log.Printf("credits: tailor debit user=%d cv=%d: %v", userID, tailored.ID, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": tailorCVResponse{
		TailorCVID: tailored.ID, BaseCVID: base.ID, Analysis: analysis, CLIToken: token,
	}})
}

// tailorSessionResponse re-establishes a tailoring session for an EXISTING tailored CV (one
// created before session binding, or whose session was lost): the CV + base ids and a freshly
// minted CLI token, so the browser can seed a new agent session bound to the same CV.
type tailorSessionResponse struct {
	TailorCVID int64  `json:"tailor_cv_id"`
	BaseCVID   int64  `json:"base_cv_id"`
	CLIToken   string `json:"cli_token"`
}

// StartTailorSession mints a CLI credential for an existing tailored CV so the workspace can
// resume tailoring when the CV has no bound agent session yet. Cookie-only (the browser starts
// it); 409 when the CV is not a tailored copy. Never calls the LLM.
func (a *API) StartTailorSession(c *fiber.Ctx) error {
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
	base, ok, err := a.cvStore.BaseCV(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "no base CV")
	}
	token, err := mintTailoringKey(c.Context(), a.queries, userID, time.Now())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": tailorSessionResponse{
		TailorCVID: rec.ID, BaseCVID: base.ID, CLIToken: token,
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

// tailorJob is the vacancy the agent reframes toward: enough of the posting to ground the
// reframing in the real role. The description is free text the agent reads as data (tool
// output), never as instructions.
type tailorJob struct {
	Title       string `json:"title"`
	Company     string `json:"company"`
	Slug        string `json:"public_slug"`
	Description string `json:"description"`
}

// tailorContextResponse is the reasoning context the agent reads (freehire cv context): the
// vacancy, the verdict and recommendation, per-dimension comments, and the requirement split
// the honest wall turns on — missing_have (reframe existing evidence) vs missing_gap (ask first).
type tailorContextResponse struct {
	Job            tailorJob            `json:"job"`
	Verdict        string               `json:"verdict"`
	OverallScore   int                  `json:"overall_score"`
	Recommendation string               `json:"recommendation"`
	Dimensions     []matchanalysis.Dimension   `json:"dimensions"`
	MissingHave    []matchanalysis.Requirement `json:"missing_have"`
	MissingGap     []matchanalysis.Requirement `json:"missing_gap"`
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
	job, err := a.queries.GetJob(c.Context(), rec.JobID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": tailorContext(analysis, job)})
}

// cachedAnalysis loads the cached fit analysis for (user, job), or a 409 telling the caller to
// run the fit analysis first when none is cached (or the cached blob is empty/corrupt). It
// never recomputes.
func (a *API) cachedAnalysis(c *fiber.Ctx, userID, jobID int64) (*matchanalysis.Analysis, error) {
	row, err := a.matchAnalysisCache.GetUserJobAnalysis(c.Context(), db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
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

// tailorContext projects an analysis + its vacancy to the agent's reasoning context, splitting
// requirements into the reframe-able (missing-have) and the genuine gaps (missing-gap).
func tailorContext(a *matchanalysis.Analysis, job db.Job) tailorContextResponse {
	var have, gap []matchanalysis.Requirement
	for _, r := range a.RequirementMatch {
		switch r.Status {
		case matchanalysis.StatusMissingHave:
			have = append(have, r)
		case matchanalysis.StatusMissingGap:
			gap = append(gap, r)
		}
	}
	return tailorContextResponse{
		Job: tailorJob{
			Title:       job.Title,
			Company:     job.Company,
			Slug:        job.PublicSlug,
			Description: job.Description,
		},
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
