package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/skilltag"
)

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

// scoreRenderedCV scores a CV's rendered text layer for ATS readability, against `keywords`
// as the keyword baseline. The CV's own skill set is parsed from that same text, so the
// score describes the rendered artifact and nothing outside it.
func (h *cvHandlers) scoreRenderedCV(ctx context.Context, doc cv.Document, tmpl cv.Template, keywords []string) (atscheck.Report, error) {
	text, err := h.renderedCVText(ctx, doc, tmpl)
	if err != nil {
		return atscheck.Report{}, err
	}
	return atscheck.Score(text, skilltag.Parse(text, skilltag.WithResumeAcronyms()), keywords), nil
}
