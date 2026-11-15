package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/autofillagent"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// RunAutofill fills the form on whatever page the caller's browser is
// showing: it attaches to that user's browser-tool channel as the harness end,
// reads the form, maps their profile onto it, and writes back what it can.
//
// The extension triggers this and watches its own socket; the agent never touches the
// DOM, it only drives the primitives.
//
// internal/ai/browsertools.Hub is keyed by user id, not session id, so this reaches
// whatever browser the caller's extension has attached, whichever surface issued THIS
// request. Autofill WRITES into the live form, so unlike read_current_page there is no
// degraded mode: a request not authenticated as the extension's own Bearer session JWT
// is refused outright.
func (h *autofillHandlers) RunAutofill(ctx context.Context) (*apigen.RunAutofillOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if !auth.IsExtensionBearerFromContext(ctx) {
		return nil, fiber.NewError(fiber.StatusForbidden, "autofill runs only from the browser extension's own connection")
	}

	profile, err := h.autofillProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	model, err := h.llm.bind(ctx, userID, llm.Feature(tagAutofill))
	if err != nil {
		return nil, err
	}

	caller := h.browserTools.NewCaller(userID)
	defer caller.Close()

	report, err := autofillagent.Run(
		ctx,
		caller,
		autofillagent.NewJevPlanner(
			autofillagent.LLMPlanner{Client: model},
			h.typesafeAPIKey,
		),
		autofillagent.Profile(profile.Fields()),
		// The bank travels apart from the profile: it is recalled by topic, never shown
		// to the model or counted as grounding (see autofillagent.Banked).
		autofillagent.Banked(profile.BankAnswers),
	)
	if err != nil {
		switch {
		// The run needs the caller's browser attached, a readable page and a form on it;
		// each missing piece is user-fixable and the message says how. ErrExecutorReported
		// carries the extension's own words ("no active tab") and is the MOST COMMON failure,
		// so it must not reach the default branch and file a report about a closed tab.
		case errors.Is(err, browsertools.ErrNotConnected),
			errors.Is(err, browsertools.ErrExecutorReported),
			errors.Is(err, autofillagent.ErrNoFillableFields):
			return nil, fiber.NewError(fiber.StatusConflict, err.Error())
		// Autofill is not configured on this deployment, as for the other model-backed endpoints.
		case errors.Is(err, autofillagent.ErrUnavailable):
			return nil, fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		default:
			// Handed to the shared mapper, as the cover letter endpoint does. Run makes two live
			// model calls, so this branch holds faults (gateway 5xx, llm.DefaultTimeout as
			// DeadlineExceeded, invalid plan JSON); a 409 would hide them from error reports
			// and print the raw error in the extension panel. classify already treats a
			// navigated-away caller as 499.
			return nil, err
		}
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	return &apigen.RunAutofillOK{Data: raw}, nil
}
