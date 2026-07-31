package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/strelov1/freehire/internal/cv"
)

// errNoRenderer is CV scoring's own missing-renderer error, covering both halves of the
// toolchain: the Typst renderer and the PDF text extractor. The PDF endpoint answers 501
// for the renderer half because rendering is what that caller asked for; the scores are
// accessory reads, so they report themselves unavailable instead (see their handlers).
var errNoRenderer = errors.New("cv renderer is not configured")

// renderedCVText renders a CV document and returns the text layer of the PDF it produced.
//
// The rendered text — not the document — is the input every CV score reads, and that is the
// whole point: a document field the active template never renders contributes nothing, a
// template that buries the contact block scores its reading order as an ATS would read it,
// and a render that yields no extractable text fails machine-readability instead of passing
// on the strength of JSON nobody will parse.
//
// Shared by the ATS-readability delta and the job-match score so both degrade through one
// path when the toolchain is missing — and so they can never disagree about what the
// candidate's document says.
func (h *cvHandlers) renderedCVText(ctx context.Context, doc cv.Document, tmpl cv.Template) (string, error) {
	// Both halves are checked, because a handler assembled with one and not the other
	// exists: an unchecked extractor call turns a misassembled handler into a 500 rather
	// than the unavailable score every other missing-toolchain case yields.
	if h.cvRenderer == nil || h.extractPDFText == nil {
		return "", errNoRenderer
	}
	// No headshot: this render exists to be read as text, and a portrait contributes none.
	// Fetching it would cost a bucket round trip per scored template.
	pdf, err := h.cvRenderer.Render(ctx, doc, tmpl, nil, cv.LinkHrefs{})
	if err != nil {
		return "", fmt.Errorf("render cv for scoring: %w", err)
	}
	text, err := h.extractPDFText(pdf)
	if err != nil {
		return "", fmt.Errorf("extract rendered cv text: %w", err)
	}
	return text, nil
}
