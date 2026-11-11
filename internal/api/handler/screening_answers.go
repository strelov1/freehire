package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
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

func (h *screeningAnswersHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The screening-answers record is a singleton — one per user, keyed by the
	// authenticated caller, no id in the path. GET returns the record or null; PUT
	// partially updates it (a field the body omits keeps its stored value). Same auth
	// split as /me/profile: the read takes a key, the write is cookie-only.
	api.Get("/me/screening-answers", mw.key, contract)
	api.Put("/me/screening-answers", mw.cookie, contract)
}

// GetScreeningAnswers returns the authenticated user's screening answers, or
// {"data": null} when they have stated none yet.
func (h *screeningAnswersHandlers) GetScreeningAnswers(ctx context.Context) (*apigen.GetScreeningAnswersOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	answers, err := h.store.Get(ctx, userID)
	if errors.Is(err, screeninganswers.ErrNotFound) {
		return &apigen.GetScreeningAnswersOK{Data: apigen.NilScreeningAnswers{Null: true}}, nil
	}
	if err != nil {
		return nil, err
	}
	return &apigen.GetScreeningAnswersOK{Data: apigen.NewNilScreeningAnswers(screeningAnswersOut(answers))}, nil
}

// UpdateScreeningAnswers partially updates the authenticated user's screening answers: a
// field the request body carries is validated and stored; a field it omits keeps its
// stored value; a field named in "clear" is withdrawn back to unstated. A malformed field
// (an unrecognized country code, a currency that is not a three-letter ISO 4217 code, a
// period outside the vocabulary, a non-positive salary, a negative notice period), an
// unknown name in "clear", or a field both stated and cleared is a 400 naming it.
// Cookie-only.
func (h *screeningAnswersHandlers) UpdateScreeningAnswers(ctx context.Context, req *apigen.ScreeningAnswersUpdate) (*apigen.UpdateScreeningAnswersOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	toClear, _ := req.Clear.Get()
	cleared := make([]screeninganswers.Field, 0, len(toClear))
	for _, name := range toClear {
		f, err := screeninganswers.ParseField(name)
		if err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		cleared = append(cleared, f)
	}

	countries, _ := req.AuthorizedCountries.Get()
	answers, err := h.store.Update(ctx, userID, screeninganswers.Answers{
		AuthorizedCountries:   countries,
		VisaSponsorshipNeeded: ptrOf(req.VisaSponsorshipNeeded.Get),
		DesiredSalaryAmount:   intPtrOf(req.DesiredSalaryAmount.Get),
		DesiredSalaryCurrency: ptrOf(req.DesiredSalaryCurrency.Get),
		DesiredSalaryPeriod:   ptrOf(req.DesiredSalaryPeriod.Get),
		NoticePeriodDays:      intPtrOf(req.NoticePeriodDays.Get),
		WillingToRelocate:     ptrOf(req.WillingToRelocate.Get),
		Age18OrOlder:          ptrOf(req.Age18OrOlder.Get),
		PrivacyConsent:        ptrOf(req.PrivacyConsent.Get),
	}, cleared...)
	if err != nil {
		var ve *screeninganswers.ValidationError
		if errors.As(err, &ve) {
			return nil, fiber.NewError(fiber.StatusBadRequest, ve.Error())
		}
		return nil, err
	}
	return &apigen.UpdateScreeningAnswersOK{Data: screeningAnswersOut(answers)}, nil
}

// screeningAnswersOut is the stored answers in their wire shape, each unset field left
// out as the record's `omitempty` tags did.
func screeningAnswersOut(a screeninganswers.Answers) apigen.ScreeningAnswers {
	var out apigen.ScreeningAnswers
	if len(a.AuthorizedCountries) > 0 {
		out.AuthorizedCountries.SetTo(a.AuthorizedCountries)
	}
	optNil(a.VisaSponsorshipNeeded, out.VisaSponsorshipNeeded.SetTo)
	optNilInt(a.DesiredSalaryAmount, out.DesiredSalaryAmount.SetTo)
	optNil(a.DesiredSalaryCurrency, out.DesiredSalaryCurrency.SetTo)
	optNil(a.DesiredSalaryPeriod, out.DesiredSalaryPeriod.SetTo)
	optNilInt(a.NoticePeriodDays, out.NoticePeriodDays.SetTo)
	optNil(a.WillingToRelocate, out.WillingToRelocate.SetTo)
	optNil(a.Age18OrOlder, out.Age18OrOlder.SetTo)
	optNil(a.PrivacyConsent, out.PrivacyConsent.SetTo)
	return out
}
