package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/headshot"
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
	cvStore    *cv.Store
	cvRenderer cv.Renderer
	resume     *resume.Store
	// photos serves the headshot the photo-bearing templates print. Nil-safe: an
	// unconfigured bucket, like a member with no photo, means the placeholder.
	photos             *headshot.Store
	queries            *db.Queries
	credits            *credits.Store
	matchAnalysisCache matchAnalysisStore
	// jobReader serves the vacancy a tailoring context is about. Narrow on purpose: the
	// tailoring path reads one row by id and nothing else.
	jobReader jobReader
	match     *matchHandlers
	// assistantSessions mints the conversation a tailoring workspace runs in.
	// Assigned after construction (see withAssistantSessions).
	assistantSessions *assistant.Store
	// seeder answers what a new CV starts from: the banked work history plus the sections
	// the stored structure still owns.
	seeder cv.Seeder
	// extractPDFText reads a rendered CV's text layer the way an ATS parser would. A field
	// rather than a direct call so a test can state the text layer without a poppler binary.
	extractPDFText func([]byte) (string, error)
	// editor is the only path that writes a stored CV. Every entry point — the editor's
	// autosave, the template picker, the CLI's patch, an agent tool, seeding a tailored
	// copy — commits through it, so no change happens without a revision recording it.
	editor *cvedit.Editor
}

// jobReader is the one vacancy read the tailoring context needs.
type jobReader interface {
	GetJob(ctx context.Context, id int64) (db.Job, error)
}

func newCVHandlers(pool *pgxpool.Pool, queries *db.Queries, typstBin string, resumeStore *resume.Store, photoStore *headshot.Store, creditsStore *credits.Store, match *matchHandlers) *cvHandlers {
	h := &cvHandlers{
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		// The editor is the only thing that writes a stored CV. The evidence gate is
		// attached later (withExperienceBank) because the bank is wired after this.
		editor:             cvedit.NewEditor(cvedit.NewRepository(pool, queries), nil),
		jobReader:          queries,
		resume:             resumeStore,
		photos:             photoStore,
		seeder:             bankedSeeder{resume: resumeStore, bank: newWorkHistoryReader(queries)},
		queries:            queries,
		credits:            creditsStore,
		matchAnalysisCache: queries,
		match:              match,
		extractPDFText:     resume.ExtractPDFText,
	}
	// The renderer is enabled only when a typst binary was resolved (assign only a
	// non-nil renderer so the interface stays nil when disabled — a typed-nil would
	// defeat the 501 gate).
	if r := cv.NewTypstRenderer(typstBin); r != nil {
		h.cvRenderer = r
	}
	return h
}

// withAssistantSessions gives the tailoring bootstrap the conversation store it
// mints a session in. Assigned after construction because the assistant is built
// from these handlers — the same shape authH.withAccountDeletion uses.
func (h *cvHandlers) withAssistantSessions(store *assistant.Store) {
	h.assistantSessions = store
}

// seedSource returns what a new CV starts from, and is never nil.
//
// A handler assembled without a seeder still seeds from the stored structure. "There is
// nothing to seed from" and "the seeder was not wired" are different statements:
// collapsing them hands a client that asked for a pre-filled CV a blank one, and passing
// the nil interface down to cv.Store.Tailor panics on the first tailoring bootstrap.
func (h *cvHandlers) seedSource() cv.Seeder {
	if h.seeder != nil {
		return h.seeder
	}
	return bankedSeeder{resume: h.resume}
}

