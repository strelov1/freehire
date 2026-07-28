package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
)

type tailorCVRequest struct {
	JobSlug string `json:"job_slug"`
}

// tailorCVResponse is what the fit-page CTA gets back: the ids of the new tailored CV and the
// base it was copied from, the cached analysis (so the client need not refetch), and the
// short-lived CLI token the agent session authenticates with.
type tailorCVResponse struct {
	TailorCVID int64                   `json:"tailor_cv_id"`
	BaseCVID   int64                   `json:"base_cv_id"`
	Analysis   *matchanalysis.Analysis `json:"analysis"`
	SessionID  string                  `json:"session_id"`
}

// TailorCV bootstraps a tailoring session for a vacancy: it requires a cached fit analysis
// (409 otherwise), ensures the user has a base CV (seeding one from their résumé, 409 when
// they have none), creates a vacancy-bound tailored copy, mints the CLI credential, and
// returns the ids plus the analysis. Cookie-only (the browser starts tailoring); never calls
// the LLM.
func (h *cvHandlers) TailorCV(c *fiber.Ctx) error {
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
	job, err := h.queries.GetJobBySlug(c.Context(), slug)
	if err != nil {
		return err // unknown slug → pgx.ErrNoRows → 404 via RenderError
	}
	analysis, err := h.cachedAnalysis(c, userID, job.ID)
	if err != nil {
		return err
	}
	// Attach the hard-constraint blockers + score ceiling to the analysis the tailoring
	// agent receives, so its Action strings ("do not claim X unless true") guard the
	// tailored output against fabricating a credential/degree/authorization.
	h.match.capServedAnalysis(c.Context(), userID, job, analysis)
	// Gate on points before creating anything: an out-of-credits caller is a 402 and no
	// tailored CV or session is minted. The debit itself lands after the CV exists (below).
	if bal := h.match.creditsBalance(c.Context(), userID); bal != nil && bal.Remaining < h.credits.Cost(credits.FeatureTailor) {
		return creditsError(c, *bal)
	}
	base, tailored, err := h.cvStore.Tailor(c.Context(), userID, job.ID, tailoredCVTitle(job.Title), h.resume)
	if errors.Is(err, cv.ErrNoResume) {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before tailoring")
	}
	if err != nil {
		return mapCVError(err)
	}
	sessionID, err := h.startTailoringSession(c.Context(), userID, tailored.ID, job.ID)
	if err != nil {
		return err
	}
	// Charge the tailor cost only once the session is fully minted, so a mint failure never
	// leaves the caller charged for an unusable session (a retry would mint a new CV id and
	// charge again). Idempotent by the new CV id; resuming an existing CV (a different
	// endpoint) never debits. The session already exists, so a debit error — including a
	// rare insufficient-balance race the pre-check let through — is logged, not surfaced.
	if _, err := h.credits.Debit(c.Context(), userID, credits.FeatureTailor, strconv.FormatInt(tailored.ID, 10)); err != nil {
		log.Printf("credits: tailor debit user=%d cv=%d: %v", userID, tailored.ID, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": tailorCVResponse{
		TailorCVID: tailored.ID, BaseCVID: base.ID, Analysis: analysis, SessionID: sessionID,
	}})
}

// tailorSessionResponse re-establishes a tailoring session for an EXISTING tailored CV (one
// created before session binding, or whose session was lost): the CV + base ids and a freshly
// minted CLI token, so the browser can seed a new agent session bound to the same CV.
type tailorSessionResponse struct {
	TailorCVID int64  `json:"tailor_cv_id"`
	BaseCVID   int64  `json:"base_cv_id"`
	SessionID  string `json:"session_id"`
}

// StartTailorSession mints a CLI credential for an existing tailored CV so the workspace can
// resume tailoring when the CV has no bound agent session yet. Cookie-only (the browser starts
// it); 409 when the CV is not a tailored copy. Never calls the LLM.
func (h *cvHandlers) StartTailorSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := h.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	if rec.JobID == 0 {
		return fiber.NewError(fiber.StatusConflict, "not a tailored CV")
	}
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "no base CV")
	}
	sessionID, err := h.startTailoringSession(c.Context(), userID, rec.ID, rec.JobID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": tailorSessionResponse{
		TailorCVID: rec.ID, BaseCVID: base.ID, SessionID: sessionID,
	}})
}

