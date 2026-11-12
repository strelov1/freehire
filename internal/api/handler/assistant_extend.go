package handler

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
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
func (h *assistantHandlers) PostAssistantExtend(ctx context.Context, params apigen.PostAssistantExtendParams) (apigen.PostAssistantExtendRes, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseAssistantSessionID(params.ID)
	if err != nil {
		return nil, err
	}
	sess, err := h.store.Session(ctx, id, userID)
	if err != nil {
		return nil, mapAssistantError(err)
	}
	if sess.Preset == assistant.PresetSupport {
		return nil, errSessionNotFound
	}
	// Only a tailoring session has a ceiling to extend. Every other preset is bounded by
	// the daily assistant allowance, which nothing can top up — a day is a day.
	if sess.Preset != assistant.PresetTailor {
		return nil, fiber.NewError(fiber.StatusConflict, "only a CV editing session can be extended")
	}
	if h.plans == nil {
		return &apigen.PostAssistantExtendNoContent{}, nil
	}

	d, err := h.plans.ExtendSession(ctx, userID, id.String())
	if isRefusal(err) {
		return decisionRefusal(d).assistant()
	}
	if err != nil {
		return nil, err
	}

	turns, err := h.queries.CountAssistantUserTurns(ctx, id)
	if err != nil {
		return nil, err
	}
	td, err := h.plans.AllowTurn(ctx, userID, id.String(), int(turns))
	if err != nil {
		return nil, err
	}
	allowance, err := json.Marshal(viewDecision(plan.Decision{
		Tier: d.Tier, Feature: plan.FeatureTailor, Used: d.Used,
		Limit: d.Limit, Unlimited: d.Unlimited, ResetsAt: d.ResetsAt,
	}))
	if err != nil {
		return nil, err
	}
	return &apigen.PostAssistantExtendOK{Data: apigen.AssistantExtension{
		Turns: int64(td.Turns), Ceiling: int64(td.Ceiling), Unlimited: td.Unlimited, Allowance: allowance,
	}}, nil
}

// assistant is the refusal as the assistant contract answers it: the body write402 sends,
// with the allowance as the JSON its view marshals to.
func (r planRefusal) assistant() (*apigen.AssistantPlanRefusal, error) {
	allowance, err := json.Marshal(r.allowance)
	if err != nil {
		return nil, err
	}
	out := &apigen.AssistantPlanRefusal{Error: r.message, Allowance: allowance}
	if r.offerUpgrade {
		out.UpgradeURL = apigen.NewOptString(upgradePath)
	}
	return out, nil
}
