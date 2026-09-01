package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

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
	// Cited is the letter's evidence with the claims already resolved, so the surface never
	// has to look an id up — and so it cannot forget to.
	Cited []citedAtom `json:"cited,omitempty"`
	Model string      `json:"model,omitempty"`
}

// GetCVCoverLetter serves the stored cover letter for the vacancy a tailored CV was written
// for. It NEVER calls a model: an absent draft is reported as absent, and only POST drafts one.
// That split is why this endpoint consumes no allowance at all.
//
// Cookie or API key, owner-scoped (a foreign id is the same 404 as a missing one).
func (h *cvHandlers) GetCVCoverLetter(c *fiber.Ctx) error {
	if h.letter.letters == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTarget(c)
	if err != nil {
		return err
	}
	stored, err := h.letter.letters.Get(c.Context(), userID, jobID)
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
		Cited:   citedAtomsOf(c.Context(), h.letter.bank, userID, stored.Cited),
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

	attempt := letterAttempt(c.Context(), h.letter.letters, userID, jobID)
	charge, refused, decision := chargeLetter(c.Context(), h.plans, userID, jobID, attempt)
	if refused {
		return refuse(c, decision)
	}

	client := h.llm.bind(c.Context(), userID, llm.Feature(tagCoverLetter))
	letter, err := drafter.draft(c.Context(), client, userID, jobID, coverLetterBand(c))
	if err != nil || letter == nil {
		// Every failing path gives the charge back, so it is given back once here rather than
		// in each branch — a candidate must never pay for a letter they did not get.
		releaseLetterCharge(h.plans, userID, charge)
		switch {
		case errors.Is(err, coverletter.ErrNoPublishableEvidence):
			return fiber.NewError(fiber.StatusConflict, letterFailureMessage(err))
		case err != nil:
			// Handed to the shared mapper rather than flattened here. It already knows the
			// states this path meets — fitanalysis.ErrNoAnalysis is a 409 saying "run the fit
			// analysis first", and errors.go says outright that the status is decided there
			// rather than at each call site. Flattening every failure into 502 turned that
			// answer into a Bad Gateway on production, on the majority of vacancies.
			return err
		default:
			// The LLM is unconfigured. Nothing was spent and nothing was written.
			return fiber.NewError(fiber.StatusServiceUnavailable, letterFailureMessage(nil))
		}
	}
	return c.JSON(fiber.Map{"data": coverLetterResponse{
		Present: true,
		Letter:  letter,
		Cited:   citedAtomsOf(c.Context(), h.letter.bank, userID, letter.Cited),
		Model:   modelIDOf(client),
	}})
}

// letterDrafter assembles the shared drafting path from this surface's dependencies.
func (h *cvHandlers) letterDrafter() letterDrafter {
	return letterDrafter{
		jobs: h.jobReader, fit: h.fit, bank: h.letter.bank,
		resume: h.resume, chain: h.letter.chain, letters: h.letter.letters,
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
