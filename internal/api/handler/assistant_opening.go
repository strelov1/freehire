package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// The opening briefs. Short on method: they only say which conversation this is, since how to
// open (read the context, name the vacancy, ask the first question) is in each preset's system
// prompt.
//
// They exist because a turn does not begin until a message arrives and the candidate has nothing
// to type; asking them to introduce their own interview would be the questionnaire the assistant
// is meant to save them.
const (
	rehearsalOpeningBrief = "Let's rehearse this interview. Read the context and tell me what you see, " +
		"then ask me which round to run."
	debriefOpeningBrief = "I've just had this interview. Read the context, then ask me what I was asked."
)

// openingBriefFor returns the brief an application-bound preset opens under, "" for a preset
// that opens under none. PostAssistantOpening refuses on "", so the set of presets that can speak
// first is stated once.
func openingBriefFor(preset string) string {
	switch preset {
	case assistant.PresetInterview:
		return rehearsalOpeningBrief
	case assistant.PresetDebrief:
		return debriefOpeningBrief
	default:
		return ""
	}
}

// PostAssistantOpening speaks first in a rehearsal or a debrief, streaming one ordinary turn
// under a server-side brief.
//
// It refuses a session whose opening was already answered: a reload that re-ran it would restart
// the interview over what the candidate already said.
func (h *assistantHandlers) PostAssistantOpening(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.runner == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// The vacancy as well as the preset: the context tool is registered only for a session that
	// carries one, so an unbound session would open on a tool it does not have.
	brief := openingBriefFor(sess.Preset)
	if brief == "" || sess.JobID == nil {
		return fiber.NewError(fiber.StatusConflict, "this conversation does not open by itself")
	}
	transcript, err := h.store.Transcript(c.Context(), sess.ID)
	if err != nil {
		return err
	}
	// An ANSWERED opening, not any transcript: the runner records the brief before calling the
	// model, so a turn that dies upstream leaves one message behind. Refusing on that would strand
	// the session with a line the candidate did not write and no way to retry.
	for _, m := range transcript {
		if m.Role == assistant.RoleAssistant {
			return fiber.NewError(fiber.StatusConflict, "this conversation has already started")
		}
	}
	return h.streamTurn(c, sess, brief, nil, assistant.TurnConfig{})
}
