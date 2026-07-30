package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/skilltag"
)

// errNoRenderer is scoring's own missing-renderer error. The PDF endpoint answers 501 for
// the same condition because rendering is what that caller asked for; the delta is an
// accessory read, so it reports the score as unavailable instead (see the handler).
var errNoRenderer = errors.New("cv renderer is not configured")

// scoreRenderedCV renders a CV document and scores the text layer of the PDF it produced,
// against `keywords` as the keyword baseline.
//
// The rendered text — not the document — is the scoring input, and that is the whole point:
// a document field the active template never renders contributes nothing, a template that
// buries the contact block scores its reading order as an ATS would read it, and a render
// that yields no extractable text fails machine-readability instead of passing on the
// strength of JSON nobody will parse. The CV's own skill set is parsed from that same text
// for the same reason.
func (h *cvHandlers) scoreRenderedCV(ctx context.Context, doc cv.Document, tmpl cv.Template, keywords []string) (atscheck.Report, error) {
	if h.cvRenderer == nil {
		return atscheck.Report{}, errNoRenderer
	}
	pdf, err := h.cvRenderer.Render(ctx, doc, tmpl)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("render cv for scoring: %w", err)
	}
	text, err := h.extractPDFText(pdf)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("extract rendered cv text: %w", err)
	}
	return atscheck.Score(text, skilltag.Parse(text, skilltag.WithResumeAcronyms()), keywords), nil
}
