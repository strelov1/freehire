package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TailorCV bootstraps a tailoring session for a vacancy: it ensures the user has a base CV
// (seeding one from their résumé, 409 when they have none), creates a vacancy-bound tailored
// copy, mints the CLI credential, and returns the ids plus the cached analysis if one already
// exists — no cached analysis is required to start. Cookie or API key: the browser starts
// tailoring, and so does a CLI holding the caller's own key, which is what lets an agent
// enter the cycle instead of being handed a CV id copied out of a browser URL. Never calls
// the LLM.
func (h *cvHandlers) TailorCV(ctx context.Context, req *apigen.CvTailorRequest) (apigen.TailorCVRes, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(req.JobSlug.Value)
	if slug == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "job_slug is required")
	}
	baseID, err := h.requestedTailorBase(ctx, userID, req.BaseCvID.Value)
	if err != nil {
		return nil, err
	}
	job, err := h.queries.GetJobBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if err := jobVisibleToCaller(job, userID, true); err != nil {
		return nil, err
	}
	analysis := h.optionalAnalysis(ctx, userID, job.ID)
	// Attach the hard-constraint blockers + score ceiling to the analysis the tailoring
	// agent receives, so its Action strings ("do not claim X unless true") guard the
	// tailored output against fabricating a credential/degree/authorization.
	h.match.capServedAnalysis(ctx, userID, job, analysis)
	// Gate on the plan before creating anything: a caller starting a NEW tailoring session
	// with none of today's allowance left gets a 402, and no CV, session or model call is
	// made. This is a pre-check on the standing; the charge itself lands once the session
	// exists (below), because only then is there a session id to charge it against.
	//
	// It applies only to a vacancy the caller has not tailored for yet. Returning to a
	// session that already exists costs nothing, and refusing here would lock somebody out
	// of work they have already paid for — the workspace is addressed by vacancy, so a
	// reload arrives at exactly this line.
	if refusal := h.tailoringRefusal(ctx, userID, job.ID); refusal != nil {
		return refusal.cv()
	}
	// When the base CV is behind the latest résumé upload, refresh it from the seed before
	// Tailor copies it into a new vacancy-bound row. Reload of an existing tailored copy
	// still returns that copy unchanged (Tailor is idempotent); the base refresh is a no-op
	// once updated_at catches the upload stamp. Best-effort: this refresh exists to keep the
	// base in sync, not to gate opening the tailor workspace — a role over the bullet cap (or
	// any other refresh failure) must not block bootstrap. Tailor below falls back to the base
	// as it currently stands.
	if err := h.reseedBaseIfStaleVsUpload(ctx, userID, baseID); err != nil {
		log.Printf("cv: base refresh before tailor bootstrap user=%d: %v", userID, err)
	}
	base, tailored, justCreated, err := h.cvStore.Tailor(ctx, userID, job.ID, baseID, tailoredCVTitle(job.Title), h.seedSource(), skilltag.PreferredFromText(job.Description))
	if errors.Is(err, cv.ErrNoResume) {
		return nil, fiber.NewError(fiber.StatusConflict, "add a résumé before tailoring")
	}
	if err != nil {
		return nil, mapCVError(err)
	}
	// Existing vacancy-bound copies (and a base left empty before provisional contacts
	// were usable) heal empty header fields on bootstrap the same way GetCV does.
	if trec, gerr := h.cvStore.Get(ctx, tailored.ID, userID); gerr == nil {
		if _, herr := h.healRecordHeader(ctx, userID, trec); herr != nil {
			log.Printf("cv: healing tailored header on tailor %s: %v", tailored.ID, herr)
		}
	}
	h.healBaseHeaderIfNeeded(ctx, userID)
	// Open the copy's history with where it came from, so the feed starts at the beginning
	// rather than mid-story with the first edit. Best-effort and idempotency-aware: Tailor
	// returns the EXISTING copy on a reload, and a second milestone would be a second
	// "created from your base CV" under a CV that was created once.
	if justCreated {
		if _, err := h.editor.Seed(ctx, tailored.ID, userID, "Created from your base CV"); err != nil {
			log.Printf("cv: seeding the revision history for %s: %v", tailored.ID, err)
		}
	}
	// A reload of /tailor/<slug> re-runs this request. The CV it reaches is the one that
	// already exists (Tailor is idempotent per vacancy), so its conversation must be reached
	// too — minting a second session here would rebind the CV and orphan everything the
	// candidate had already said, which is exactly what "my chat disappeared" was.
	sessionID, err := h.existingTailoringSession(ctx, userID, tailored.ID)
	if err != nil {
		return nil, err
	}
	justStarted := sessionID == ""
	if justStarted {
		sessionID, err = h.startTailoringSession(ctx, userID, tailored.ID, job.ID)
		if err != nil {
			return nil, err
		}
	}
	// Charge the session only once it is fully minted, so a mint failure never leaves the
	// caller charged for an unusable session. The charge is keyed by the session id, which
	// is also what the turn ceiling is counted from — the first charge is that session's
	// first ceiling.
	//
	// Only a session this request created is charged. A reload reaches the existing one and
	// pays nothing, which is the whole reason the workspace can be addressed by vacancy.
	//
	// The session already exists by this point, so a charge error — including the rare race
	// the pre-check let through — is logged rather than surfaced: refusing here would take
	// away a workspace that is already open and already usable.
	if justStarted {
		if _, err := h.plans.StartSession(ctx, userID, sessionID); err != nil {
			log.Printf("plan: charging a tailoring session user=%d session=%s: %v", userID, sessionID, err)
		}
	}
	// Place the vacancy on the Tracking Kanban. A bare bookmark is not enough — the
	// board columns only show staged rows; saved-only lives under Activity → Saved.
	// Stage is set to preparing without applied_at (preparing ≠ submitted); it has its
	// own column and auto-promotes to applied on a real apply signal. Runs on create
	// and resume; never overwrites an existing stage. Best-effort: the CV and session
	// already exist.
	if h.jobs != nil {
		if err := h.jobs.EnsureOnBoard(ctx, userID, job.ID); err != nil {
			log.Printf("cv: placing vacancy on board after tailor user=%d job=%d: %v", userID, job.ID, err)
		}
	}
	// The analysis is the fit-analysis package's own type, shared with the match surface and
	// the tools: it travels as the JSON it marshals to (null when none is cached).
	rawAnalysis, err := json.Marshal(analysis)
	if err != nil {
		return nil, err
	}
	return &apigen.TailorCVCreated{Data: apigen.CvTailorResult{
		TailorCvID: tailored.ID.String(), BaseCvID: base.ID.String(), Analysis: rawAnalysis, SessionID: sessionID,
		ColdStartRunning: justCreated,
	}}, nil
}

