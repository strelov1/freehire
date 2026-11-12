package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/job/companyrelation"
)

// companyRelationHandlers serves the company corrections people propose (two slugs are one
// employer, or one company is another's subsidiary) and the moderator queue that decides
// them. The rules live in internal/job/companyrelation. This file only parses and renders.
type companyRelationHandlers struct {
	relations *companyrelation.Service
}

func (h *companyRelationHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie-only: a proposal is a person's judgement about two employers, and a leaked API
	// key must not be able to fill the moderators' queue.
	api.Post("/companies/:slug/relations", mw.cookie, contract)

	// The moderator queue sits under its own prefix rather than /companies/..., where a
	// literal segment would compete with the /companies/:slug read.
	api.Get("/company-relations", mw.cookie, mw.moderator, contract)
	api.Post("/company-relations/:id/approve", mw.cookie, mw.moderator, contract)
	api.Post("/company-relations/:id/reject", mw.cookie, mw.moderator, contract)
}

// companyRelationError maps the service's sentinels onto statuses. The ErrInvalid and
// ErrConflict messages name the reason and are written for the person who proposed, so
// they go out as they are.
func companyRelationError(err error) error {
	switch {
	case errors.Is(err, companyrelation.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, companyrelation.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, companyrelation.ErrDuplicate),
		errors.Is(err, companyrelation.ErrConflict),
		errors.Is(err, companyrelation.ErrDecided):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	default:
		return err
	}
}

// ProposeCompanyRelation records a correction for a moderator: {"kind": "same"|"subsidiary",
// "target_slug": "...", "note": "..."}. The route's :slug is the company the person is
// looking at, the one that retires (`same`) or the subsidiary (`subsidiary`).
func (h *companyRelationHandlers) ProposeCompanyRelation(ctx context.Context, req *apigen.CompanyRelationRequest, params apigen.ProposeCompanyRelationParams) (*apigen.ProposeCompanyRelationCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	row, err := h.relations.Propose(ctx, userID, companyrelation.Proposal{
		Kind:        companyrelation.Kind(req.Kind.Value),
		CompanySlug: params.Slug,
		TargetSlug:  req.TargetSlug.Value,
		Note:        req.Note.Value,
	})
	if err != nil {
		return nil, companyRelationError(err)
	}
	// The proposer and decider ids stay internal.
	return &apigen.ProposeCompanyRelationCreated{Data: apigen.CompanyRelationProposal{
		ID:          row.ID,
		Kind:        row.Kind,
		CompanySlug: row.CompanySlug,
		TargetSlug:  row.TargetSlug,
		Note:        row.Note,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt.Time,
	}}, nil
}

// ListPendingCompanyRelations serves the moderator queue, oldest first, one row per
// proposal with both companies' names and open-job counts so the decision needs no second page.
func (h *companyRelationHandlers) ListPendingCompanyRelations(ctx context.Context) (*apigen.ListPendingCompanyRelationsOK, error) {
	rows, err := h.relations.Pending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.PendingCompanyRelation, len(rows))
	for i, r := range rows {
		out[i] = apigen.PendingCompanyRelation{
			ID:              r.ID,
			Kind:            apigen.CompanyRelationKind(r.Kind),
			CompanySlug:     r.CompanySlug,
			CompanyName:     r.CompanyName,
			CompanyJobCount: int64(r.CompanyJobCount),
			TargetSlug:      r.TargetSlug,
			TargetName:      r.TargetName,
			TargetJobCount:  int64(r.TargetJobCount),
			Note:            r.Note,
			CreatedAt:       r.CreatedAt.Time,
			Status:          apigen.PendingCompanyRelationStatus(r.Status),
		}
	}
	return &apigen.ListPendingCompanyRelationsOK{Data: out}, nil
}

// ApproveCompanyRelation applies a proposal. A `same` approval moves the retiring slug's jobs
// within the request and answers with how many moved. Approving it again resumes a move that
// failed part-way.
func (h *companyRelationHandlers) ApproveCompanyRelation(ctx context.Context, params apigen.ApproveCompanyRelationParams) (*apigen.ApproveCompanyRelationOK, error) {
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	moderatorID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.relations.Approve(ctx, id, moderatorID)
	if err != nil {
		return nil, companyRelationError(err)
	}
	return &apigen.ApproveCompanyRelationOK{Data: apigen.CompanyRelationApproval{Kind: string(d.Kind), MovedJobs: d.MovedJobs}}, nil
}

// RejectCompanyRelation closes a pending proposal without applying it.
func (h *companyRelationHandlers) RejectCompanyRelation(ctx context.Context, params apigen.RejectCompanyRelationParams) (*apigen.RejectCompanyRelationOK, error) {
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	moderatorID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.relations.Reject(ctx, id, moderatorID); err != nil {
		return nil, companyRelationError(err)
	}
	return &apigen.RejectCompanyRelationOK{Data: apigen.CompanyRelationRejection{Rejected: true}}, nil
}
