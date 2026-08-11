package handler

import (
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

// applySeedContent builds the next editable state from a résumé seed while keeping the
// candidate's presentation choices. Title and template stay on the row; margins and style
// stay on the document. Body sections come from cv.Seed; header contacts merge so an empty
// seed field never blanks a value the candidate already has on the page.
//
// Shared by Reset from résumé for both the base CV and the tailored copy so the two cannot
// drift on what "preserve presentation" means.
func applySeedContent(keep cvedit.State, seeded cv.Document) cvedit.State {
	doc := seeded
	doc.Margins = keep.Margins
	doc.Style = keep.Style
	doc.Header = mergeSeedHeader(keep.Header, seeded.Header)
	return cvedit.State{
		Title:      keep.Title,
		TemplateID: keep.TemplateID,
		Document:   doc,
	}
}

// mergeSeedHeader is seed-first for reset-from-résumé: a non-empty seed field
// replaces. Contrast fillEmptyHeaderFields (keep-first) on GET heal. See
// internal/resume/AGENTS.md.
func mergeSeedHeader(keep, seeded cv.Header) cv.Header {
	out := seeded
	if out.FullName == "" {
		out.FullName = keep.FullName
	}
	if out.Email == "" {
		out.Email = keep.Email
	}
	if out.Phone == "" {
		out.Phone = keep.Phone
	}
	if out.Location == "" {
		out.Location = keep.Location
	}
	if len(out.Links) == 0 {
		out.Links = keep.Links
	}
	return out
}