// requestedTailorBase validates the optional base_cv_id of a tailor request: empty is the
// default base (uuid.Nil), a malformed id is a 400, a foreign or missing one a 404 and a
// tailored copy a 409.
func (h *cvHandlers) requestedTailorBase(ctx context.Context, userID int64, raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "invalid base_cv_id")
	}
	if _, err := h.cvStore.BaseByID(ctx, id, userID); err != nil {
		return uuid.Nil, mapCVError(err)
	}
	return id, nil
}

// refuseNewTailoring answers the 402 when the caller's tailoring allowance would actually
// turn them away AND this vacancy would be a new session. It reports whether it refused,
// and the error from writing that refusal.
//
// It asks Refuses rather than Exhausted, so the pre-check follows the same enforcement
// switch every other surface does: with tailoring still in shadow the bootstrap runs and
// the ledger simply records that it would not have. Refusing here on a spent allowance
// alone would make one feature enforce while the rest were only counting.
//
// The order matters: the allowance is checked first because it is a cheap read, and the
// "have they tailored this vacancy before" question is only asked of somebody who has run
// out — for everybody else it is a query nobody needs.
//
// Every failure here lets the request through. A standing that cannot be read and a
// tailored-CV lookup that errors are both bookkeeping problems, and refusing a candidate
// because our accounting hiccuped is the worse outcome; the charge below is atomic and
// remains the real ceiling.
func (h *cvHandlers) tailoringRefusal(ctx context.Context, userID, jobID int64) *planRefusal {
	if h.plans == nil {
		return nil
	}
	st, err := h.plans.Standing(ctx, userID, plan.FeatureTailor)
	if err != nil {
		log.Printf("plan: tailoring standing for user %d: %v", userID, err)
		return nil
	}
	if !st.Refuses() {
		return nil
	}
	if _, err := h.queries.GetTailoredCVForJob(ctx, db.GetTailoredCVForJobParams{
		UserID: userID, JobID: pgtype.Int8{Int64: jobID, Valid: true},
	}); err == nil {
		return nil // already tailored for this vacancy: returning to it is free
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("cv: looking up an existing tailored CV for user %d job %d: %v", userID, jobID, err)
		return nil
	}
	refusal := standingRefusal(st)
	return &refusal
}

