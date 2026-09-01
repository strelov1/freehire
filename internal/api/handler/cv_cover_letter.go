package handler

import (
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// coverLetterResponse is the wire shape for a stored draft. Present is false — with no letter
// — when the pair has never been drafted, so the workspace renders an empty state rather than
// an error. Stale rides alongside rather than hiding the letter: a letter written by a retired
// model is still the letter the candidate may already have sent.
type coverLetterResponse struct {
	Present bool                `json:"present"`
	Stale   bool                `json:"stale,omitempty"`
	Letter  *coverletter.Letter `json:"letter,omitempty"`
	Model   string              `json:"model,omitempty"`
}

// GetCVCoverLetter serves the stored cover letter for the vacancy a tailored CV was written
// for. It NEVER calls a model: an absent draft is reported as absent, and only POST drafts one.
// That split is why this endpoint consumes no allowance at all.
//
// Cookie or API key, owner-scoped (a foreign id is the same 404 as a missing one).
func (h *cvHandlers) GetCVCoverLetter(c *fiber.Ctx) error {
	if h.letters == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTarget(c)
	if err != nil {
		return err
	}
	stored, err := h.letters.Get(c.Context(), userID, jobID)
	if err != nil {
		return err
	}
	if stored == nil {
		return c.JSON(fiber.Map{"data": coverLetterResponse{}})
	}
	// Staleness is measured against the vacancy's language, not the caller's profile — the
	// employer reads this letter. Reading the job costs one row we have already loaded.
	job, err := h.jobReader.GetJob(c.Context(), jobID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": coverLetterResponse{
		Present: true,
		Stale:   stored.Stale(h.letterModelID(), job.PostingLanguage),
		Letter:  &stored.Letter,
		Model:   stored.Model,
	}})
}

// DraftCVCoverLetter runs the three-stage chain and replaces the stored draft.
//
// The allowance is taken BEFORE the chain runs and released when the chain produces nothing
// usable, so a failed gateway does not cost the candidate a draft. The model calls go out on
// the candidate's own gateway credential under the cover-letter tag.
func (h *cvHandlers) DraftCVCoverLetter(c *fiber.Ctx) error {
	if h.letters == nil || h.bank == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTarget(c)
	if err != nil {
		return err
	}
	job, err := h.jobReader.GetJob(c.Context(), jobID)
	if err != nil {
		return err
	}
	// Required produces an analysis when none is cached, exactly as the assistant's
	// interview_context tool and the autopilot's run plan do, and is not charged for it. A
	// candidate who asks for a letter on a vacancy they never analysed gets one.
	tailoring, err := h.fit.TailoringContext(c.Context(), userID, job)
	if err != nil {
		return err
	}
	atoms, err := coverletter.Gather(c.Context(), h.bank, userID, tailoring.MissingHave)
	if err != nil {
		return err
	}
	candidate, ok := reviewableResume(h.resume, c, userID)
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "upload a CV first: a letter is written from your own experience")
	}

	// The stored draft's own timestamp identifies this attempt. A retry of the same request
	// computes the same reference and takes nothing more; a redraft happens after a successful
	// save moved that timestamp, so it computes a new one and pays again — which is right,
	// because a redraft is a second set of model calls.
	stored, err := h.letters.Get(c.Context(), userID, jobID)
	if err != nil {
		return err
	}
	attempt := "first"
	if stored != nil {
		attempt = stored.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}

	charge, refused, err := h.chargeCoverLetter(c, userID, jobID, attempt)
	if refused || err != nil {
		return err
	}

	analyzer := h.letterChain.As(h.llm.bind(c.Context(), userID, llm.Feature(tagCoverLetter)))
	letter, err := analyzer.Draft(c.Context(), coverletter.Input{
		Context:         tailoring,
		Candidate:       candidate,
		Atoms:           atoms,
		Band:            coverLetterBand(c),
		PostingLanguage: job.PostingLanguage,
	})
	switch {
	case errors.Is(err, coverletter.ErrNoPublishableEvidence):
		h.releaseCoverLetter(c, userID, charge)
		return fiber.NewError(fiber.StatusConflict,
			"nothing in your experience bank is yours to cite yet: confirm an achievement first")
	case err != nil:
		h.releaseCoverLetter(c, userID, charge)
		log.Printf("coverletter: drafting for user %d job %d: %v", userID, jobID, err)
		return fiber.NewError(fiber.StatusBadGateway, "the letter could not be drafted")
	case letter == nil:
		// The LLM is unconfigured. Nothing was spent and nothing was written.
		h.releaseCoverLetter(c, userID, charge)
		return fiber.NewError(fiber.StatusServiceUnavailable, "drafting is unavailable on this deployment")
	}

	if err := h.letters.Save(c.Context(), userID, jobID, *letter, h.letterModelID()); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": coverLetterResponse{
		Present: true,
		Letter:  letter,
		Model:   h.letterModelID(),
	}})
}

