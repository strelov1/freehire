package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/jdresolve"
)

// jdResolveHandlers serves the entry point that turns an existing job's slug, an
// external URL, or pasted JD text into a job usable by the tailor workspace.
type jdResolveHandlers struct {
	resolver *jdresolve.Resolver
}

func newJDResolveHandlers(resolver *jdresolve.Resolver) *jdResolveHandlers {
	return &jdResolveHandlers{resolver: resolver}
}

func (h *jdResolveHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/me/jd/resolve", mw.cookie, h.Resolve)
}

// jdResolveRequest is exactly one of JobSlug/URL/Text. Title/Company are optional hints
// used only alongside Text (they improve classification/skill derivation but are not
// required — see internal/job/jobderive).
type jdResolveRequest struct {
	JobSlug string `json:"job_slug"`
	URL     string `json:"url"`
	Text    string `json:"text"`
	Title   string `json:"title"`
	Company string `json:"company"`
}

type jdResolveResponse struct {
	JobSlug string `json:"job_slug"`
}

// jdResolveError maps the jdresolve sentinels onto HTTP statuses. Anything else (e.g. a
// DB failure) falls through to the central RenderError as a 500.
func jdResolveError(err error) error {
	switch {
	case errors.Is(err, jdresolve.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, jdresolve.ErrUnreadableURL):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "could not read a vacancy from that url")
	default:
		return err
	}
}

// Resolve turns the request's job_slug/url/text into a job usable by the tailor
// workspace. Exactly one of the three is required; anything else is a 400 before any
// resolution is attempted.
func (h *jdResolveHandlers) Resolve(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in jdResolveRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	in.JobSlug = strings.TrimSpace(in.JobSlug)
	in.URL = strings.TrimSpace(in.URL)
	in.Text = strings.TrimSpace(in.Text)

	present := 0
	for _, v := range []string{in.JobSlug, in.URL, in.Text} {
		if v != "" {
			present++
		}
	}
	if present != 1 {
		return fiber.NewError(fiber.StatusBadRequest, "exactly one of job_slug, url, or text is required")
	}

	slug, err := h.resolver.Resolve(c.Context(), userID, jdresolve.Request{
		JobSlug: in.JobSlug,
		URL:     in.URL,
		Text:    in.Text,
		Title:   strings.TrimSpace(in.Title),
		Company: strings.TrimSpace(in.Company),
	})
	if err != nil {
		return jdResolveError(err)
	}
	return c.JSON(fiber.Map{"data": jdResolveResponse{JobSlug: slug}})
}
