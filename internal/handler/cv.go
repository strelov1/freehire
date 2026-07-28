package handler

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
)

// CV-builder HTTP surface: per-user structured CVs (CRUD + seed) and on-demand PDF
// rendering. Mutations are cookie-only (RequireAuth); the read + render endpoints also
// accept an API key (RequireAuthOrKey) so the tailoring agent's CLI can fetch a CV and its
// PDF. All routes are open to every signed-in user (the beta gate was lifted when tailoring
// went public; AI credits meter the LLM spend). Every operation is owner-scoped — a foreign
// id is a 404, never a leak.

// cvHandlers serves the CV builder + AI tailoring routes. The renderer is nil when no
// typst binary is configured; the PDF endpoint then returns 501 while the rest of the
// builder still works. match links the tailoring flow to the fit-analysis helpers
// (blocker capping, credits balance) it reuses.
type cvHandlers struct {
	// cvStore owns the CV-builder use cases (per-user structured CVs, CRUD + seed).
	cvStore            *cv.Store
	cvRenderer         cv.Renderer
	resume             *resume.Store
	queries            *db.Queries
	credits            *credits.Store
	matchAnalysisCache matchAnalysisStore
	match              *matchHandlers
}

func newCVHandlers(queries *db.Queries, typstBin string, resumeStore *resume.Store, creditsStore *credits.Store, match *matchHandlers) *cvHandlers {
	h := &cvHandlers{
		cvStore:            cv.NewStore(cv.NewQueriesRepository(queries)),
		resume:             resumeStore,
		queries:            queries,
		credits:            creditsStore,
		matchAnalysisCache: queries,
		match:              match,
	}
	// The renderer is enabled only when a typst binary was resolved (assign only a
	// non-nil renderer so the interface stays nil when disabled — a typed-nil would
	// defeat the 501 gate).
	if r := cv.NewTypstRenderer(typstBin); r != nil {
		h.cvRenderer = r
	}
	return h
}

func (h *cvHandlers) register(api fiber.Router, mw middleware) {
	// CV builder + AI tailoring: open to every signed-in user (AI credits meter the LLM spend).
	// Cookie-only, owner-scoped (a foreign id is a 404). The PDF endpoint 501s when no typst
	// binary is configured; the rest still works.
	api.Get("/cv-templates", mw.cookie, h.ListCVTemplates)
	api.Get("/me/cvs", mw.cookie, h.ListCVs)
	api.Post("/me/cvs", mw.cookie, h.CreateCV)
	// Read + render accept a key too, so the tailoring agent's CLI can fetch a CV and its
	// PDF; mutations stay cookie-only (POST/PUT/DELETE — the browser owns authoring).
	// cvKey, not key: the bootstrap mints the agent a narrow `cv`-scoped key, and key
	// (RequireAuthOrKey) admits full-scope keys only — it would answer the agent 403.
	api.Get("/me/cvs/:id", mw.cvKey, h.GetCV)
	api.Put("/me/cvs/:id", mw.cookie, h.UpdateCV)
	// Change only the template (the gallery's one-field switch); cookie-only like other mutations.
	api.Put("/me/cvs/:id/template", mw.cookie, h.SetCVTemplate)
	api.Delete("/me/cvs/:id", mw.cookie, h.DeleteCV)
	api.Get("/me/cvs/:id/pdf", mw.cvKey, h.RenderCVPDF)
	// Tailoring: the browser starts a session (cookie-only bootstrap); the agent's CLI drives
	// the edit + context/get/render reads with its minted `cv`-scoped key.
	api.Post("/me/cvs/tailor", mw.cookie, h.TailorCV)
	api.Post("/me/cvs/:id/tailor-session", mw.cookie, h.StartTailorSession)
	api.Patch("/me/cvs/:id", mw.cvKey, h.PatchCV)
	api.Put("/me/cvs/:id/session", mw.cvKey, h.SetCVSession)
	api.Get("/me/cvs/:id/tailor-context", mw.cvKey, h.TailorContext)
}

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
	// AgentSessionID is the roy session bound to a tailored CV (empty when none) — the tailoring
	// workspace resumes it.
	AgentSessionID string      `json:"agent_session_id"`
	Document       cv.Document `json:"document"`
}

