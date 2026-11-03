package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/job/companyrelation"
)

// companyRelationsPerHour bounds how many relations one account may propose in an hour.
// Each proposal is a row a moderator has to read, so the bound protects the queue more than
// the database. Ten is far above what a person correcting the companies they know needs.
const companyRelationsPerHour = 10

// companyRelationHandlers serves the company corrections people propose (two slugs are one
// employer, or one company is another's subsidiary) and the moderator queue that decides
// them. The rules live in internal/job/companyrelation. This file only parses and renders.
type companyRelationHandlers struct {
	relations *companyrelation.Service
	throttler ratelimit.Throttler
}

func (h *companyRelationHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only: a proposal is a person's judgement about two employers, and a leaked API
	// key must not be able to fill the moderators' queue.
	limit := ratelimit.Middleware(h.throttler, ratelimit.KeyByUserOrIP("company-relation"), companyRelationsPerHour, time.Hour)
	api.Post("/companies/:slug/relations", mw.cookie, limit, h.Propose)

	// The moderator queue sits under its own prefix rather than /companies/..., where a
	// literal segment would compete with the /companies/:slug read.
	api.Get("/company-relations", mw.cookie, mw.moderator, h.ListPending)
	api.Post("/company-relations/:id/approve", mw.cookie, mw.moderator, h.Approve)
	api.Post("/company-relations/:id/reject", mw.cookie, mw.moderator, h.Reject)
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

// companyRelationProposalResponse is the public shape of a stored proposal. The proposer
// and decider ids stay internal.
type companyRelationProposalResponse struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	CompanySlug string    `json:"company_slug"`
	TargetSlug  string    `json:"target_slug"`
	Note        string    `json:"note"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// Propose records a correction for a moderator: {"kind": "same"|"subsidiary",
// "target_slug": "...", "note": "..."}. The route's :slug is the company the person is
// looking at, the one that retires (`same`) or the subsidiary (`subsidiary`).
func (h *companyRelationHandlers) Propose(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in struct {
		Kind       string `json:"kind"`
		TargetSlug string `json:"target_slug"`
		Note       string `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	row, err := h.relations.Propose(c.Context(), userID, companyrelation.Proposal{
		Kind:        companyrelation.Kind(in.Kind),
		CompanySlug: c.Params("slug"),
		TargetSlug:  in.TargetSlug,
		Note:        in.Note,
	})
	if err != nil {
		return companyRelationError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": companyRelationProposalResponse{
		ID:          row.ID,
		Kind:        row.Kind,
		CompanySlug: row.CompanySlug,
		TargetSlug:  row.TargetSlug,
		Note:        row.Note,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt.Time,
	}})
}

// pendingCompanyRelation is one row of the moderator queue, with both companies' names and
// open-job counts so the decision needs no second page.
type pendingCompanyRelation struct {
	ID              int64     `json:"id"`
	Kind            string    `json:"kind"`
	CompanySlug     string    `json:"company_slug"`
	CompanyName     string    `json:"company_name"`
	CompanyJobCount int32     `json:"company_job_count"`
	TargetSlug      string    `json:"target_slug"`
	TargetName      string    `json:"target_name"`
	TargetJobCount  int32     `json:"target_job_count"`
	Note            string    `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
	// Status is "pending", or "approved" for a merge whose job move stopped part-way and
	// is listed so a moderator can resume it.
	Status string `json:"status"`
}

// ListPending serves the moderator queue, oldest first.
func (h *companyRelationHandlers) ListPending(c *fiber.Ctx) error {
	rows, err := h.relations.Pending(c.Context())
	if err != nil {
		return err
	}
	out := make([]pendingCompanyRelation, len(rows))
	for i, r := range rows {
		out[i] = pendingCompanyRelation{
			ID:              r.ID,
			Kind:            r.Kind,
			CompanySlug:     r.CompanySlug,
			CompanyName:     r.CompanyName,
			CompanyJobCount: r.CompanyJobCount,
			TargetSlug:      r.TargetSlug,
			TargetName:      r.TargetName,
			TargetJobCount:  r.TargetJobCount,
			Note:            r.Note,
			CreatedAt:       r.CreatedAt.Time,
			Status:          r.Status,
		}
	}
	return c.JSON(fiber.Map{"data": out})
}

// Approve applies a proposal. A `same` approval moves the retiring slug's jobs within the
// request and answers with how many moved. Approving it again resumes a move that failed
// part-way.
func (h *companyRelationHandlers) Approve(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	moderatorID, err := requireUserID(c)
	if err != nil {
		return err
	}
	d, err := h.relations.Approve(c.Context(), id, moderatorID)
	if err != nil {
		return companyRelationError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"kind": d.Kind, "moved_jobs": d.MovedJobs}})
}

// Reject closes a pending proposal without applying it.
func (h *companyRelationHandlers) Reject(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	moderatorID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.relations.Reject(c.Context(), id, moderatorID); err != nil {
		return companyRelationError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"rejected": true}})
}