// refuseNewTailoring is tailoringRefusal for a Fiber handler: it writes the 402 itself and
// reports whether it refused, and the error from writing it.
func (h *cvHandlers) refuseNewTailoring(c *fiber.Ctx, userID, jobID int64) (bool, error) {
	refusal := h.tailoringRefusal(c.Context(), userID, jobID)
	if refusal == nil {
		return false, nil
	}
	return true, refusal.write(c)
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

// StartTailorSession mints a CLI credential for an existing tailored CV so the workspace can
// resume tailoring when the CV has no bound agent session yet. Cookie-only (the browser starts
// it); 409 when the CV is not a tailored copy. Never calls the LLM.
func (h *cvHandlers) StartTailorSession(ctx context.Context, params apigen.StartTailorSessionParams) (*apigen.StartTailorSessionCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	rec, err := h.cvStore.Get(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	if rec.JobID == 0 {
		return nil, fiber.NewError(fiber.StatusConflict, "not a tailored CV")
	}
	base, ok, err := h.cvStore.BaseFor(ctx, userID, rec)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fiber.NewError(fiber.StatusConflict, "no base CV")
	}
	sessionID, err := h.startTailoringSession(ctx, userID, rec.ID, rec.JobID)
	if err != nil {
		return nil, err
	}
	// Same board placement as TailorCV: this path is how a ?cv= resume mints a missing
	// session, and without it the vacancy would stay off the Kanban.
	if h.jobs != nil {
		if err := h.jobs.EnsureOnBoard(ctx, userID, rec.JobID); err != nil {
			log.Printf("cv: placing vacancy on board after tailor session user=%d job=%d: %v", userID, rec.JobID, err)
		}
	}
	return &apigen.StartTailorSessionCreated{Data: apigen.CvTailorSession{
		TailorCvID: rec.ID.String(), BaseCvID: base.ID.String(), SessionID: sessionID,
	}}, nil
}

// startTailoringSession creates the conversation a tailoring workspace runs in: a
// tailor-preset session bound to the CV and its vacancy, stored on the CV so
// reopening the workspace resumes the same chat instead of starting over. The
// binding is what confines the CV tools — they close over these ids rather than
// taking them from the model — so it is created here, where ownership of both is
// already established.
func (h *cvHandlers) startTailoringSession(ctx context.Context, userID int64, cvID uuid.UUID, jobID int64) (string, error) {
	return h.startCVSession(ctx, userID, cvID, assistant.PresetTailor, &jobID)
}

// startCVSession creates the conversation bound to a CV (and to a vacancy when jobID is set) and
// stores its id on the CV, so reopening the workspace resumes the same chat.
func (h *cvHandlers) startCVSession(ctx context.Context, userID int64, cvID uuid.UUID, preset string, jobID *int64) (string, error) {
	if h.assistantSessions == nil {
		return "", fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	sess, err := h.assistantSessions.CreateSession(ctx, userID, preset, &cvID, jobID)
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
func (h *cvHandlers) PatchCV(ctx context.Context, req jx.Raw, params apigen.PatchCVParams) (*apigen.PatchCVOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}

	// Decode strictly: an unknown field or a type mismatch fails with a reason the caller can
	// act on, rather than being ignored and editing something else.
	var in struct {
		Ops  []cvedit.Op `json:"ops"`
		Note string      `json:"note"`
	}
	decoder := json.NewDecoder(bytes.NewReader(req))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "invalid operations: "+err.Error())
	}
	actor := cvedit.ActorCandidate
	if auth.ViaAPIKeyFromContext(ctx) {
		actor = cvedit.ActorAgent
	}
	meta, _, err := h.editor.Commit(ctx, id, userID, cvedit.Change{
		Actor:  actor,
		Origin: cvedit.OriginCLI,
		Note:   in.Note,
		Ops:    in.Ops,
	})
	if err != nil {
		return nil, mapCVError(err)
	}
	return &apigen.PatchCVOK{Data: cvMetaOf(meta)}, nil
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

// GetCVTailorContext serves the cached fit analysis for a tailored CV, projected to the tailoring
// reasoning context. Cookie or API key. 409 when the CV is not a tailored copy (no bound
// vacancy) or has no cached analysis; never calls the LLM.
func (h *cvHandlers) GetCVTailorContext(ctx context.Context, params apigen.GetCVTailorContextParams) (*apigen.GetCVTailorContextOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	rec, err := h.cvStore.Get(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	if rec.JobID == 0 {
		return nil, fiber.NewError(fiber.StatusConflict, "not a tailored CV")
	}
	job, err := h.jobReader.GetJob(ctx, rec.JobID)
	if err != nil {
		return nil, err
	}
	// fitanalysis.ErrNoAnalysis renders as the 409 this endpoint documents; classify maps it
	// once, at the port boundary, for every route that can meet it.
	tctx, err := h.fit.TailoringContext(ctx, userID, job, postingRequirements(job))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(tctx)
	if err != nil {
		return nil, err
	}
	return &apigen.GetCVTailorContextOK{Data: raw}, nil
}

// optionalAnalysis loads the cached fit analysis for (user, job) if one exists, returning nil
// when none is cached yet (or the cached blob is unreadable) rather than refusing — the
// tailoring bootstrap no longer requires one to exist before starting.
func (h *cvHandlers) optionalAnalysis(ctx context.Context, userID, jobID int64) *matchanalysis.Analysis {
	analysis, _ := h.fit.Optional(ctx, userID, jobID)
	return analysis
}

// postingRequirements is what a posting asks for, in its own words: the model's enrichment
// list, or the markup parser's derived column when the model stated none.
//
// It reads through jobview rather than off the row because those two producers are
// reconciled in exactly ONE place — jobview.FromDomain — and a second copy of that rule
// here is the drift its own documentation warns about. It lives in the handler rather than
// beside the reader that wants it (fitanalysis, in the candidate block) because candidate
// sits BELOW job in the layering and may not import jobview at all.
//
// A row the projection cannot read yields no requirements rather than an error: the
// tailoring context is worth serving without them, and refusing it over one unreadable
// enrichment blob would trade a complete answer for none. Logged, though — the only way
// here is a corrupt blob, and that must not look to us like a posting that stated nothing
// (the same distinction fitanalysis.Optional draws for a failed analysis read).
func postingRequirements(job db.Job) []enrich.Requirement {
	view, err := jobview.FromRow(job)
	if err != nil {
		log.Printf("handler: requirements for job %d: %v", job.ID, err)
		return nil
	}
	return view.Enrichment.Requirements
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