// cvTailoredResponse is a tailored CV in the /my/cvs re-open list: metadata plus the vacancy
// slug and the bound agent session, so the client links each row to its tailoring workspace.
type cvTailoredResponse struct {
	cvMetaResponse
	JobSlug        string `json:"job_slug"`
	JobTitle       string `json:"job_title"`
	JobCompany     string `json:"job_company"`
	AgentSessionID string `json:"agent_session_id"`
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

func recordResponse(rec cv.Record) cvResponse {
	return cvResponse{cvMetaResponse: metaResponse(rec.Meta), AgentSessionID: rec.AgentSessionID, Document: rec.Document}
}

// ListCVTemplates returns the registered CV templates and their display metadata (id, label,
// style, ats_safe) so the UI can render the template gallery. Static registry data — no DB.
func (h *cvHandlers) ListCVTemplates(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": cv.Templates()})
}

// ListCVs returns the caller's TAILORED CVs (the re-open list), newest edit first, each with
// its vacancy slug and bound agent session so the client links back to the tailoring workspace.
func (h *cvHandlers) ListCVs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	items, err := h.cvStore.ListTailored(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]cvTailoredResponse, len(items))
	for i, m := range items {
		out[i] = cvTailoredResponse{
			cvMetaResponse: metaResponse(m.Meta),
			JobSlug:        m.JobSlug,
			JobTitle:       m.JobTitle,
			JobCompany:     m.JobCompany,
			AgentSessionID: m.AgentSessionID,
		}
	}
	return c.JSON(fiber.Map{"data": out})
}

// CreateCV creates a CV, optionally seeded from the caller's stored résumé structure.
func (h *cvHandlers) CreateCV(c *fiber.Ctx) error {
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
		if st, ok, err := h.resume.Structured(c.Context(), userID); err == nil && ok {
			doc = cv.Seed(st)
		}
	}

	meta, err := h.cvStore.Create(c.Context(), userID, cvTitle(in.Title), tmplID, doc)
	if err != nil {
		return err
	}
	// Return the full record so the client can open the editor without a second fetch.
	rec, err := h.cvStore.Get(c.Context(), meta.ID, userID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": recordResponse(rec)})
}

// GetCV returns one owned CV with its full document.
func (h *cvHandlers) GetCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := h.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	// An API-key caller (the CV-tailoring agent runs its own model over the CV) must not see
	// the contact block; the owner's own cookie session sees it in full. The stored contacts
	// are untouched and still render in the PDF.
	if auth.ViaAPIKey(c) {
		rec.Document.Header.FullName = ""
		rec.Document.Header.Email = ""
		rec.Document.Header.Phone = ""
		rec.Document.Header.Links = nil
	}
	return c.JSON(fiber.Map{"data": recordResponse(rec)})
}

// UpdateCV replaces an owned CV's title, template, and document.
func (h *cvHandlers) UpdateCV(c *fiber.Ctx) error {
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
	meta, err := h.cvStore.Update(c.Context(), int64(id), userID, cvTitle(in.Title), tmplID, in.Document)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}

// DeleteCV removes an owned CV.
func (h *cvHandlers) DeleteCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	if err := h.cvStore.Delete(c.Context(), int64(id), userID); err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type setCVSessionRequest struct {
	SessionID string `json:"session_id"`
}

// SetCVSession binds a roy agent session to an owned CV so the tailoring workspace can re-open
// that exact session later. Cookie or API key; owner-scoped (a foreign/missing id is a 404).
func (h *cvHandlers) SetCVSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var in setCVSessionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.cvStore.SetSession(c.Context(), int64(id), userID, in.SessionID); err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type setCVTemplateRequest struct {
	TemplateID string `json:"template_id"`
}

// SetCVTemplate changes only the template of an owned CV (title and document untouched). The
// gallery uses this to switch templates without re-sending the whole document. Owner-scoped
// (a foreign/missing id is a 404); an unknown template_id is a 400.
func (h *cvHandlers) SetCVTemplate(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var in setCVTemplateRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}
	if err := h.cvStore.SetTemplate(c.Context(), int64(id), userID, tmplID); err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RenderCVPDF renders an owned CV to PDF and streams it. 501 when no renderer is
// configured (no typst binary); the CRUD surface still works in that state.
func (h *cvHandlers) RenderCVPDF(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if h.cvRenderer == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rec, err := h.cvStore.Get(c.Context(), int64(id), userID)
	if err != nil {
		return mapCVError(err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	pdf, err := h.cvRenderer.Render(c.Context(), rec.Document, tmpl)
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
		// Surface the specific reason (unknown field, wrong type, out-of-range index)
		// so an LLM caller can fix the patch instead of retrying against a generic 422.
		reason := strings.TrimPrefix(err.Error(), cv.ErrInvalidPatch.Error()+": ")
		return fiber.NewError(fiber.StatusUnprocessableEntity, reason)
	default:
		return err
	}
}
