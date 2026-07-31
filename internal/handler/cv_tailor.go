package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
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
	TailorCVID string                  `json:"tailor_cv_id"`
	BaseCVID   string                  `json:"base_cv_id"`
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
	base, tailored, err := h.cvStore.Tailor(c.Context(), userID, job.ID, tailoredCVTitle(job.Title), h.seedSource())
	if errors.Is(err, cv.ErrNoResume) {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before tailoring")
	}
	if err != nil {
		return mapCVError(err)
	}
	// Open the copy's history with where it came from, so the feed starts at the beginning
	// rather than mid-story with the first edit. Best-effort and idempotency-aware: Tailor
	// returns the EXISTING copy on a reload, and a second milestone would be a second
	// "created from your base CV" under a CV that was created once.
	if tailored.CreatedAt.Equal(tailored.UpdatedAt) {
		if _, err := h.editor.Seed(c.Context(), tailored.ID, userID, "Created from your base CV"); err != nil {
			log.Printf("cv: seeding the revision history for %s: %v", tailored.ID, err)
		}
	}
	// A reload of /tailor/<slug> re-runs this request. The CV it reaches is the one that
	// already exists (Tailor is idempotent per vacancy), so its conversation must be reached
	// too — minting a second session here would rebind the CV and orphan everything the
	// candidate had already said, which is exactly what "my chat disappeared" was.
	sessionID, err := h.existingTailoringSession(c.Context(), userID, tailored.ID)
	if err != nil {
		return err
	}
	if sessionID == "" {
		sessionID, err = h.startTailoringSession(c.Context(), userID, tailored.ID, job.ID)
		if err != nil {
			return err
		}
	}
	// Charge the tailor cost only once the session is fully minted, so a mint failure never
	// leaves the caller charged for an unusable session (a retry would mint a new CV id and
	// charge again). Idempotent by the new CV id; resuming an existing CV (a different
	// endpoint) never debits. The session already exists, so a debit error — including a
	// rare insufficient-balance race the pre-check let through — is logged, not surfaced.
	if _, err := h.credits.Debit(c.Context(), userID, credits.FeatureTailor, tailored.ID.String()); err != nil {
		log.Printf("credits: tailor debit user=%d cv=%d: %v", userID, tailored.ID, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": tailorCVResponse{
		TailorCVID: tailored.ID.String(), BaseCVID: base.ID.String(), Analysis: analysis, SessionID: sessionID,
	}})
}

// existingTailoringSession reports the conversation already bound to a tailored CV, or "" when
// it has none. A session id that no longer resolves to a live conversation counts as none: the
// binding is text on the CV, and a deleted conversation must not strand the workspace.
func (h *cvHandlers) existingTailoringSession(ctx context.Context, userID int64, cvID uuid.UUID) (string, error) {
	rec, err := h.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		return "", mapCVError(err)
	}
	if rec.AgentSessionID == "" {
		return "", nil
	}
	if h.assistantSessions == nil {
		return rec.AgentSessionID, nil
	}
	id, err := uuid.Parse(rec.AgentSessionID)
	if err != nil {
		return "", nil
	}
	if _, err := h.assistantSessions.Session(ctx, id, userID); err != nil {
		return "", nil
	}
	return rec.AgentSessionID, nil
}

// tailorSessionResponse re-establishes a tailoring session for an EXISTING tailored CV (one
// created before session binding, or whose session was lost): the CV + base ids and a freshly
// minted CLI token, so the browser can seed a new agent session bound to the same CV.
type tailorSessionResponse struct {
	TailorCVID string `json:"tailor_cv_id"`
	BaseCVID   string `json:"base_cv_id"`
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
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	rec, err := h.cvStore.Get(c.Context(), id, userID)
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
		TailorCVID: rec.ID.String(), BaseCVID: base.ID.String(), SessionID: sessionID,
	}})
}

// startTailoringSession creates the conversation a tailoring workspace runs in: a
// tailor-preset session bound to the CV and its vacancy, stored on the CV so
// reopening the workspace resumes the same chat instead of starting over. The
// binding is what confines the CV tools — they close over these ids rather than
// taking them from the model — so it is created here, where ownership of both is
// already established.
func (h *cvHandlers) startTailoringSession(ctx context.Context, userID int64, cvID uuid.UUID, jobID int64) (string, error) {
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

// PatchCV applies a batch of path operations to an owned CV. Cookie or API key (the agent's
// CLI uses the key). Bad addressing is a 422; a foreign or missing id is a 404.
//
// The actor follows the credential rather than the body: a key is what the tailoring agent
// holds, so a keyed caller edits as the agent and meets the agent's policy — the contact
// block is closed to it, and an operation that states something about the candidate has to
// cite banked evidence. A cookie is the candidate at their own keyboard.
func (h *cvHandlers) PatchCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}

	// Decode strictly: an unknown field or a type mismatch fails with a reason the caller can
	// act on, rather than being ignored and editing something else.
	var in struct {
		Ops  []cvedit.Op `json:"ops"`
		Note string      `json:"note"`
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "invalid operations: "+err.Error())
	}
	for i, op := range in.Ops {
		if _, err := cvedit.ParsePath(string(op.Path)); err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity,
				fmt.Sprintf("operation %d: %s", i+1, err))
		}
	}

	actor := cvedit.ActorCandidate
	if auth.ViaAPIKey(c) {
		actor = cvedit.ActorAgent
	}
	meta, _, err := h.editor.Commit(c.Context(), id, userID, cvedit.Change{
		Actor:  actor,
		Origin: cvedit.OriginCLI,
		Note:   in.Note,
		Ops:    in.Ops,
	})
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

// tailorDescriptionLimit bounds the posting inside a tailoring context. A recorded session
// spent 11.4 KB of its opening round on one description in raw HTML — more than a third of the
// whole conversation, before the agent had done anything.
const tailorDescriptionLimit = 6000

// clipRunes bounds a string on a rune boundary, so a multi-byte character is never cut in half.
func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
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
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	rec, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	if rec.JobID == 0 {
		return fiber.NewError(fiber.StatusConflict, "not a tailored CV")
	}
	ctx, err := h.tailoringContext(c.Context(), userID, rec.JobID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": ctx})
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
			Title:   job.Title,
			Company: job.Company,
			Slug:    job.PublicSlug,
			// The posting is the largest thing in the turn and the least trusted: it reaches
			// the model as words, bounded, the same way get_job already serves it. Sending
			// markup spends the context window on tags and widens what there is to misread.
			Description: clipRunes(formatDescription(job.Description, "markdown"), tailorDescriptionLimit),
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

// tailoringContext assembles what a tailoring reader needs: the cached analysis, projected
// over the vacancy it is about. Both the HTTP endpoint and the agent's tool go through here,
// so neither can drift into reading a different analysis or a different vacancy.
func (h *cvHandlers) tailoringContext(ctx context.Context, userID, jobID int64) (tailorContextResponse, error) {
	analysis, err := h.cachedAnalysisCtx(ctx, userID, jobID)
	if err != nil {
		return tailorContextResponse{}, err
	}
	job, err := h.jobReader.GetJob(ctx, jobID)
	if err != nil {
		return tailorContextResponse{}, err
	}
	return tailorContext(analysis, job), nil
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
