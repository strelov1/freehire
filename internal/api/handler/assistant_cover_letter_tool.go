package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// coverLetterDraftTool lets a tailoring session write the cover letter the vacancy's own
// application form asks for. It runs the same letterDrafter and stores under the same key as
// POST /me/cvs/:id/cover-letter, so chat and button cannot produce different letters for one pair.
// The vacancy is closed over, not an argument: the model cannot address a different vacancy.
func (h *assistantHandlers) coverLetterDraftTool(jobID int64) assistant.Tool {
	return assistant.Tool{
		Name: "cover_letter_draft",
		Description: "Write the cover letter for the vacancy this CV is being tailored to, from the " +
			"candidate's confirmed achievements and any saved evidence selection. The candidate should review " +
			"the result before sending. Replaces any letter already drafted for this vacancy. " +
			"Optional \"band\": \"short\" or \"standard\" (default).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"band": map[string]any{
					"type":        "string",
					"enum":        []string{string(coverletter.BandShort), string(coverletter.BandStandard)},
					"description": "How long the letter should be. Defaults to standard.",
				},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Band string `json:"band"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			// A tool error is a sentence the model can act on; a nil dereference would panic inside the SSE
			// writer's goroutine, out of Registry.Call's error path. Same guard as cv_context's.
			drafter := h.letterDrafter()
			if !drafter.ready() {
				return nil, errors.New("drafting a cover letter is unavailable in this deployment")
			}

			prepared, err := drafter.prepare(ctx, userID, jobID)
			if err != nil {
				return nil, err
			}
			attempt := letterAttempt(ctx, h.letter.letters, userID, jobID)
			charge, refused, _ := chargeLetter(ctx, h.plans, userID, jobID, attempt)
			if refused {
				// A refusal reaches the model as a sentence it can relay, not as a 402: the
				// turn itself was already paid for and must still end with an answer.
				return nil, errors.New("the candidate has used today's cover-letter allowance; " +
					"tell them it resets tomorrow, or that Pro lifts the limit")
			}
			caller, err := h.binding().caller(ctx, userID, llm.Feature(tagCoverLetter))
			if err != nil {
				releaseLetterCharge(h.plans, userID, charge)
				return nil, errors.New(letterFailureMessage(err))
			}
			letter, err := drafter.draft(ctx, caller, userID, jobID, toolBand(in.Band), prepared)
			if err != nil || letter == nil {
				// Every failing path gives the charge back, so it is given back once here.
				releaseLetterCharge(h.plans, userID, charge)
				// The same sentence the other two paths render; the raw error ("fitanalysis: no analysis has been
				// run for this job") is useless for the model to relay to a candidate.
				return nil, errors.New(letterFailureMessage(err))
			}
			return letter, nil
		},
	}
}

// letterDrafter assembles the shared drafting path from this surface's dependencies.
func (h *assistantHandlers) letterDrafter() letterDrafter {
	return letterDrafter{
		jobs: h.jobs, fit: h.fit, bank: h.letter.bank,
		resume: h.resumeStore(), chain: h.letter.chain, letters: h.letter.letters,
	}
}

// toolBand reads the model's requested length. An unrecognised value takes the standard band rather
// than refusing: the bands are a product decision, and a round of budget spent correcting a typo
// teaches the model nothing.
func toolBand(s string) coverletter.Band {
	if s == string(coverletter.BandShort) {
		return coverletter.BandShort
	}
	return coverletter.BandStandard
}

// The chat path charges FeatureCoverLetter on top of the turn's FeatureAssistant, deliberately: the
// turn pays for the conversation, the letter for the three further model calls it triggers.

// resumeStore is the structured-résumé half of the candidate projection. Nil-safe: the
// composition degrades to the bank alone, which is what a candidate with no uploaded file has.
func (h *assistantHandlers) resumeStore() *resume.Store {
	if h.resume == nil {
		return nil
	}
	return h.resume.resume
}
