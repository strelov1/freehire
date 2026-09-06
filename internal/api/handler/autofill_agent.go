package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/autofillagent"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// RunAgentAutofill fills the form on whatever page the caller's browser is
// showing: it attaches to that user's browser-tool channel as the harness end,
// reads the form, maps their profile onto it, and writes back what it can.
//
// The extension triggers this and then watches its own socket do the work — the
// agent never touches the DOM itself, it only drives the primitives.
//
// internal/ai/browsertools.Hub is keyed by user id, not session id, so this call reaches
// whatever browser the caller's extension currently has attached — regardless of
// which surface issued THIS request. Unlike read_current_page (see the
// confine-browse-preset-to-extension change), there is no degraded mode to fall back
// to: autofill WRITES into the live form on that page, so a request that did not
// authenticate as the extension's own Bearer session JWT is refused outright rather
// than run against a browser the caller on this surface never opened.
func (h *autofillHandlers) RunAgentAutofill(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !auth.IsExtensionBearer(c) {
		return fiber.NewError(fiber.StatusForbidden, "autofill runs only from the browser extension's own connection")
	}

	profile, err := h.autofillProfile(c.Context(), userID)
	if err != nil {
		return err
	}

	caller := h.browserTools.NewCaller(userID)
	defer caller.Close()

	report, err := autofillagent.Run(
		c.Context(),
		caller,
		autofillagent.LLMPlanner{Client: h.llm.bind(c.Context(), userID, llm.Feature(tagAutofill))},
		autofillagent.Profile(profile.Fields()),
	)
	if err != nil {
		switch {
		// The run needs the caller's browser attached and a form on the page. Either
		// missing is a state the user can fix, and the sentence names what to do.
		case errors.Is(err, browsertools.ErrNotConnected), errors.Is(err, autofillagent.ErrNoFillableFields):
			return fiber.NewError(fiber.StatusConflict, err.Error())
		// Autofill is not configured on this deployment — the same answer the other
		// model-backed endpoints give for the same reason.
		case errors.Is(err, autofillagent.ErrUnavailable):
			return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		default:
			// Handed to the shared mapper, as the cover letter's endpoint does with the
			// same shape of failure. Run makes two live model calls, so this branch holds
			// a gateway 5xx, our own 90-second llm.DefaultTimeout arriving as
			// context.DeadlineExceeded, and "the model's plan is not valid JSON" —
			// faults, every one. Flattening them into a 409 answered a state conflict for
			// a broken gateway, kept them out of Sentry, and printed the raw error in the
			// extension's panel. classify already knows a caller who navigated away is a
			// 499 and not a fault, so nothing here has to guess at that.
			return err
		}
	}
	return c.JSON(fiber.Map{"data": report})
}
