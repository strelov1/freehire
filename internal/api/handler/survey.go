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

// surveyHandlers serves the caller's own onboarding survey — the three self-reported
// segmentation facts (job-search stage, biggest challenge, current income) — plus the
// marker recording that the account has been through the wizard at all.
//
// The marker sits here rather than in its own file because it is the same conversation:
// this is what the wizard writes on its way out, and splitting it off would put two halves
// of one screen's persistence in two places.
type surveyHandlers struct {
	store      *survey.Store
	onboarding onboardingCompletionStore
}

func newSurveyHandlers(store *survey.Store, onboarding onboardingCompletionStore) *surveyHandlers {
	return &surveyHandlers{store: store, onboarding: onboarding}
}

func (h *surveyHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The survey record is a singleton — one per user, keyed by the authenticated caller,
	// no id in the path. Same auth split as /me/screening-answers: the read takes a key,
	// the write is cookie-only.
	api.Get("/me/survey", mw.key, contract)
	api.Put("/me/survey", mw.cookie, contract)
	// Cookie-only, and deliberately a POST rather than a field on some PUT: it records an
	// event ("this account finished onboarding"), it takes no body, and there is nothing
	// about it a caller could set to a different value.
	api.Post("/me/onboarding/complete", mw.cookie, contract)
}

// GetSurvey returns the authenticated user's survey answers. A user who has answered
// nothing reads an object with every field absent — NOT null and not a 404. The wizard
// reads this to decide which steps it may skip, and a null would make it ask again for
// answers it already holds.
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

// UpdateSurvey partially updates the authenticated user's survey answers: a field the request
// body carries is validated and stored; a field it omits keeps its stored value. A value
// outside its vocabulary, a note beside a coded challenge, a malformed currency or a
// non-positive income is a 400 naming the offending field. Cookie-only.
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
// routed there again. Idempotent: the underlying statement is guarded on IS NULL, so a
// repeat call (a double-clicked finish, a decline after a finish) affects no rows, keeps
// the original timestamp, and still answers 200 — a second call is not a conflict.
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

// surveyOut is the stored survey in its wire shape, each unset field left out as the
// record's `omitempty` tags did.
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
