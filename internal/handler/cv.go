package handler

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cv"
)

// CV-builder HTTP surface: per-user structured CVs (CRUD + seed) and on-demand PDF
// rendering. Mutations are cookie-only (RequireAuth); the read + render endpoints also
// accept an API key (RequireAuthOrKey) so the tailoring agent's CLI can fetch a CV and its
// PDF. All routes are gated to beta testers / moderators (RequireModeratorOrBeta) at
// registration. Every operation is owner-scoped — a foreign id is a 404, never a leak.

const maxCVTitleRunes = 200

type cvMetaResponse struct {
	ID         int64     `json:"id"`
	Title      string    `json:"title"`
	TemplateID string    `json:"template_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type cvResponse struct {
	cvMetaResponse
	Document cv.Document `json:"document"`
}

type createCVRequest struct {
	Title string `json:"title"`
	// TemplateID selects the template; empty defaults to the classic-ats template.
	TemplateID string `json:"template_id"`
	// Seed pre-fills the new CV from the caller's stored résumé structure when available.
	Seed bool `json:"seed"`
}

type updateCVRequest struct {
	Title      string      `json:"title"`
	TemplateID string      `json:"template_id"`
	Document   cv.Document `json:"document"`
}

func metaResponse(m cv.Meta) cvMetaResponse {
	return cvMetaResponse{ID: m.ID, Title: m.Title, TemplateID: m.TemplateID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// ListCVs returns the caller's CVs as metadata, newest edit first.
func (a *API) ListCVs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	metas, err := a.cvStore.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]cvMetaResponse, len(metas))
	for i, m := range metas {
		out[i] = metaResponse(m)
	}
	return c.JSON(fiber.Map{"data": out})
}

// CreateCV creates a CV, optionally seeded from the caller's stored résumé structure.
func (a *API) CreateCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createCVRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}

	// Seeding pulls from the stored résumé's structured extraction, which lives in
	// Postgres (resume_structured) — independent of S3 object storage, so it is NOT gated
	// on résumé-storage being enabled. A missing structure degrades to an empty skeleton.
	doc := cv.EmptyDocument()
	if in.Seed {
		if st, ok, err := a.resume.Structured(c.Context(), userID); err == nil && ok {
			doc = cv.Seed(st)
		}
	}

	meta, err := a.cvStore.Create(c.Context(), userID, cvTitle(in.Title), tmplID, doc)
	if err != nil {
		return err
	}
	// Return the full record so the client can open the editor without a second fetch.
	rec, err := a.cvStore.Get(c.Context(), meta.ID, userID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": cvResponse{metaResponse(rec.Meta), rec.Document}})
}

// GetCV returns one owned CV with its full document.
func (a *API) GetCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := a.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": cvResponse{metaResponse(rec.Meta), rec.Document}})
}

// UpdateCV replaces an owned CV's title, template, and document.
func (a *API) UpdateCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var in updateCVRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}
	meta, err := a.cvStore.Update(c.Context(), int64(id), userID, cvTitle(in.Title), tmplID, in.Document)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}

// DeleteCV removes an owned CV.
func (a *API) DeleteCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	if err := a.cvStore.Delete(c.Context(), int64(id), userID); err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RenderCVPDF renders an owned CV to PDF and streams it. 501 when no renderer is
// configured (no typst binary); the CRUD surface still works in that state.
func (a *API) RenderCVPDF(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if a.cvRenderer == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := a.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	pdf, err := a.cvRenderer.Render(c.Context(), rec.Document, tmpl)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="cv.pdf"`)
	return c.Send(pdf)
}

// validCVTemplate rejects an unknown template_id (400) and resolves an empty one to the
// default; it returns the id to persist.
func validCVTemplate(id string) (string, error) {
	tmpl, err := cv.ResolveTemplate(id)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "unknown template")
	}
	return tmpl.ID, nil
}

// cvTitle trims, bounds, and defaults the CV title.
func cvTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Untitled CV"
	}
	if r := []rune(title); len(r) > maxCVTitleRunes {
		return strings.TrimSpace(string(r[:maxCVTitleRunes]))
	}
	return title
}

// mapCVError translates cv-domain errors into HTTP errors (ErrNotFound → 404, unknown
// template → 400); any other error propagates as a 500.
func mapCVError(err error) error {
	switch {
	case errors.Is(err, cv.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, cv.ErrUnknownTemplate):
		return fiber.NewError(fiber.StatusBadRequest, "unknown template")
	case errors.Is(err, cv.ErrInvalidPatch):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "invalid patch")
	default:
		return err
	}
}
