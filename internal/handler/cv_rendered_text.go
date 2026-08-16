package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ledongthuc/pdf"

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

// renderedCVPageCount renders a CV document and counts the pages Typst actually laid it out
// onto — the number a length instruction in a prompt cannot see, since the model never
// receives the rendered artifact, only the JSON document it wrote.
//
// Only the renderer is required, not the text extractor: a page count needs no text layer,
// so this stays available exactly when RenderCVPDF itself would work.
func (h *cvHandlers) renderedCVPageCount(ctx context.Context, doc cv.Document, tmpl cv.Template) (int, error) {
	if h.cvRenderer == nil {
		return 0, errNoRenderer
	}
	data, err := h.cvRenderer.Render(ctx, doc, tmpl, nil, cv.LinkHrefs{})
	if err != nil {
		return 0, fmt.Errorf("render cv for page count: %w", err)
	}
	return pdfPageCount(data)
}

// pdfPageCount reads a page count out of PDF bytes this process itself just produced (Typst's
// own output, never a client upload), but ledongthuc/pdf panics — via bare `panic()` deep in
// its xref and object parsing — on a malformed file rather than returning an error, and this
// is the library's first production (non-test) caller. The recover keeps a Typst edge case
// this parser mishandles from taking down the goroutine it runs in, which here is the
// assistant tool loop's unrecovered SSE stream writer: nothing else in that call chain catches
// a panic.
//
// A zero or negative count reads the same as a parse failure: NumPage() derives it by walking
// Root -> Pages -> Count and returns 0 on a page-tree shape it does not expect, with no error
// of its own — trusting that silently would tell the model an empty CV is a one-page CV.
func pdfPageCount(data []byte) (pages int, err error) {
	defer func() {
		if r := recover(); r != nil {
			pages, err = 0, fmt.Errorf("parse rendered cv pdf: %v", r)
		}
	}()
	rd, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("read rendered cv pdf: %w", err)
	}
	n := rd.NumPage()
	if n <= 0 {
		return 0, fmt.Errorf("rendered cv pdf reports %d pages", n)
	}
	return n, nil
}
