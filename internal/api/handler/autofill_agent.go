package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/autofillagent"
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
		profileFields(profile),
	)
	if err != nil {
		// The run needs the caller's browser attached, a form on the page, and a
		// configured model. Any of those missing is a state problem, not a bad
		// request or a server fault.
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	return c.JSON(fiber.Map{"data": report})
}

// profileFields flattens the canonical profile into the keyed values the agent
// grounds its plan in.
func profileFields(p autofillProfile) autofillagent.Profile {
	return autofillagent.Profile{
		"full_name":  p.FullName,
		"first_name": p.FirstName,
		"last_name":  p.LastName,
		"email":      p.Email,
		"phone":      p.Phone,
		"location":   p.Location,
		"linkedin":   p.LinkedIn,
		"github":     p.GitHub,
		"portfolio":  p.Portfolio,

		"authorized_countries":    p.AuthorizedCountries,
		"visa_sponsorship_needed": p.VisaSponsorshipNeeded,
		"desired_salary":          p.DesiredSalary,
		"notice_period":           p.NoticePeriod,
		"willing_to_relocate":     p.WillingToRelocate,
		"age_18_or_older":         p.Age18OrOlder,
	}
}
