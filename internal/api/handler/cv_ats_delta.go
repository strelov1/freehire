package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// atsDeltaResponse is the wire shape for the tailoring ATS delta. Available is false — with
// a short reason and no Delta — when the comparison could not be made for an environmental
// reason, so the workspace renders an absence instead of an error. BaseCVID names which base
// CV the comparison was against: the baseline is the base CV as it stands NOW, not a snapshot
// from when the copy was made, and a number whose baseline is anonymous invites misreading.
// GetCVATSDelta serves what tailoring did to a CV's ATS readiness: the tailored copy scored
// against the base CV it came from, with the template, the page margins and the keyword
// baseline held identical so the difference is the document's content and nothing else.
//
// Cookie-only and owner-scoped (a foreign id is the same 404 as a missing one). A CV that is
// not a tailored copy has no defined baseline and is a 409 rather than a fabricated zero.
// Recomputed per request and never stored, so it always reflects the current documents,
// template and scoring rules.
func (h *cvHandlers) GetCVATSDelta(ctx context.Context, params apigen.GetCVATSDeltaParams) (*apigen.GetCVATSDeltaOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	tailored, err := h.cvStore.Get(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	// Two different reasons a CV has no comparison, and the caller cannot act on the wrong one.
	// A pruned vacancy leaves a tailored copy with no job_id, so JobID alone cannot tell them apart.
	if !tailored.IsTailored {
		return nil, fiber.NewError(fiber.StatusConflict, "this is a base CV: there is nothing to compare it against")
	}
	if tailored.JobID == 0 {
		return nil, fiber.NewError(fiber.StatusConflict, "the vacancy this CV was tailored for no longer exists")
	}
	base, ok, err := h.cvStore.BaseFor(ctx, userID, tailored)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fiber.NewError(fiber.StatusConflict, "no base CV to compare against")
	}
	tmpl, err := cv.ResolveTemplate(tailored.TemplateID)
	if err != nil {
		return nil, mapCVError(err)
	}
	job, err := h.jobReader.GetJob(ctx, tailored.JobID)
	if err != nil {
		return nil, err
	}

	// Hold everything but the content constant: the base is rendered with the tailored copy's
	// template, margins, and typography (a copy of the document — the stored base is never
	// touched), and both sides are scored against the vacancy's own canonical skills.
	//
	// Typography is in that list because type size and leading decide how much text lands on a
	// page, and the score reads the rendered PDF's text layer. Omitting it would hand the
	// candidate a way to move their own delta by changing a font.
	baseDoc := base.Document
	baseDoc.Margins = tailored.Document.Margins
	baseDoc.Style = tailored.Document.Style

	baseReport, err := h.scoreRenderedCV(ctx, baseDoc, tmpl, job.Skills)
	if err != nil {
		return h.atsDeltaUnavailable(err)
	}
	tailoredReport, err := h.scoreRenderedCV(ctx, tailored.Document, tmpl, job.Skills)
	if err != nil {
		return h.atsDeltaUnavailable(err)
	}

	delta := atscheck.Compare(baseReport, tailoredReport)
	rawDelta, err := json.Marshal(delta)
	if err != nil {
		return nil, err
	}
	return &apigen.GetCVATSDeltaOK{Data: apigen.CvAtsDelta{
		Available: true,
		BaseCvID:  apigen.NewOptString(base.ID.String()),
		Delta:     rawDelta,
	}}, nil
}

// atsDeltaUnavailable answers a scoring failure as an absent delta rather than an error: the
// workspace this read serves must keep loading and editing when the renderer is missing or a
// compile fails. The reason is a stable short string — the underlying error is logged, not
// served, so a toolchain detail never reaches the client.
func (h *cvHandlers) atsDeltaUnavailable(err error) (*apigen.GetCVATSDeltaOK, error) {
	reason := "could not read the rendered CV"
	if errors.Is(err, errNoRenderer) {
		reason = "CV rendering is not available"
	} else {
		log.Printf("cv ats-delta: %v", err)
	}
	return &apigen.GetCVATSDeltaOK{Data: apigen.CvAtsDelta{Available: false, Reason: apigen.NewOptString(reason)}}, nil
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
