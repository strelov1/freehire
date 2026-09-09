package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/answerbank"
)

// answerBankHandlers serves the candidate's own bank of screening answers.
type answerBankHandlers struct{ bank *answerbank.Store }

// register mounts the bank's three routes. mw.key, matching every other /me route that a
// client other than the web app may reasonably call: the CLI (strelov1/freehire-cli) reaches
// these with an API key.
func (h *answerBankHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/answer-bank", mw.key, h.ListAnswers)
	api.Put("/me/answer-bank", mw.key, h.SaveAnswer)
	api.Delete("/me/answer-bank/:id", mw.key, h.DeleteAnswer)
}

// bankedAnswerResponse is one answer on the wire. provenance rides along so a surface can
// show which answers are the candidate's own — the store's List returns every provenance
// deliberately.
//
// UpdatedAt is a time.Time, not a pre-formatted string: encoding/json marshals it as RFC3339
// with whatever sub-second precision it carries, which is both correct and free. Formatting
// it by hand re-implemented the standard and dropped the fraction while doing so.
type bankedAnswerResponse struct {
	ID         int64     `json:"id"`
	Topic      string    `json:"topic"`
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	Provenance string    `json:"provenance"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListAnswers returns the caller's whole bank, newest first.
func (h *answerBankHandlers) ListAnswers(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	answers, err := h.bank.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]bankedAnswerResponse, 0, len(answers))
	for _, a := range answers {
		out = append(out, bankedAnswerResponse{
			ID: a.ID, Topic: a.Topic, Question: a.Question, Answer: a.Answer,
			Provenance: a.Provenance, UpdatedAt: a.UpdatedAt.UTC(),
		})
	}
	return c.JSON(fiber.Map{"data": out})
}

// saveAnswerRequest is what the review screen and the CLI both send.
type saveAnswerRequest struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// SaveAnswer records the caller's answer to one question.
//
// The provenance is AuthorCandidate because of WHERE this ran — a request the candidate
// themselves authenticated — never because a body said so. There is no field for it in
// saveAnswerRequest, and that absence is the enforcement.
func (h *answerBankHandlers) SaveAnswer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in saveAnswerRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	err = h.bank.Save(c.Context(), userID, in.Question, in.Answer, answerbank.AuthorCandidate)
	switch {
	case errors.Is(err, answerbank.ErrEmptyAnswer):
		return fiber.NewError(fiber.StatusBadRequest, "the answer is empty")
	case errors.Is(err, answerbank.ErrUnkeyable):
		return fiber.NewError(fiber.StatusBadRequest, "this question cannot be saved — it has no readable text")
	case errors.Is(err, answerbank.ErrTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "this answer is too long for a screening question")
	case err != nil:
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"saved": true}})
}

// DeleteAnswer removes one of the caller's own answers. Nothing else ever removes from the
// bank.
func (h *answerBankHandlers) DeleteAnswer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid answer id")
	}
	if err := h.bank.Delete(c.Context(), userID, id); err != nil {
		if errors.Is(err, answerbank.ErrNotFound) {
			// 404, never 403: a foreign id and a missing one look alike, so a probing
			// caller learns nothing about what another candidate holds.
			return fiber.NewError(fiber.StatusNotFound, "no such answer")
		}
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"deleted": true}})
}
