package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/screeninganswers"
)

// screeningAnswersHandlers serves the caller's own screening answers — the six
// candidate-stated facts (authorized countries, visa sponsorship, desired salary, notice
// period, willingness to relocate, 18+) that repeat across ATS application forms and no CV
// can supply. The use case lives in screeninganswers.Store; the handlers translate wire ↔
// domain and delegate to it.
type screeningAnswersHandlers struct {
	store *screeninganswers.Store
}

func newScreeningAnswersHandlers(store *screeninganswers.Store) *screeningAnswersHandlers {
	return &screeningAnswersHandlers{store: store}
}

func (h *screeningAnswersHandlers) register(api fiber.Router, mw middleware) {
	// The screening-answers record is a singleton — one per user, keyed by the
	// authenticated caller, no id in the path. GET returns the record or null; PUT
	// partially updates it (a field the body omits keeps its stored value). Same auth
	// split as /me/profile: the read takes a key, the write is cookie-only.
	api.Get("/me/screening-answers", mw.key, h.GetScreeningAnswers)
	api.Put("/me/screening-answers", mw.cookie, h.PutScreeningAnswers)
}

// GetScreeningAnswers returns the authenticated user's screening answers, or
// {"data": null} when they have stated none yet.
func (h *screeningAnswersHandlers) GetScreeningAnswers(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	answers, err := h.store.Get(c.Context(), userID)
	if errors.Is(err, screeninganswers.ErrNotFound) {
		return c.JSON(fiber.Map{"data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": answers})
}

// PutScreeningAnswers partially updates the authenticated user's screening answers: a
// field the request body carries is validated and stored; a field it omits keeps its
// stored value. A malformed field (an unrecognized country code, a currency that is not a
// three-letter ISO 4217 code, a period outside the vocabulary, a non-positive salary, a
// negative notice period) is a 400 naming the invalid value. Cookie-only.
func (h *screeningAnswersHandlers) PutScreeningAnswers(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in screeninganswers.Answers
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	answers, err := h.store.Update(c.Context(), userID, in)
	if err != nil {
		var ve *screeninganswers.ValidationError
		if errors.As(err, &ve) {
			return fiber.NewError(fiber.StatusBadRequest, ve.Error())
		}
		return err
	}
	return c.JSON(fiber.Map{"data": answers})
}
