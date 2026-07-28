package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/userprofile"
)

// noProfileResult is what get_profile answers when the caller has never saved a
// profile. It is a result, not an error: the agent has learned something true and
// should act on it. The instruction matters as much as the fact — collecting the
// same preferences in chat would produce a better answer this once and nothing
// durable, where the profile page's values persist and drive the ranked feed,
// subscriptions and fit analysis.
type noProfileResult struct {
	Profile any    `json:"profile"`
	Next    string `json:"next"`
}

// getProfileTool reads the caller's own saved profile: the specializations, skills
// and location preferences they curated, plus their structured CV with the contact
// fields left out.
//
// This is the tool that stops the agent interrogating a user for what they have
// already told the product. Registered for every session, and worth calling before
// the first search of a conversation.
func (h *assistantHandlers) getProfileTool() assistant.Tool {
	return assistant.Tool{
		Name: "get_profile",
		Description: "Read the user's saved job-search profile: the roles and skills they " +
			"want, the skills they want to avoid, where and how they will work, and their CV " +
			"(headline, years, experience, education — contacts are not included). Call this " +
			"before asking the user what they are looking for; only ask about what the profile " +
			"does not answer. Takes no arguments.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			profile, err := h.profile.userProfile.Get(ctx, userID)
			if errors.Is(err, userprofile.ErrNotFound) {
				return noProfileResult{
					Next: "The user has not saved a profile yet. Point them at /my/profile to " +
						"fill it in — it persists and drives their recommendations — rather than " +
						"collecting the same answers in this conversation.",
				}, nil
			}
			if err != nil {
				return nil, err
			}
			return toProfileResponse(profile, h.profile.structuredCV(ctx, userID)), nil
		},
	}
}
