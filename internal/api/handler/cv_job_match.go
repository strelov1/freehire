package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// cvJobMatchResponse is the wire shape for the tailoring job-match score. Available is false
// — with a short reason and no Score — when the score could not be produced, so the
// workspace renders an absence instead of an error. That covers both an absent toolchain
// and a vacancy nothing could be matched against: from the panel's side they are the same
// answer, and giving the client one falsy case to handle is worth more than telling the two
// apart in a field nobody would branch on.
type cvJobMatchResponse struct {
	Available bool           `json:"available"`
	Reason    string         `json:"reason,omitempty"`
	Score     *cvmatch.Score `json:"score,omitempty"`
}

// GetCVJobMatch serves how well a tailored CV matches the vacancy it was written for,
// measured on the rendered PDF's text layer and against that vacancy alone.
//
// Unlike the ATS delta this reads no base CV and renders one document rather than two,
// which is what makes it cheap enough for the workspace to refresh after every saved edit.
// Deterministic and free: no model is called, so nothing here is metered.
//
// Cookie-only and owner-scoped (a foreign id is the same 404 as a missing one). A CV with
// no vacancy has nothing to be matched against and is a 409 naming which case it is.
func (h *cvHandlers) GetCVJobMatch(c *fiber.Ctx) error {
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
	// Two different reasons a CV has no vacancy, and the caller cannot act on the wrong
	// one. A pruned vacancy leaves a tailored copy with no job_id, so JobID alone cannot
	// tell them apart.
	if !tailored.IsTailored {
		return fiber.NewError(fiber.StatusConflict, "this is a base CV: it is not bound to a vacancy")
	}
	if tailored.JobID == 0 {
		return fiber.NewError(fiber.StatusConflict, "the vacancy this CV was tailored for no longer exists")
	}
	tmpl, err := cv.ResolveTemplate(tailored.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	job, err := h.jobReader.GetJob(c.Context(), tailored.JobID)
	if err != nil {
		return err
	}

	// A missing analysis degrades Requirements Coverage; it does not fail the read. The
	// workspace opens before any analysis is guaranteed current, and a score of the other
	// three categories is worth more than a refusal.
	analysis, hasAnalysis := h.fit.Optional(c.Context(), userID, tailored.JobID)

	score, err := h.cvJobMatchScore(c.Context(), tailored.Document, tmpl, cvmatch.Input{
		JobTitle:     job.Title,
		JobSkills:    job.Skills,
		Requirements: cvJobMatchRequirements(analysis),
		HasAnalysis:  hasAnalysis,
	})
	if err != nil {
		return h.cvJobMatchUnavailable(c, err)
	}
	// Nothing about this vacancy could be evaluated. Reporting a zero would read as a CV
	// that matched nothing, which is a different and defamatory claim.
	if len(score.Contributing) == 0 {
		return c.JSON(fiber.Map{"data": cvJobMatchResponse{
			Available: false,
			Reason:    "nothing about this vacancy could be matched automatically",
		}})
	}
	return c.JSON(fiber.Map{"data": cvJobMatchResponse{Available: true, Score: &score}})
}

// cvJobMatchScore fills the document half of a job-match input from the rendered CV and
// scores it. The caller supplies the vacancy half.
//
// The CV's skills are parsed from the rendered text — the same parse, of the same text, the
// ATS score reads — so the two surfaces can never disagree about what the document says.
func (h *cvHandlers) cvJobMatchScore(ctx context.Context, doc cv.Document, tmpl cv.Template, in cvmatch.Input) (cvmatch.Score, error) {
	text, err := h.renderedCVText(ctx, doc, tmpl)
	if err != nil {
		return cvmatch.Score{}, err
	}
	in.CVText = text
	in.CVSkills = skilltag.Parse(text, skilltag.WithResumeAcronyms())
	return cvmatch.Compute(in), nil
}

// cvJobMatchRequirements projects the cached ledger onto the scorer's input, carrying a
// requirement's text and priority — which describe the vacancy and hold for any CV — plus
// the cached status, which is used only where the text names nothing checkable. The
// evidence is deliberately dropped: it cites the base profile, not this document.
func cvJobMatchRequirements(analysis *matchanalysis.Analysis) []cvmatch.Requirement {
	if analysis == nil {
		return nil
	}
	out := make([]cvmatch.Requirement, 0, len(analysis.RequirementMatch))
	for _, r := range analysis.RequirementMatch {
		out = append(out, cvmatch.Requirement{
			Text:         r.Text,
			Priority:     r.Priority,
			CachedStatus: r.Status,
		})
	}
	return out
}

// cvJobMatchUnavailable answers a scoring failure as an absent score rather than an error:
// the workspace this read serves must keep loading and editing when the renderer is missing
// or a compile fails. The reason is a stable short string — the underlying error is logged,
// not served, so a toolchain detail never reaches the client.
func (h *cvHandlers) cvJobMatchUnavailable(c *fiber.Ctx, err error) error {
	reason := "could not read the rendered CV"
	if errors.Is(err, errNoRenderer) {
		reason = "CV rendering is not available"
	} else {
		log.Printf("cv job-match: %v", err)
	}
	return c.JSON(fiber.Map{"data": cvJobMatchResponse{Available: false, Reason: reason}})
}
