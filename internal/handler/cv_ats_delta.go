package handler

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/skilltag"
)

// errNoRenderer is scoring's own missing-renderer error, covering both halves of the
// toolchain: the Typst renderer and the PDF text extractor. The PDF endpoint answers 501 for
// the renderer half because rendering is what that caller asked for; the delta is an
// accessory read, so it reports the score as unavailable instead (see the handler).
var errNoRenderer = errors.New("cv renderer is not configured")

// atsDeltaResponse is the wire shape for the tailoring ATS delta. Available is false — with
// a short reason and no Delta — when the comparison could not be made for an environmental
// reason, so the workspace renders an absence instead of an error. BaseCVID names which base
// CV the comparison was against: the baseline is the base CV as it stands NOW, not a snapshot
// from when the copy was made, and a number whose baseline is anonymous invites misreading.
type atsDeltaResponse struct {
	Available bool            `json:"available"`
	Reason    string          `json:"reason,omitempty"`
	BaseCVID  string          `json:"base_cv_id,omitempty"`
	Delta     *atscheck.Delta `json:"delta,omitempty"`
}

// GetCVATSDelta serves what tailoring did to a CV's ATS readiness: the tailored copy scored
// against the base CV it came from, with the template, the page margins and the keyword
// baseline held identical so the difference is the document's content and nothing else.
//
// Cookie-only and owner-scoped (a foreign id is the same 404 as a missing one). A CV that is
// not a tailored copy has no defined baseline and is a 409 rather than a fabricated zero.
// Recomputed per request and never stored, so it always reflects the current documents,
// template and scoring rules.
func (h *cvHandlers) GetCVATSDelta(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	tailored, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	// Two different reasons a CV has no comparison, and the caller cannot act on the wrong one.
	// A pruned vacancy leaves a tailored copy with no job_id, so JobID alone cannot tell them apart.
	if !tailored.IsTailored {
		return fiber.NewError(fiber.StatusConflict, "this is a base CV: there is nothing to compare it against")
	}
	if tailored.JobID == 0 {
		return fiber.NewError(fiber.StatusConflict, "the vacancy this CV was tailored for no longer exists")
	}
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "no base CV to compare against")
	}
	tmpl, err := cv.ResolveTemplate(tailored.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	job, err := h.jobReader.GetJob(c.Context(), tailored.JobID)
	if err != nil {
		return err
	}

	// Hold everything but the content constant: the base is rendered with the tailored copy's
	// template and margins (a copy of the document — the stored base is never touched), and
	// both sides are scored against the vacancy's own canonical skills.
	baseDoc := base.Document
	baseDoc.Margins = tailored.Document.Margins

	baseReport, err := h.scoreRenderedCV(c.Context(), baseDoc, tmpl, job.Skills)
	if err != nil {
		return h.atsDeltaUnavailable(c, err)
	}
	tailoredReport, err := h.scoreRenderedCV(c.Context(), tailored.Document, tmpl, job.Skills)
	if err != nil {
		return h.atsDeltaUnavailable(c, err)
	}

	delta := atscheck.Compare(baseReport, tailoredReport)
	return c.JSON(fiber.Map{"data": atsDeltaResponse{
		Available: true,
		BaseCVID:  base.ID.String(),
		Delta:     &delta,
	}})
}

// atsDeltaUnavailable answers a scoring failure as an absent delta rather than an error: the
// workspace this read serves must keep loading and editing when the renderer is missing or a
// compile fails. The reason is a stable short string — the underlying error is logged, not
// served, so a toolchain detail never reaches the client.
func (h *cvHandlers) atsDeltaUnavailable(c *fiber.Ctx, err error) error {
	reason := "could not read the rendered CV"
	if errors.Is(err, errNoRenderer) {
		reason = "CV rendering is not available"
	} else {
		log.Printf("cv ats-delta: %v", err)
	}
	return c.JSON(fiber.Map{"data": atsDeltaResponse{Available: false, Reason: reason}})
}

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
	// Both halves are checked, because a handler assembled with one and not the other exists:
	// an unchecked extractor call turns a misassembled handler into a 500 rather than the
	// unavailable delta every other missing-toolchain case yields.
	if h.cvRenderer == nil || h.extractPDFText == nil {
		return atscheck.Report{}, errNoRenderer
	}
	// No headshot: this render exists to be read as text, and a portrait contributes
	// none. Fetching it would cost a bucket round trip per scored template.
	pdf, err := h.cvRenderer.Render(ctx, doc, tmpl, nil)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("render cv for scoring: %w", err)
	}
	text, err := h.extractPDFText(pdf)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("extract rendered cv text: %w", err)
	}
	return atscheck.Score(text, skilltag.Parse(text, skilltag.WithResumeAcronyms()), keywords), nil
}
