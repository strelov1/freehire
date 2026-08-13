package cvedit

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
)

// Revert undoes one revision by applying its inverse to the CURRENT document.
//
// Two consequences worth stating, because both are the point rather than side effects. Edits
// made after the one being undone survive — only what that revision did is reversed. And the
// undo is recorded as a revision of its own, naming what it reversed, so the log is never
// rewritten: an undo can itself be undone, and the feed keeps describing what actually
// happened to the document.
//
// Neither the path policy nor the evidence gate applies here. An inverse restores text the
// candidate already had on the page — asking it to cite evidence for words it is putting
// back would refuse the undo of the very edit the gate let through.
func (e *Editor) Revert(ctx context.Context, cvID uuid.UUID, userID int64, revisionID uuid.UUID) (cv.Meta, Revision, error) {
	var (
		meta cv.Meta
		undo Revision
	)
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		target, ok, err := tx.Revision(ctx, revisionID)
		if err != nil {
			return err
		}
		// A revision id names an entry in ONE CV's history. Without this, undoing it through
		// another CV of the same owner would apply that CV's inverses to this document — a
		// write to the wrong place, driven entirely by a path parameter.
		if !ok || target.CVID != cvID || target.Reverted() {
			return ErrNothingToUndo
		}
		meta, undo, err = e.undo(ctx, tx, cvID, target.Inverse, []uuid.UUID{target.ID},
			"Undid: "+target.Title, target.ID)
		return err
	})
	return meta, undo, err
}

// RevertBatch undoes every standing revision of one agent turn or autopilot run, newest
// first. It is what "undo the run" means now that there is no pre-run snapshot: grouping by
// batch removes the edge two concurrent runs used to create, where each snapshotted a
// half-edited document and reverting returned to the middle of the other one.
//
// The run report is not cleared here. It describes what the run made of each REQUIREMENT,
// which is a different question from what it changed, and it lives on the CV row rather than
// in the log — the caller clears it, because it owns that column.
func (e *Editor) RevertBatch(ctx context.Context, cvID uuid.UUID, userID int64, batchID uuid.UUID) (cv.Meta, error) {
	var meta cv.Meta
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		standing, err := tx.InBatch(ctx, batchID)
		if err != nil {
			return err
		}
		if len(standing) == 0 {
			return ErrNothingToUndo
		}

		// Newest first: a run's later edits are undone before the earlier ones they were
		// layered on, which is the only order that leaves the earlier inverses applicable.
		var (
			inverse []Op
			ids     []uuid.UUID
		)
		for _, rev := range standing {
			inverse = append(inverse, rev.Inverse...)
			ids = append(ids, rev.ID)
		}

		meta, _, err = e.undo(ctx, tx, cvID, inverse, ids, "Undid the agent's run", uuid.Nil)
		return err
	})
	return meta, err
}

// undo applies inverse operations, marks what they reversed, and records the undo as a new
// revision. Everything happens inside the caller's transaction against the locked row.
func (e *Editor) undo(ctx context.Context, tx Tx, cvID uuid.UUID, inverse []Op,
	reverted []uuid.UUID, title string, revertsID uuid.UUID) (cv.Meta, Revision, error) {

	before, baseVersion, err := tx.State(ctx)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}

	applied, applyRedo, err := Apply(before, inverse)
	if err != nil {
		// The place the inverse would restore is gone. This is a fact about the document as
		// it stands, not a malformed request — and it cannot be known without trying, which
		// is why the control is offered and the failure explained.
		return cv.Meta{}, Revision{}, fmt.Errorf("%w: %s", ErrCannotUndo, err)
	}
	after := applied
	after.Sanitize()

	// Same rule as a commit: the precise inverse unless the sanitizer moved something, in
	// which case the diff against what was stored is the only one that applies cleanly.
	redo := applyRedo
	if !Equal(applied, after) {
		redo = Diff(after, before)
	}

	meta, err := tx.Save(ctx, after)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}
	for _, id := range reverted {
		if _, err := tx.MarkReverted(ctx, id); err != nil {
			return cv.Meta{}, Revision{}, err
		}
	}

	rev, err := tx.Insert(ctx, Revision{
		CVID:        cvID,
		Actor:       ActorCandidate,
		Origin:      OriginEditor,
		Title:       title,
		Ops:         inverse,
		Inverse:     redo,
		BaseVersion: baseVersion,
		RevertsID:   revertsID,
	})
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}
	if err := tx.Trim(ctx, feedLimit); err != nil {
		return cv.Meta{}, Revision{}, err
	}
	return meta, rev, nil
}
