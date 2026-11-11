package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *jdResolveHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Post("/me/jd/resolve", mw.cookie, contract)
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
func (h *jdResolveHandlers) ResolveJobDescription(ctx context.Context, req *apigen.JobDescriptionSource) (*apigen.ResolveJobDescriptionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := jdResolveRequest{
		JobSlug: strings.TrimSpace(req.JobSlug.Value),
		URL:     strings.TrimSpace(req.URL.Value),
		Text:    strings.TrimSpace(req.Text.Value),
		Title:   req.Title.Value,
		Company: req.Company.Value,
	}

	present := 0
	for _, v := range []string{in.JobSlug, in.URL, in.Text} {
		if v != "" {
			present++
		}
	}
	if present != 1 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "exactly one of job_slug, url, or text is required")
	}

	slug, err := h.resolver.Resolve(ctx, userID, jdresolve.Request{
		JobSlug: in.JobSlug,
		URL:     in.URL,
		Text:    in.Text,
		Title:   strings.TrimSpace(in.Title),
		Company: strings.TrimSpace(in.Company),
	})
	if err != nil {
		return nil, jdResolveError(err)
	}
	return &apigen.ResolveJobDescriptionOK{Data: apigen.ResolvedJob{JobSlug: slug}}, nil
}
