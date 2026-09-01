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
	// employer reads this letter.
	job, err := h.jobReader.GetJob(c.Context(), jobID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": coverLetterResponse{
		Present: true,
		Stale:   stored.Stale(modelIDOf(h.llm.client), job.PostingLanguage),
		Letter:  &stored.Letter,
		Model:   stored.Model,
	}})
}

// DraftCVCoverLetter runs the three-stage chain and replaces the stored draft.
//
// The allowance is taken BEFORE the chain and released when the chain produces nothing usable,
// so a failed gateway does not cost the candidate a draft. The model calls go out on the
// candidate's own gateway credential under the cover-letter tag.
func (h *cvHandlers) DraftCVCoverLetter(c *fiber.Ctx) error {
	drafter := h.letterDrafter()
	if !drafter.ready() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTarget(c)
	if err != nil {
		return err
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

	client := h.llm.bind(c.Context(), userID, llm.Feature(tagCoverLetter))
	letter, err := drafter.draft(c.Context(), client, userID, jobID, coverLetterBand(c))
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
	return c.JSON(fiber.Map{"data": coverLetterResponse{
		Present: true,
		Letter:  letter,
		Model:   modelIDOf(client),
	}})
}

// letterDrafter assembles the shared drafting path from this surface's dependencies.
func (h *cvHandlers) letterDrafter() letterDrafter {
	return letterDrafter{
		jobs: h.jobReader, fit: h.fit, bank: h.bank, profile: h.letterProfile,
		resume: h.resume, chain: h.letterChain, letters: h.letters,
	}
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

// chargeCoverLetter takes one allowance for drafting against this vacancy. A deployment with
// no meter charges nothing and runs — the fail-open rule the assistant's metering follows.
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
