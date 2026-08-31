package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
)

// PostAssistantExtend buys a tailoring session another ceiling's worth of turns, spending
// one of the day's tailoring sessions to do it.
//
// It is a separate decision from starting a session, and deliberately an explicit one: the
// candidate has seen what the session produced so far and is choosing to spend more of
// today on this vacancy rather than on another. Extending silently when the ceiling was
// reached would spend their day for them.
//
// Extending is idempotent under a double click. Two simultaneous requests read the same
// ceiling count and attempt the same next reference; the ledger's index admits one and
// reports the other as already charged, so the race costs one session, not two.
func (h *assistantHandlers) PostAssistantExtend(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := assistantSessionID(c)
	if err != nil {
		return err
	}
	sess, err := h.store.Session(c.Context(), id, userID)
	if err != nil {
		return mapAssistantError(err)
	}
	// Only a tailoring session has a ceiling to extend. Every other preset is bounded by
	// the daily assistant allowance, which nothing can top up — a day is a day.
	if sess.Preset != assistant.PresetTailor {
		return fiber.NewError(fiber.StatusConflict, "only a CV editing session can be extended")
	}
	if h.plans == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}

	d, err := h.plans.ExtendSession(c.Context(), userID, id.String())
	if isRefusal(err) {
		return refuse(c, d)
	}
	if err != nil {
		return err
	}

	turns, err := h.queries.CountAssistantUserTurns(c.Context(), id)
	if err != nil {
		return err
	}
	td, err := h.plans.AllowTurn(c.Context(), userID, id.String(), int(turns))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"turns":     td.Turns,
		"ceiling":   td.Ceiling,
		"unlimited": td.Unlimited,
		"allowance": viewDecision(plan.Decision{
			Tier: d.Tier, Feature: plan.FeatureTailor, Used: d.Used,
			Limit: d.Limit, Unlimited: d.Unlimited, ResetsAt: d.ResetsAt,
		}),
	}})
}
