package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/survey"
)

// onboardingCompletionStore is the slice of *db.Queries the completion marker needs, kept
// narrow so the handler is unit-testable without a database.
type onboardingCompletionStore interface {
	MarkOnboardingComplete(ctx context.Context, id int64) error
}

// surveyHandlers serves the caller's own onboarding survey (job-search stage, biggest
// challenge, current income) plus the marker that the account has been through the wizard.
type surveyHandlers struct {
	store      *survey.Store
	onboarding onboardingCompletionStore
}

func newSurveyHandlers(store *survey.Store, onboarding onboardingCompletionStore) *surveyHandlers {
	return &surveyHandlers{store: store, onboarding: onboarding}
}

func (h *surveyHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// A singleton per user, keyed by the caller. Same auth split as /me/screening-answers:
	// the read takes a key, the write is cookie-only.
	api.Get("/me/survey", mw.key, contract)
	api.Put("/me/survey", mw.cookie, contract)
	// Cookie-only, and a POST rather than a PUT field: it records an event and takes no body.
	api.Post("/me/onboarding/complete", mw.cookie, contract)
}

// GetSurvey returns the caller's survey answers. A user with none gets an object with every
// field absent, not null or a 404: the wizard reads it to decide which steps to skip.
func (h *surveyHandlers) GetSurvey(ctx context.Context) (*apigen.GetSurveyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	answers, err := h.store.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetSurveyOK{Data: surveyOut(answers)}, nil
}

// UpdateSurvey partially updates the caller's survey answers: a field present is validated
// and stored, an omitted one is kept. A value outside its vocabulary, a note beside a coded
// challenge, a malformed currency or a non-positive income is a 400 naming the field. Cookie-only.
func (h *surveyHandlers) UpdateSurvey(ctx context.Context, req *apigen.SurveyResponses) (*apigen.UpdateSurveyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	challenges, _ := req.BiggestChallenges.Get()
	answers, err := h.store.Update(ctx, userID, survey.Responses{
		JobSearchStage:        ptrOf(req.JobSearchStage.Get),
		BiggestChallenges:     challenges,
		BiggestChallengeNote:  ptrOf(req.BiggestChallengeNote.Get),
		CurrentIncomeAmount:   intPtrOf(req.CurrentIncomeAmount.Get),
		CurrentIncomeCurrency: ptrOf(req.CurrentIncomeCurrency.Get),
		CurrentIncomePeriod:   ptrOf(req.CurrentIncomePeriod.Get),
	})
	if err != nil {
		var ve *survey.ValidationError
		if errors.As(err, &ve) {
			return nil, fiber.NewError(fiber.StatusBadRequest, ve.Error())
		}
		return nil, err
	}
	return &apigen.UpdateSurveyOK{Data: surveyOut(answers)}, nil
}

// CompleteOnboarding records that this account has been through the wizard, so it is never
// routed there again. Idempotent: the statement is guarded on IS NULL, so a repeat keeps the
// original timestamp and still answers 200.
func (h *surveyHandlers) CompleteOnboarding(ctx context.Context) (*apigen.CompleteOnboardingOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if err := h.onboarding.MarkOnboardingComplete(ctx, userID); err != nil {
		return nil, err
	}
	return &apigen.CompleteOnboardingOK{Data: apigen.OnboardingCompleted{OnboardingComplete: true}}, nil
}

// surveyOut is the stored survey in its wire shape, unset fields left out.
func surveyOut(r survey.Responses) apigen.SurveyResponses {
	var out apigen.SurveyResponses
	optNil(r.JobSearchStage, out.JobSearchStage.SetTo)
	if len(r.BiggestChallenges) > 0 {
		out.BiggestChallenges.SetTo(r.BiggestChallenges)
	}
	optNil(r.BiggestChallengeNote, out.BiggestChallengeNote.SetTo)
	optNilInt(r.CurrentIncomeAmount, out.CurrentIncomeAmount.SetTo)
	optNil(r.CurrentIncomeCurrency, out.CurrentIncomeCurrency.SetTo)
	optNil(r.CurrentIncomePeriod, out.CurrentIncomePeriod.SetTo)
	return out
}
