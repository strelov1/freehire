package atsapply

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
)

// Drafter drafts a free-text answer for one question, grounded in grounding. ok is false
// when no groundable answer exists — a legitimate outcome, not an error: the model found
// nothing in the candidate's own data to say, so the question still parks rather than
// receiving an invented answer.
type Drafter interface {
	Draft(ctx context.Context, question MergedField, grounding GroundingContext) (answer string, ok bool, err error)
}

// LetterReader reads a candidate's own already-drafted cover letter for one job, when one
// exists. *coverletter.Store satisfies it directly — the same structural fit AtomReader
// already has over *experience.Store — so no adapter type is needed: a nil *Stored means
// "no letter yet", exactly coverletter.Store.Get's own contract, not an error this package
// re-derives.
type LetterReader interface {
	Get(ctx context.Context, userID, jobID int64) (*coverletter.Stored, error)
}

// coverLetterAnswer looks up the candidate's own existing letter for (userID, jobID) and, if
// one exists with a non-blank body, returns it as a ready answer — ok is false when no
// letter has been drafted yet (the ordinary case for a job never drafted for), when the
// stored letter's body is blank (a state that should not occur, but answering a required
// field with an empty string is worse than falling through to the drafter), and when the
// read itself fails, which degrades to "nothing found" here rather than failing the whole
// attempt, the same "a failure to read the grounding source degrades to drafting nothing"
// discipline client.go's own resolve already follows for buildGroundingContext.
func coverLetterAnswer(ctx context.Context, letters LetterReader, userID, jobID int64) (answer string, ok bool) {
	stored, err := letters.Get(ctx, userID, jobID)
	if err != nil {
		log.Printf("atsapply: read cover letter for user %d job %d: %v — falling back to the generic drafter", userID, jobID, err)
		return "", false
	}
	if stored == nil || strings.TrimSpace(stored.Body) == "" {
		return "", false
	}
	return stored.Body, true
}

// answerFor prefers the candidate's own existing cover letter over drafter for a
// cover-letter-semantic FREE-TEXT field, falling through to drafter for everything else
// (including a select/radio the label heuristic happens to match — the letter's prose is
// never one of the platform's own option labels, and matchOption would just park it, where
// the generic drafter at least has a chance of picking a real option) and whenever no
// letter exists yet.
func answerFor(ctx context.Context, f MergedField, drafter Drafter, grounding GroundingContext, letters LetterReader, userID, jobID int64) (answer string, ok bool, err error) {
	if letters != nil && (f.Kind == "text" || f.Kind == "textarea") && isCoverLetterTextField(f) {
		if answer, ok := coverLetterAnswer(ctx, letters, userID, jobID); ok {
			return answer, true, nil
		}
	}
	return drafter.Draft(ctx, f, grounding)
}

// draftable reports whether a field is even a candidate for drafting: required (an
// optional field with no answer is already a fine outcome, nothing to fix), labeled (a
// field with no label — the DOM-only shape reconcile.go's own tests measure, e.g. an
// undeclared EEOC control — can never be VERIFIED non-sensitive, since isSensitiveLabel
// has no text to check; failing closed here is what found and fixed by code review after
// the empty-string case slipped through isSensitiveLabel's Contains checks), a kind a
// free-text or single-choice answer actually fits (never a file — see resolveOne's file
// case — and never a checkbox_group, which resolveOne's own Multi note already scopes
// out), not sensitive, and not a geography/residency question (geography.go) — a
// different reason to park (unverifiable, not off-limits) but the same outcome: never
// reaches the model.
func draftable(f MergedField) bool {
	if !f.Required || f.Label == "" {
		return false
	}
	switch f.Kind {
	case "text", "textarea", "select":
	default:
		return false
	}
	return !isSensitiveLabel(f.Label) && !isGeographyLabel(f.Label)
}

// ResolveWithDrafting runs the deterministic Resolve pass first, then offers the drafter
// exactly the required, non-sensitive, free-text-or-single-choice fields it left unmapped
// — never a field Resolve already answered, never an optional field, never a sensitive
// one. A drafted answer is checked against the field's own offered options exactly as a
// deterministic one is (matchOption): a draft that matches no option still parks.
//
// drafter may be nil (an unconfigured deployment, or a caller that has not wired one in
// yet) — the deterministic Plan is returned unchanged, the same outcome as today.
//
// letters may also be nil (no letter store wired in), in which case every field drafts the
// same way it always has. When it is set, a field draftable's own gates already accept AND
// identified as cover-letter-semantic (isCoverLetterTextField) prefers the candidate's own
// existing letter for (userID, jobID) over the generic drafter — see
// atsapply-cover-letter-reuse's spec. userID/jobID are only ever used for that lookup.
func ResolveWithDrafting(ctx context.Context, fields []MergedField, answers map[string]string, drafter Drafter, grounding GroundingContext, hasApprovedCV bool, letters LetterReader, userID, jobID int64) (Plan, error) {
	plan := Resolve(fields, answers, hasApprovedCV)
	if drafter == nil {
		return plan, nil
	}

	byID := make(map[string]MergedField, len(fields))
	for _, f := range fields {
		byID[f.ID] = f
	}

	stillUnmapped := make([]autoapply.UnmappedField, 0, len(plan.Unmapped))
	for _, u := range plan.Unmapped {
		f, known := byID[u.ID]
		if !known || !draftable(f) {
			stillUnmapped = append(stillUnmapped, u)
			continue
		}

		answer, ok, err := answerFor(ctx, f, drafter, grounding, letters, userID, jobID)
		if err != nil {
			return Plan{}, fmt.Errorf("draft %q: %w", f.ID, err)
		}
		if !ok {
			stillUnmapped = append(stillUnmapped, u)
			continue
		}

		value, matched := matchOption(f, answer)
		if !matched {
			stillUnmapped = append(stillUnmapped, u)
			continue
		}
		plan.Fields = append(plan.Fields, ResolvedField{ID: f.ID, Kind: f.Kind, Multi: f.Multi, Value: value})
	}
	plan.Unmapped = stillUnmapped
	return plan, nil
}
