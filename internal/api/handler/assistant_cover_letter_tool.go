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
// application form asks for.
//
// It runs the SAME letterDrafter as POST /me/cvs/:id/cover-letter and stores under the same
// key, so the chat path and the button path cannot drift into producing different letters for
// one pair. What differs is only how the request arrives and how a refusal is phrased.
//
// The vacancy is closed over rather than taken as an argument, like every other tool in a
// tailoring session: the model has no way to address a different vacancy, not even by guessing
// an id.
func (h *assistantHandlers) coverLetterDraftTool(jobID int64) assistant.Tool {
	return assistant.Tool{
		Name: "cover_letter_draft",
		Description: "Write the cover letter for the vacancy this CV is being tailored to, from the " +
			"candidate's own banked achievements. Every claim about their experience comes from evidence they " +
			"asserted — nothing is invented. Replaces any letter already drafted for this vacancy. " +
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
			// A tool error is a sentence the model can act on; a nil dereference here is a
			// panic inside the SSE writer's goroutine, where Registry.Call's error path cannot
			// reach it. Same guard, same reason as cv_context's.
			drafter := h.letterDrafter()
			if !drafter.ready() {
				return nil, errors.New("drafting a cover letter is unavailable in this deployment")
			}

			attempt := letterAttempt(ctx, h.letter.letters, userID, jobID)
			charge, refused, _ := chargeLetter(ctx, h.plans, userID, jobID, attempt)
			if refused {
				// A refusal reaches the model as a sentence it can relay, not as a 402: the
				// turn itself was already paid for and must still end with an answer.
				return nil, errors.New("the candidate has used today's cover-letter allowance; " +
					"tell them it resets tomorrow, or that Pro lifts the limit")
			}
			client := userLLM(ctx, h.keys, h.llm, userID, llm.Feature(tagCoverLetter))
			letter, err := drafter.draft(ctx, client, userID, jobID, toolBand(in.Band))
			if err != nil || letter == nil {
				// Every failing path gives the charge back, so it is given back once here.
				releaseLetterCharge(h.plans, userID, charge)
				// The same sentence the other two paths render. Returning the raw error instead
				// made the model relay "fitanalysis: no analysis has been run for this job" —
				// true, and useless to relay to a candidate.
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

// toolBand reads the model's requested length. An unrecognised value takes the standard band
// rather than refusing: the bands are a product decision, not a measured limit, and spending a
// round of the turn's budget to correct a typo teaches the model nothing about the letter.
func toolBand(s string) coverletter.Band {
	if s == string(coverletter.BandShort) {
		return coverletter.BandShort
	}
	return coverletter.BandStandard
}

// The chat path charges FeatureCoverLetter on top of the turn's own FeatureAssistant,
// deliberately: the turn pays for the conversation, the letter pays for the three further
// model calls the conversation triggered. Metering it as a turn alone would price a letter
// written in chat at a fraction of the identical letter written from the button.

// resumeStore is the structured-résumé half of the candidate projection. Nil-safe: the
// composition degrades to the bank alone, which is what a candidate with no uploaded file has.
func (h *assistantHandlers) resumeStore() *resume.Store {
	if h.resume == nil {
		return nil
	}
	return h.resume.resume
}