func (h *cvHandlers) register(api fiber.Router, mw middleware) {
	// CV builder + AI tailoring: open to every signed-in user (AI credits meter the LLM spend).
	// Cookie-only, owner-scoped (a foreign id is a 404). The PDF endpoint 501s when no typst
	// binary is configured; the rest still works.
	api.Get("/cv-templates", mw.cookie, h.ListCVTemplates)
	api.Get("/cv-fonts", mw.cookie, h.ListCVFonts)
	api.Get("/me/cvs", mw.cookie, h.ListCVs)
	api.Post("/me/cvs", mw.cookie, h.CreateCV)
	// Read + render accept a key too (keyAuth), so the tailoring agent's CLI can fetch a CV
	// and its PDF; mutations stay cookie-only (POST/PUT/DELETE — the browser owns authoring).
	api.Get("/me/cvs/:id", mw.key, h.GetCV)
	api.Put("/me/cvs/:id", mw.cookie, h.UpdateCV)
	// Change only the template (the gallery's one-field switch); cookie-only like other mutations.
	api.Put("/me/cvs/:id/template", mw.cookie, h.SetCVTemplate)
	api.Delete("/me/cvs/:id", mw.cookie, h.DeleteCV)
	api.Get("/me/cvs/:id/pdf", mw.key, h.RenderCVPDF)
	// Tailoring: the browser starts a session (cookie-only bootstrap); the agent's CLI drives
	// the edit + context/get/render reads with its minted API key (keyAuth = cookie or Bearer).
	api.Post("/me/cvs/tailor", mw.cookie, h.TailorCV)
	api.Post("/me/cvs/:id/tailor-session", mw.cookie, h.StartTailorSession)
	api.Patch("/me/cvs/:id", mw.key, h.PatchCV)
	api.Put("/me/cvs/:id/session", mw.key, h.SetCVSession)
	api.Get("/me/cvs/:id/tailor-context", mw.key, h.TailorContext)
	// Undo a whole autopilot run. Cookie-only: it rewrites the document, and the browser
	// is where the candidate saw the run happen.
	api.Post("/me/cvs/:id/autopilot/undo", mw.cookie, h.UndoAutopilotRun)
	// What tailoring did to the CV's ATS readiness. Cookie-only, and deliberately so: the
	// tailoring agent authenticates with a CLI credential, so this gate is what keeps the
	// score out of the reach of the thing being measured.
	api.Get("/me/cvs/:id/ats-delta", mw.cookie, h.GetCVATSDelta)
	// How well the tailored CV matches the vacancy it was written for. Cookie-only for the
	// same reason as the delta: the tailoring agent authenticates with a CLI credential, and
	// this gate is what keeps the score out of the reach of the thing being measured.
	api.Get("/me/cvs/:id/job-match", mw.cookie, h.GetCVJobMatch)
}

const maxCVTitleRunes = 200

