package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/autofillagent"
)

// RunAgentAutofill fills the form on whatever page the caller's browser is
// showing: it attaches to that user's browser-tool channel as the harness end,
// reads the form, maps their profile onto it, and writes back what it can.
//
// The extension triggers this and then watches its own socket do the work — the
// agent never touches the DOM itself, it only drives the primitives.
func (h *autofillHandlers) RunAgentAutofill(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
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
		autofillagent.LLMPlanner{Client: h.llm.bind(c.Context(), userID, tagAutofill)},
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
