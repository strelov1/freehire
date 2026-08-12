package handler

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
	"github.com/strelov1/freehire/internal/skilltag"
)

// commitSurfaceAlign rewrites the tailored CV so skill wording matches the vacancy's
// preferred surfaces, as its own system revision (not an agent batch). No-op when the
// document is already aligned or the vacancy names no recognised skills.
func (h *cvHandlers) commitSurfaceAlign(ctx context.Context, userID int64, cvID uuid.UUID, jobDescription string) error {
	if h.editor == nil {
		return nil
	}
	preferred := skilltag.PreferredFromText(jobDescription)
	if len(preferred) == 0 {
		return nil
	}
	rec, err := h.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		return err
	}
	aligned := cv.Align(rec.Document, preferred)
	if !cv.AlignChanged(rec.Document, preferred) {
		return nil
	}
	_, _, err = h.editor.CommitDocument(ctx, cvID, userID,
		cvedit.ActorSystem, cvedit.OriginImport,
		cvedit.State{Title: rec.Title, TemplateID: rec.TemplateID, Document: aligned})
	if err != nil {
		return err
	}
	return nil
}

// logSurfaceAlign is best-effort commitSurfaceAlign for call sites that must not fail the
// surrounding flow (autopilot start).
func (h *cvHandlers) logSurfaceAlign(ctx context.Context, userID int64, cvID uuid.UUID, jobDescription string) {
	if err := h.commitSurfaceAlign(ctx, userID, cvID, jobDescription); err != nil {
		log.Printf("cv: surface-align cv=%s: %v", cvID, err)
	}
}