type cvMetaResponse struct {
	ID         string    `json:"id"`
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
	// AutopilotReport is the last unattended run's account of itself, one entry per
	// requirement. The workspace panel renders it from this read rather than by parsing
	// the conversation, so it survives a reload. Empty when no run has happened.
	AutopilotReport []cv.AutopilotEntry `json:"autopilot_report,omitempty"`
	// AutopilotRevertable says whether the pre-run snapshot is still held — whether
	// "undo the run" has anything to restore.
	AutopilotRevertable bool `json:"autopilot_revertable"`
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
	return cvMetaResponse{ID: m.ID.String(), Title: m.Title, TemplateID: m.TemplateID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func recordResponse(rec cv.Record) cvResponse {
	return cvResponse{
		cvMetaResponse:      metaResponse(rec.Meta),
		AgentSessionID:      rec.AgentSessionID,
		Document:            rec.Document,
		AutopilotReport:     rec.AutopilotReport,
		AutopilotRevertable: rec.AutopilotRevertable,
	}
}

// ListCVTemplates returns the registered CV templates and their display metadata (id, label,
// style, ats_safe) so the UI can render the template gallery. Static registry data — no DB.
func (h *cvHandlers) ListCVTemplates(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": cv.Templates()})
}

// ListCVFonts returns the registered typefaces a CV may choose (id, label, the familiar face
// it matches, and the CSS stack the live preview renders with). Static registry data — no DB.
// The client reads it instead of hard-coding a list, which is what keeps a second copy of the
// registry from growing in the web app and drifting from the one the renderer obeys.
func (h *cvHandlers) ListCVFonts(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": cv.Fonts()})
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

	// Seeding pulls the work history from the experience bank and the rest from the stored
	// structure — both in Postgres, independent of S3 object storage, so it is NOT gated on
	// résumé-storage being enabled. Nothing known degrades to an empty skeleton.
	doc := cv.EmptyDocument()
	if in.Seed {
		if st, ok, err := h.seedSource().Structured(c.Context(), userID); err == nil && ok {
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

// GetCV returns one owned CV: the full document on the owner's own session, and the
// document without its contact block to an agent reading with a key.
func (h *cvHandlers) GetCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	// An API-key caller is an agent running its own model over the CV, so it reads through
	// the redacting accessor — the same one the in-process assistant's cv_get tool uses, so
	// one implementation covers both and neither can drift. The owner's own cookie session
	// reads in full; the stored contacts are untouched either way and still render in the PDF.
	read := h.cvStore.Get
	if auth.ViaAPIKey(c) {
		read = h.cvStore.GetForModel
	}
	rec, err := read(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": recordResponse(rec)})
}

// UpdateCV replaces an owned CV's title, template, and document.
func (h *cvHandlers) UpdateCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in updateCVRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}
	// A whole-document save is an input format, not a second way to write the document: the
	// differ derives the operations, and from the editor's point of view this is
	// indistinguishable from an agent's batch. The actor is decided here, by the entry point
	// that authenticated the caller, and never read from the body.
	meta, _, err := h.editor.CommitDocument(c.Context(), id, userID,
		cvedit.ActorCandidate, cvedit.OriginEditor,
		cvedit.State{Title: cvTitle(in.Title), TemplateID: tmplID, Document: in.Document})
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}

// UndoAutopilotRun restores the document an autopilot run started from and clears the run's
// report along with it.
//
// The report goes with the document deliberately: it describes edits that the undo has just
// removed, so keeping it would leave the panel claiming work that is no longer on the page.
// A CV with no snapshot is a 409 rather than a silent no-op — nothing to undo is an answer,
// not a success.
func (h *cvHandlers) UndoAutopilotRun(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	meta, err := h.cvStore.RevertAutopilot(c.Context(), id, userID)
	if err != nil {
		if errors.Is(err, cv.ErrNoAutopilotRun) {
			return fiber.NewError(fiber.StatusConflict, "there is no autopilot run to undo")
		}
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
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	if err := h.cvStore.Delete(c.Context(), id, userID); err != nil {
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
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in setCVSessionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.cvStore.SetSession(c.Context(), id, userID, in.SessionID); err != nil {
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
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in setCVTemplateRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}
	// Through the editor like every other change: "switched to the Sidebar template" is a
	// legitimate line in the history, and one the candidate may want to undo.
	path, err := cvedit.ParsePath("template_id")
	if err != nil {
		return err
	}
	_, _, err = h.editor.Commit(c.Context(), id, userID, cvedit.Change{
		Actor:  cvedit.ActorCandidate,
		Origin: cvedit.OriginTemplate,
		Ops:    []cvedit.Op{{Kind: cvedit.OpSet, Path: path, Value: tmplID}},
	})
	if err != nil {
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
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	rec, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	pdf, err := h.cvRenderer.Render(c.Context(), rec.Document, tmpl, headshotForTemplate(c.Context(), h.photos, userID, tmpl))
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
	case errors.Is(err, cvedit.ErrInvalidOp):
		// Same reasoning as above, for the path-addressed operations: the reason IS the
		// remedy, and a caller that cannot see it can only retry the same mistake.
		return fiber.NewError(fiber.StatusUnprocessableEntity,
			strings.TrimPrefix(err.Error(), cvedit.ErrInvalidOp.Error()+": "))
	case errors.Is(err, cvedit.ErrForbiddenPath), errors.Is(err, cvedit.ErrEvidenceRequired):
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	case errors.Is(err, cvedit.ErrNothingToUndo):
		return fiber.NewError(fiber.StatusConflict, "there is nothing to undo")
	case errors.Is(err, cvedit.ErrCannotUndo):
		// A fact about the document as it stands, not a malformed request: the place this
		// edit changed is gone, so its inverse has nowhere to land.
		return fiber.NewError(fiber.StatusConflict,
			"this edit can no longer be undone — the part of the CV it changed is gone")
	default:
		return err
	}
}

// cvPathID parses the :id route param as a CV's UUID. A malformed id cannot name
// any CV, so it is reported as missing rather than as a bad request — keeping
// "not a CV" and "not yours" the same answer, which is what stops the id from
// being probeable.
func cvPathID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, "cv not found")
	}
	return id, nil
}
