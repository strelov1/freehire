package handler

import (
	"context"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// CreateCV creates a CV, optionally seeded from the caller's stored résumé structure.
func (h *cvHandlers) CreateCV(ctx context.Context, req *apigen.CvCreate) (*apigen.CreateCVCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	// An explicit template on the request always wins; an omitted one falls back to the
	// caller's saved appearance defaults, then to the system default — never straight to
	// classic-ats the way validCVTemplate("") would on its own. The request has no
	// style/margin fields, so those always come from the saved (or system) defaults.
	defaults, _, err := h.cvStore.GetAppearanceDefaults(ctx, userID)
	if err != nil {
		return nil, err
	}
	requestedTemplateID := req.TemplateID.Value
	if requestedTemplateID == "" {
		requestedTemplateID = defaults.TemplateID
	}
	tmplID, err := validCVTemplate(requestedTemplateID)
	if err != nil {
		return nil, err
	}

	// Seeding pulls the work history from the experience bank and the rest from the stored
	// structure — both in Postgres, independent of S3 object storage, so it is NOT gated on
	// résumé-storage being enabled. Nothing known degrades to an empty skeleton.
	doc := cv.EmptyDocument()
	if req.Seed.Value {
		// A read that failed is not a candidate with nothing to seed from. Collapsing the
		// two answered 201 with an empty skeleton for someone whose CV we hold, and the
		// only way back is to notice and start again. Every other caller of this seed
		// propagates (cv_reseed.go).
		st, ok, err := h.seedSource().Structured(ctx, userID)
		if err != nil {
			return nil, err
		}
		if ok {
			doc = cv.Seed(st)
		}
	}
	doc.Style = defaults.Style
	doc.Margins = defaults.Margins

	meta, err := h.cvStore.Create(ctx, userID, cvTitle(req.Title.Value), tmplID, doc)
	if err != nil {
		return nil, err
	}
	// Return the full record so the client can open the editor without a second fetch.
	rec, err := h.cvStore.Get(ctx, meta.ID, userID)
	if err != nil {
		return nil, err
	}
	out, err := cvRecordOf(rec)
	if err != nil {
		return nil, err
	}
	return &apigen.CreateCVCreated{Data: out}, nil
}

// GetCV returns one owned CV: the full document on the owner's own session, and the
// document without its contact block to an agent reading with a key.
func (h *cvHandlers) GetCV(ctx context.Context, params apigen.GetCVParams) (*apigen.GetCVOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	// An API-key caller is an agent running its own model over the CV, so it reads through
	// the redacting accessor — the same one the in-process assistant's cv_get tool uses, so
	// one implementation covers both and neither can drift. The owner's own cookie session
	// reads in full; the stored contacts are untouched either way and still render in the PDF.
	viaKey := auth.ViaAPIKeyFromContext(ctx)
	read := h.cvStore.Get
	if viaKey {
		read = h.cvStore.GetForModel
	}
	rec, err := read(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	// Owner cookie sessions: fill empty header contacts from résumé identity
	// (owned / current / provisional — see healRecordHeader). API-key reads stay
	// redacted and never trigger a heal write. List and PDF do not heal.
	if !viaKey && rec.IsTailored {
		healed, herr := h.healRecordHeader(ctx, userID, rec)
		if herr != nil {
			log.Printf("cv: healing tailored header %s: %v", id, herr)
		} else {
			rec = healed
		}
		h.healBaseHeaderIfNeeded(ctx, userID)
	}
	out, err := cvRecordOf(rec)
	if err != nil {
		return nil, err
	}
	return &apigen.GetCVOK{Data: out}, nil
}

// UpdateCV replaces an owned CV's title, template, and document.
func (h *cvHandlers) UpdateCV(ctx context.Context, req *apigen.CvUpdate, params apigen.UpdateCVParams) (*apigen.UpdateCVOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	// The document is the CV package's own type: it decodes here, as it did from the body.
	var doc cv.Document
	if len(req.Document) > 0 {
		if err := json.Unmarshal(req.Document, &doc); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
	}
	tmplID, err := validCVTemplate(req.TemplateID.Value)
	if err != nil {
		return nil, err
	}
	// A whole-document save is an input format, not a second way to write the document: the
	// differ derives the operations, and from the editor's point of view this is
	// indistinguishable from an agent's batch. The actor is decided here, by the entry point
	// that authenticated the caller, and never read from the body.
	meta, _, err := h.editor.CommitDocument(ctx, id, userID,
		cvedit.ActorCandidate, cvedit.OriginEditor,
		cvedit.State{Title: cvTitle(req.Title.Value), TemplateID: tmplID, Document: doc})
	if err != nil {
		return nil, mapCVError(err)
	}
	return &apigen.UpdateCVOK{Data: cvMetaOf(meta)}, nil
}

// DeleteCV removes an owned CV.
func (h *cvHandlers) DeleteCV(ctx context.Context, params apigen.DeleteCVParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return err
	}
	if err := h.cvStore.Delete(ctx, id, userID); err != nil {
		return mapCVError(err)
	}
	return nil
}

// SetCVSession binds a roy agent session to an owned CV so the tailoring workspace can re-open
// that exact session later. Cookie or API key; owner-scoped (a foreign/missing id is a 404).
func (h *cvHandlers) SetCVSession(ctx context.Context, req *apigen.CvSessionBinding, params apigen.SetCVSessionParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return err
	}
	if err := h.cvStore.SetSession(ctx, id, userID, req.SessionID.Value); err != nil {
		return mapCVError(err)
	}
	return nil
}

// RenderCVPDF renders an owned CV to PDF and streams it. 501 when no renderer is
// configured (no typst binary); the CRUD surface still works in that state.
func (h *cvHandlers) RenderCVPDF(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if h.pdf.renderer == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	pdf, err := h.pdf.render(c.Context(), userID, id)
	if err != nil {
		return mapCVError(err)
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="cv.pdf"`)
	return c.Send(pdf)
}