// coverLetterTarget resolves the caller and the vacancy their CV is bound to. A base CV has no
// vacancy to write to, and a tailored copy whose vacancy was pruned has lost it — the caller
// cannot act on the wrong one, so they are separate messages.
func (h *cvHandlers) coverLetterTarget(c *fiber.Ctx) (int64, int64, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return 0, 0, err
	}
	id, err := cvPathID(c)
	if err != nil {
		return 0, 0, err
	}
	rec, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return 0, 0, mapCVError(err)
	}
	if !rec.IsTailored {
		return 0, 0, fiber.NewError(fiber.StatusConflict, "this is a base CV: it is not bound to a vacancy")
	}
	if rec.JobID == 0 {
		return 0, 0, fiber.NewError(fiber.StatusConflict, "the vacancy this CV was tailored for no longer exists")
	}
	return userID, rec.JobID, nil
}

// coverLetterBand reads the requested length. An unrecognised value takes the standard band:
// the bands are a product decision rather than a measured limit, so a typo asking for brevity
// is better served with a normal letter than with a refusal.
func coverLetterBand(c *fiber.Ctx) coverletter.Band {
	if c.Query("band") == string(coverletter.BandShort) {
		return coverletter.BandShort
	}
	return coverletter.BandStandard
}

// chargeCoverLetter takes one allowance for drafting against this vacancy.
//
// The reference is per (job, attempt-of-the-stored-draft) rather than per job alone: a redraft
// is a second set of model calls and must cost a second allowance, while a retried request for
// the same attempt must not. A deployment with no meter charges nothing and runs, the
// fail-open rule the assistant's metering already follows.
func (h *cvHandlers) chargeCoverLetter(c *fiber.Ctx, userID, jobID int64, attempt string) (string, bool, error) {
	if h.plans == nil {
		return "", false, nil
	}
	ref := coverLetterRef(jobID, attempt)
	d, err := h.plans.Consume(c.Context(), userID, plan.FeatureCoverLetter, ref)
	switch {
	case err == nil && d.Charge == 0:
		return "", false, nil // already paid for under this reference
	case err == nil:
		return ref, false, nil
	case isRefusal(err):
		return "", true, refuse(c, d)
	default:
		log.Printf("plan: charging a cover letter for user %d: %v", userID, err)
		return "", false, nil
	}
}

// coverLetterRef names one drafting attempt in the usage ledger. The ledger's uniqueness index
// is on (user_id, feature, ref) for a consume, so this string is what makes a retry idempotent.
func coverLetterRef(jobID int64, attempt string) string {
	return "cover-letter#" + strconv.FormatInt(jobID, 10) + "#" + attempt
}

// letterModelID names the model a draft is stamped with. Empty on a deployment with no
// gateway, which Stale then reads as "matches", because a letter cannot be stale against a
// model that does not exist.
func (h *cvHandlers) letterModelID() string {
	if h.llm.client == nil {
		return ""
	}
	return h.llm.client.ModelID()
}

// releaseCoverLetter gives back what a draft took when it produced nothing the candidate can
// use. Safe to call blind: an empty reference releases nothing.
func (h *cvHandlers) releaseCoverLetter(c *fiber.Ctx, userID int64, ref string) {
	if h.plans == nil || ref == "" {
		return
	}
	if err := h.plans.Release(c.Context(), userID, plan.FeatureCoverLetter, ref); err != nil {
		log.Printf("plan: releasing a cover letter for user %d: %v", userID, err)
	}
}
