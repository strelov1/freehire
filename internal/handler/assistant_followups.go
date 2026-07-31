package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/assistant"
)

// PostAssistantFollowUps suggests up to three questions to ask next, drawn from the
// conversation's most recent exchange.
//
// Two things are deliberately NOT errors here. A conversation with nothing to follow
// up on — no answer yet, or an answer that was all tool calls — is an empty list. So
// is every failure: an unconfigured model, a model that erred, an answer we could not
// parse. The strip is decoration, and a decoration that reports a problem the caller
// cannot act on is worse than one that quietly does not appear. The failure is logged
// instead, because "the strip never shows up" is otherwise indistinguishable from "the
// model had nothing to suggest".
//
// A POST rather than a GET despite reading nothing: it spends a model call, and a GET
// is the one method every prefetcher, crawler and browser feels free to make twice.
func (h *assistantHandlers) PostAssistantFollowUps(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.followUps == nil {
		return c.JSON(fiber.Map{"data": fiber.Map{"followups": []string{}}})
	}
	messages, err := h.store.Transcript(c.Context(), sess.ID)
	if err != nil {
		return err
	}
	exchange, ok := assistant.LastExchange(messages)
	if !ok {
		return c.JSON(fiber.Map{"data": fiber.Map{"followups": []string{}}})
	}
	suggestions, err := h.followUps.Suggest(c.Context(), exchange)
	if err != nil {
		log.Printf("assistant: follow-ups for session %s: %v", sess.ID, err)
		suggestions = nil
	}
	if suggestions == nil {
		suggestions = []string{}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"followups": suggestions}})
}
