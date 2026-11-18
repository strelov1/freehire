package handler

import (
	"context"
	"encoding/json"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
)

// ListCVTemplates returns the registered CV templates and their display metadata (id, label,
// style, photo) so the UI can render the template gallery. Static registry data — no DB.
func (h *cvHandlers) ListCVTemplates(ctx context.Context) (*apigen.ListCVTemplatesOK, error) {
	raw, err := json.Marshal(cv.Templates())
	if err != nil {
		return nil, err
	}
	return &apigen.ListCVTemplatesOK{Data: raw}, nil
}

// ListCVFonts returns the registered typefaces a CV may choose (id, label, the familiar face
// it matches, and the CSS stack the live preview renders with). Static registry data — no DB.
// The client reads it instead of hard-coding a list, which is what keeps a second copy of the
// registry from growing in the web app and drifting from the one the renderer obeys.
func (h *cvHandlers) ListCVFonts(ctx context.Context) (*apigen.ListCVFontsOK, error) {
	raw, err := json.Marshal(cv.Fonts())
	if err != nil {
		return nil, err
	}
	return &apigen.ListCVFontsOK{Data: raw}, nil
}

// SetCVTemplate changes only the template of an owned CV (title and document untouched). The
// gallery uses this to switch templates without re-sending the whole document. Owner-scoped
// (a foreign/missing id is a 404); an unknown template_id is a 400.
func (h *cvHandlers) SetCVTemplate(ctx context.Context, req *apigen.CvTemplateChoice, params apigen.SetCVTemplateParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return err
	}
	tmplID, err := validCVTemplate(req.TemplateID.Value)
	if err != nil {
		return err
	}
	// Through the editor like every other change: "switched to the Sidebar template" is a
	// legitimate line in the history, and one the candidate may want to undo.
	path, err := cvedit.ParsePath("template_id")
	if err != nil {
		return err
	}
	_, _, err = h.editor.Commit(ctx, id, userID, cvedit.Change{
		Actor:  cvedit.ActorCandidate,
		Origin: cvedit.OriginTemplate,
		Ops:    []cvedit.Op{{Kind: cvedit.OpSet, Path: path, Value: tmplID}},
	})
	if err != nil {
		return mapCVError(err)
	}
	return nil
}
