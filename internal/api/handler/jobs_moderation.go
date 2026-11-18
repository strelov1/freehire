package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// moderationError maps the moderation sentinels onto HTTP statuses. ErrInvalid carries a
// user-facing message surfaced in the 400 body; anything else (e.g. a DB failure) falls
// through to the central RenderError as a 500.
func moderationError(err error) error {
	switch {
	case errors.Is(err, moderation.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, moderation.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// CreateJob creates a hand-curated vacancy (moderator only). The body is validated by the
// service, so a missing required field or a bad URL is a 400 before any DB write. Returns
// the created job in the public wire shape with 201. url/title/company are required (by the
// service); source is the posting's real origin (defaults to "manual" when absent); remote
// defaults to false when absent.
//
// The optional structured facets: geography/work-mode/skills override dictionary
// derivation, and the salary fields become an authoritative manual salary. The
// moderation layer sanitizes them (drops unknown/invalid values).
func (h *jobsHandlers) CreateJob(ctx context.Context, req *apigen.JobCreate) (*apigen.CreateJobCreated, error) {
	actorID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	in := moderation.CreateInput{
		URL:            req.URL.Or(""),
		Source:         req.Source.Or(""),
		Title:          req.Title.Or(""),
		Company:        req.Company.Or(""),
		Location:       req.Location.Or(""),
		Remote:         req.Remote.Or(false),
		Description:    req.Description.Or(""),
		PostedAt:       ptrOf(req.PostedAt.Get),
		Skills:         req.Skills,
		Regions:        req.Regions,
		Cities:         req.Cities,
		WorkMode:       req.WorkMode.Or(""),
		EmploymentType: req.EmploymentType.Or(""),
		Seniority:      req.Seniority.Or(""),
		SalaryMin:      intPtrOf(req.SalaryMin.Get),
		SalaryMax:      intPtrOf(req.SalaryMax.Get),
		SalaryCurrency: req.SalaryCurrency.Or(""),
		SalaryPeriod:   req.SalaryPeriod.Or(""),
	}
	domainJob, extras, err := h.moderation.Create(ctx, actorID, in)
	if err != nil {
		return nil, moderationError(err)
	}

	view, err := jobview.FromDomain(domainJob, extras)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	return &apigen.CreateJobCreated{Data: jx.Raw(raw)}, nil
}

// UpdateJob partially edits a manual vacancy (moderator only), addressed by public slug.
// Every body field is optional, and a field left out (nil) is unchanged; the source
// identity (url) is not editable. A non-manual or unknown slug is a 404. Returns the
// updated job in the public wire shape.
func (h *jobsHandlers) UpdateJob(ctx context.Context, req *apigen.JobUpdate, params apigen.UpdateJobParams) (*apigen.UpdateJobOK, error) {
	actorID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	domainJob, extras, err := h.moderation.Update(ctx, actorID, params.Slug, moderation.UpdatePatch{
		Title:       ptrOf(req.Title.Get),
		Company:     ptrOf(req.Company.Get),
		Location:    ptrOf(req.Location.Get),
		Remote:      ptrOf(req.Remote.Get),
		Description: ptrOf(req.Description.Get),
		PostedAt:    ptrOf(req.PostedAt.Get),
	})
	if err != nil {
		return nil, moderationError(err)
	}

	view, err := jobview.FromDomain(domainJob, extras)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateJobOK{Data: jx.Raw(raw)}, nil
}
