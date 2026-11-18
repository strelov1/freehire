package handler

import (
	"context"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
)

// ListCVRevisions serves the history of what changed this CV, newest first.
//
// The feed carries the addresses each revision touched but not the operations themselves:
// the preview needs the places to underline, and the inverses hold the candidate's own
// earlier text, which has no business travelling out for a list view.
func (h *cvHandlers) ListCVRevisions(ctx context.Context, params apigen.ListCVRevisionsParams) (*apigen.ListCVRevisionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	// Owner-scoped read first, so a foreign CV is a 404 rather than an empty feed — the
	// same answer "no such CV" and "not yours" have everywhere else here.
	if _, err := h.cvStore.Get(ctx, id, userID); err != nil {
		return nil, mapCVError(err)
	}
	revisions, err := h.editor.History(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	raw, err := json.Marshal(cvedit.Views(revisions))
	if err != nil {
		return nil, err
	}
	return &apigen.ListCVRevisionsOK{Data: raw}, nil
}

// UndoCVRevision reverses one entry, leaving later edits in place. The undo is itself
// recorded, so the feed keeps describing what actually happened to the document.
func (h *cvHandlers) UndoCVRevision(ctx context.Context, params apigen.UndoCVRevisionParams) (*apigen.UndoCVRevisionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	revisionID, err := uuid.Parse(params.Rid)
	if err != nil {
		// A malformed id names no revision, so it is reported as missing rather than as a
		// bad request — keeping "no such revision" and "not yours" the same answer.
		return nil, fiber.NewError(fiber.StatusNotFound, "revision not found")
	}
	meta, undo, err := h.editor.Revert(ctx, id, userID, revisionID)
	if err != nil {
		return nil, mapCVError(err)
	}
	view, err := json.Marshal(undo.View())
	if err != nil {
		return nil, err
	}
	return &apigen.UndoCVRevisionOK{Data: apigen.CvRevisionUndo{Cv: cvMetaOf(meta), Revision: view}}, nil
}

// UndoCVRevisionBatch reverses every standing edit of one agent turn, newest first.
//
// The batch id comes from the feed rather than from a column on the CV: the run's own
// revisions carry it, which is what let the pre-run snapshot be retired along with the edge
// two concurrent runs used to create.
func (h *cvHandlers) UndoCVRevisionBatch(ctx context.Context, params apigen.UndoCVRevisionBatchParams) (*apigen.UndoCVRevisionBatchOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	batchID, err := uuid.Parse(params.Bid)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "run not found")
	}
	meta, err := h.editor.RevertBatch(ctx, id, userID, batchID)
	if err != nil {
		return nil, mapCVError(err)
	}
	// The run report goes with the run: it describes what the run made of each requirement,
	// and those edits have just been reversed. Best-effort — the document is already back,
	// and failing the request now would tell the candidate nothing was undone.
	if err := h.cvStore.SetAutopilotReport(ctx, id, userID, nil); err != nil {
		log.Printf("cv: clearing the run report after a batch revert: %v", err)
	}
	return &apigen.UndoCVRevisionBatchOK{Data: cvMetaOf(meta)}, nil
}
