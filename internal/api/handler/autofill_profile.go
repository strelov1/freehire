package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/api/candidateprofile"
)

// autofillHandlers serves the contact block the browser extension fills application forms
// with, both as a plain read and as an agent-driven run over the caller's own browser.
//
// It holds the profile assembler (internal/api/candidateprofile — the same one cmd/auto-apply
// resolves an application form against, so a value a person sees in a form and a value an
// automated attempt resolves against can never diverge), the browser-tool hub the agent run
// drives the caller's page through, and the model binding that run plans on.
type autofillHandlers struct {
	profiles *candidateprofile.Assembler
	// browserTools is shared with /tools/ws and the assistant — the hub is per-process and
	// routes strictly within one user's channel.
	browserTools *browsertools.Hub
	// llm plans the agent-driven fill. A nil client is the LLM being unconfigured: the run
	// reports the feature is off and the deterministic read still works.
	llm llmBinding
}

func newAutofillHandlers(cvs candidateprofile.CVReader, resumes candidateprofile.ResumeReader, accounts candidateprofile.AccountReader, screeningAnswers candidateprofile.ScreeningAnswersReader, tools *browsertools.Hub, llm llmBinding) *autofillHandlers {
	return &autofillHandlers{
		profiles:     candidateprofile.NewAssembler(cvs, resumes, accounts, screeningAnswers),
		browserTools: tools,
		llm:          llm,
	}
}

func (h *autofillHandlers) register(api fiber.Router, mw middleware) {
	// Canonical autofill fields (name/email/phone/location/links) for the browser
	// extension to write into application forms. keyAuth (Bearer).
	api.Get("/me/autofill-profile", mw.key, h.AutofillProfile)
	// Agent-driven autofill: the caller's own browser is driven over the browser-tool
	// wire. keyAuth admits the cookie too, same as every other mw.key route — the
	// handler itself refuses anything but the extension's own Bearer session JWT,
	// because unlike a read this one writes into a live form.
	api.Post("/me/autofill/run", mw.key, h.RunAgentAutofill)
}

// autofillProfile assembles a user's canonical autofill fields via internal/api/candidateprofile.
func (h *autofillHandlers) autofillProfile(ctx context.Context, userID int64) (candidateprofile.Profile, error) {
	return h.profiles.Assemble(ctx, userID)
}

// AutofillProfile returns the caller's canonical autofill fields. keyAuth so the
// browser extension (Bearer) can read it. All-empty (bar the account email) when no
// source states a contact.
func (h *autofillHandlers) AutofillProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	profile, err := h.autofillProfile(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": profile})
}
