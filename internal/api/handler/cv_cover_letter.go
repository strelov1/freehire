package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

// contract is the draft as the contract answers it. The letter and its citations are the
// cover-letter package's own shapes (the stream sends the same ones), so they travel as the
// JSON they marshal to, and stay absent when the Go field was omitted.
func (r coverLetterResponse) contract() (apigen.CvCoverLetter, error) {
	out := apigen.CvCoverLetter{Present: r.Present, Model: optString(r.Model)}
	if r.Stale {
		out.Stale = apigen.NewOptBool(true)
	}
	var err error
	if r.Letter != nil {
		if out.Letter, err = json.Marshal(r.Letter); err != nil {
			return apigen.CvCoverLetter{}, err
		}
	}
	if len(r.Cited) > 0 {
		if out.Cited, err = json.Marshal(r.Cited); err != nil {
			return apigen.CvCoverLetter{}, err
		}
	}
	return out, nil
}

// GetCVCoverLetter serves the stored cover letter for the vacancy a tailored CV was written
// for. It NEVER calls a model: an absent draft is reported as absent, and only POST drafts one.
// That split is why this endpoint consumes no allowance at all.
//
// Cookie or API key, owner-scoped (a foreign id is the same 404 as a missing one).
func (h *cvHandlers) GetCVCoverLetter(ctx context.Context, params apigen.GetCVCoverLetterParams) (*apigen.GetCVCoverLetterOK, error) {
	if h.letter.letters == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTargetOf(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	stored, err := h.letter.letters.Get(ctx, userID, jobID)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return &apigen.GetCVCoverLetterOK{Data: apigen.CvCoverLetter{}}, nil
	}
	// Staleness is measured against the vacancy's language, not the caller's profile — the
	// employer reads this letter.
	job, err := h.jobReader.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out, err := coverLetterResponse{
		Present: true,
		Stale:   stored.Stale(h.letter.chain.ModelID(), job.PostingLanguage),
		Letter:  &stored.Letter,
		Cited:   citedAtomsOf(ctx, h.letter.bank, userID, stored.Cited),
		Model:   stored.Model,
	}.contract()
	if err != nil {
		return nil, err
	}
	return &apigen.GetCVCoverLetterOK{Data: out}, nil
}

// DraftCVCoverLetter runs the three-stage chain and replaces the stored draft.
//
// The allowance is taken BEFORE the chain and released when the chain produces nothing usable,
// so a failed gateway does not cost the candidate a draft. The model calls go out on the
// candidate's own gateway credential under the cover-letter tag.
func (h *cvHandlers) DraftCVCoverLetter(ctx context.Context, params apigen.DraftCVCoverLetterParams) (apigen.DraftCVCoverLetterRes, error) {
	drafter := h.letterDrafter()
	if !drafter.ready() {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTargetOf(ctx, params.ID)
	if err != nil {
		return nil, err
	}

	attempt := letterAttempt(ctx, h.letter.letters, userID, jobID)
	charge, refused, decision := chargeLetter(ctx, h.plans, userID, jobID, attempt)
	if refused {
		return decisionRefusal(decision).cv()
	}

	caller, err := h.llm.caller(ctx, userID, llm.Feature(tagCoverLetter))
	if err != nil {
		releaseLetterCharge(h.plans, userID, charge)
		return nil, err
	}
	letter, err := drafter.draft(ctx, caller, userID, jobID, bandOf(params.Band.Value))
	if err != nil || letter == nil {
		// Every failing path gives the charge back, so it is given back once here rather than
		// in each branch — a candidate must never pay for a letter they did not get.
		releaseLetterCharge(h.plans, userID, charge)
		switch {
		case errors.Is(err, coverletter.ErrNoPublishableEvidence):
			return nil, fiber.NewError(fiber.StatusConflict, letterFailureMessage(err))
		case err != nil:
			// Handed to the shared mapper rather than flattened here. It already knows the
			// states this path meets — fitanalysis.ErrNoAnalysis is a 409 saying "run the fit
			// analysis first", and errors.go says outright that the status is decided there
			// rather than at each call site. Flattening every failure into 502 turned that
			// answer into a Bad Gateway on production, on the majority of vacancies.
			return nil, err
		default:
			// The LLM is unconfigured. Nothing was spent and nothing was written.
			return nil, fiber.NewError(fiber.StatusServiceUnavailable, letterFailureMessage(nil))
		}
	}
	out, err := coverLetterResponse{
		Present: true,
		Letter:  letter,
		Cited:   citedAtomsOf(ctx, h.letter.bank, userID, letter.Cited),
		Model:   h.letter.chain.ModelID(),
	}.contract()
	if err != nil {
		return nil, err
	}
	return &apigen.DraftCVCoverLetterOK{Data: out}, nil
}

// letterDrafter assembles the shared drafting path from this surface's dependencies.
func (h *cvHandlers) letterDrafter() letterDrafter {
	return letterDrafter{
		jobs: h.jobReader, fit: h.fit, bank: h.letter.bank,
		resume: h.resume, chain: h.letter.chain, letters: h.letter.letters,
	}
}

// coverLetterTarget resolves the caller and the vacancy their CV is bound to, for a Fiber
// handler (the stream).
func (h *cvHandlers) coverLetterTarget(c *fiber.Ctx) (int64, int64, error) {
	return h.coverLetterTargetOf(c.Context(), c.Params("id"))
}

// coverLetterTargetOf resolves the caller and the vacancy their CV is bound to. A base CV has
// no vacancy to write to, and a tailored copy whose vacancy was pruned has lost it — the
// caller cannot act on the wrong one, so they are separate messages.
func (h *cvHandlers) coverLetterTargetOf(ctx context.Context, rawID string) (int64, int64, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return 0, 0, err
	}
	id, err := cvID(rawID)
	if err != nil {
		return 0, 0, err
	}
	rec, err := h.cvStore.Get(ctx, id, userID)
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
	return bandOf(c.Query("band"))
}

func bandOf(requested string) coverletter.Band {
	if requested == string(coverletter.BandShort) {
		return coverletter.BandShort
	}
	return coverletter.BandStandard
}