// startTailoringSession creates the conversation a tailoring workspace runs in: a
// tailor-preset session bound to the CV and its vacancy, stored on the CV so
// reopening the workspace resumes the same chat instead of starting over. The
// binding is what confines the CV tools — they close over these ids rather than
// taking them from the model — so it is created here, where ownership of both is
// already established.
func (h *cvHandlers) startTailoringSession(ctx context.Context, userID, cvID, jobID int64) (string, error) {
	if h.assistantSessions == nil {
		return "", fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	sess, err := h.assistantSessions.CreateSession(ctx, userID, assistant.PresetTailor, &cvID, &jobID)
	if err != nil {
		return "", err
	}
	id := sess.ID.String()
	if err := h.cvStore.SetSession(ctx, cvID, userID, id); err != nil {
		return "", err
	}
	return id, nil
}

// PatchCV applies one field-level patch to an owned CV. Cookie or API key (the agent's CLI
// uses the key). Bad addressing is a 422; a foreign/missing id is a 404.
func (h *cvHandlers) PatchCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	// Decode strictly: reject unknown fields and type mismatches so a mis-addressed
	// op (a stray "skill" field, a numeric "group") fails with a reason the agent can
	// act on, instead of being silently ignored and editing the wrong section.
	p, err := cv.DecodePatch(c.Body())
	if err != nil {
		return mapCVError(err)
	}
	// The tailoring agent (API-key caller) never sees the contact block and must not be able
	// to write it either — reject a header-contact patch so the stored identifiers stay ours.
	if auth.ViaAPIKey(c) && p.Op == cv.PatchSetHeaderField && isContactHeaderField(p.Field) {
		return fiber.NewError(fiber.StatusForbidden, "contact fields are not editable in a tailoring session")
	}
	meta, err := h.cvStore.Patch(c.Context(), int64(id), userID, p)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}

// isContactHeaderField reports whether a set_header_field patch targets a direct contact
// identifier the tailoring agent must never write (location is allowed — it is not PII).
func isContactHeaderField(field string) bool {
	switch field {
	case "full_name", "email", "phone":
		return true
	}
	return false
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
	Job            tailorJob                   `json:"job"`
	Verdict        string                      `json:"verdict"`
	OverallScore   int                         `json:"overall_score"`
	Recommendation string                      `json:"recommendation"`
	Dimensions     []matchanalysis.Dimension   `json:"dimensions"`
	MissingHave    []matchanalysis.Requirement `json:"missing_have"`
	MissingGap     []matchanalysis.Requirement `json:"missing_gap"`
	Strengths      []string                    `json:"strengths"`
	Gaps           []string                    `json:"gaps"`
}

// TailorContext serves the cached fit analysis for a tailored CV, projected to the tailoring
// reasoning context. Cookie or API key. 409 when the CV is not a tailored copy (no bound
// vacancy) or has no cached analysis; never calls the LLM.
func (h *cvHandlers) TailorContext(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := h.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	if rec.JobID == 0 {
		return fiber.NewError(fiber.StatusConflict, "not a tailored CV")
	}
	analysis, err := h.cachedAnalysis(c, userID, rec.JobID)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJob(c.Context(), rec.JobID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": tailorContext(analysis, job)})
}

// cachedAnalysis loads the cached fit analysis for (user, job), or a 409 telling the caller to
// run the fit analysis first when none is cached (or the cached blob is empty/corrupt). It
// never recomputes.
func (h *cvHandlers) cachedAnalysis(c *fiber.Ctx, userID, jobID int64) (*matchanalysis.Analysis, error) {
	return h.cachedAnalysisCtx(c.Context(), userID, jobID)
}

// cachedAnalysisCtx is cachedAnalysis over a plain context, so the assistant's CV
// tools — which have no fiber request — read the analysis through the same path
// the HTTP endpoints do.
func (h *cvHandlers) cachedAnalysisCtx(ctx context.Context, userID, jobID int64) (*matchanalysis.Analysis, error) {
	row, err := h.matchAnalysisCache.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
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
