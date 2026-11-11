package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
)

// answerBankHandlers serves the candidate's own bank of screening answers.
type answerBankHandlers struct{ bank *answerbank.Store }

// register mounts the bank's three routes. mw.key, matching every other /me route that a
// client other than the web app may reasonably call: the CLI (strelov1/freehire-cli) reaches
// these with an API key.
func (h *answerBankHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/answer-bank", mw.key, contract)
	api.Put("/me/answer-bank", mw.key, contract)
	api.Delete("/me/answer-bank/:id", mw.key, contract)
}

// ListBankedAnswers returns the caller's whole bank, newest first.
func (h *answerBankHandlers) ListBankedAnswers(ctx context.Context) (*apigen.ListBankedAnswersOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	answers, err := h.bank.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.ListBankedAnswersOK{Data: bankedAnswersOut(answers)}, nil
}

// bankedAnswersOut renders the bank for the HTTP route. The assistant and MCP tools keep
// toBankedAnswerResponses below: the MCP SDK checks a tool's answer against its Go type, and
// a generated type with its own MarshalJSON is exactly what that check has refused before.
func bankedAnswersOut(answers []answerbank.Answer) []apigen.BankedAnswer {
	out := make([]apigen.BankedAnswer, 0, len(answers))
	for _, a := range answers {
		out = append(out, apigen.BankedAnswer{
			ID: a.ID, Topic: a.Topic, Question: a.Question, Answer: a.Answer,
			Provenance: a.Provenance, UpdatedAt: a.UpdatedAt.UTC(),
		})
	}
	return out
}

// SaveBankedAnswer records the caller's answer to one question.
//
// The provenance is AuthorCandidate because of WHERE this ran — a request the candidate
// themselves authenticated — never because a body said so. There is no field for it in the
// request (apigen.SaveBankedAnswer, saveAnswerRequest for the tool), and that absence is the
// enforcement.
func (h *answerBankHandlers) SaveBankedAnswer(ctx context.Context, req *apigen.SaveBankedAnswer) (*apigen.SaveBankedAnswerOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := bankSaveError(h.bank.Save(ctx, userID, req.Question.Value, req.Answer.Value, answerbank.AuthorCandidate)); err != nil {
		return nil, err
	}
	return &apigen.SaveBankedAnswerOK{Data: apigen.Saved{Saved: true}}, nil
}

// bankSaveError names a refused save for the caller; the route and the MCP tool share it.
func bankSaveError(err error) error {
	switch {
	case errors.Is(err, answerbank.ErrEmptyAnswer):
		return fiber.NewError(fiber.StatusBadRequest, "the answer is empty")
	case errors.Is(err, answerbank.ErrUnkeyable):
		return fiber.NewError(fiber.StatusBadRequest, "this question cannot be saved — it has no readable text")
	case errors.Is(err, answerbank.ErrTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "this answer is too long for a screening question")
	}
	return err
}

// DeleteBankedAnswer removes one of the caller's own answers. Nothing else ever removes from the
// bank.
func (h *answerBankHandlers) DeleteBankedAnswer(ctx context.Context, params apigen.DeleteBankedAnswerParams) (*apigen.DeleteBankedAnswerOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(params.ID, 10, 64)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid answer id")
	}
	if err := h.bank.Delete(ctx, userID, id); err != nil {
		if errors.Is(err, answerbank.ErrNotFound) {
			return nil, fiber.NewError(fiber.StatusNotFound, "no such answer")
		}
		return nil, err
	}
	return &apigen.DeleteBankedAnswerOK{Data: apigen.Deleted{Deleted: true}}, nil
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

// toBankedAnswerResponses renders the bank for the wire; the route and the MCP tool share it.
func toBankedAnswerResponses(answers []answerbank.Answer) []bankedAnswerResponse {
	out := make([]bankedAnswerResponse, 0, len(answers))
	for _, a := range answers {
		out = append(out, bankedAnswerResponse{
			ID: a.ID, Topic: a.Topic, Question: a.Question, Answer: a.Answer,
			Provenance: a.Provenance, UpdatedAt: a.UpdatedAt.UTC(),
		})
	}
	return out
}

// saveAnswerRequest is what the review screen and the CLI both send.
type saveAnswerRequest struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
