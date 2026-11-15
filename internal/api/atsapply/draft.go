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
// when nothing groundable exists; that is not an error, the question parks instead of
// getting an invented answer.
type Drafter interface {
	Draft(ctx context.Context, question MergedField, grounding GroundingContext) (answer string, ok bool, err error)
}

// LetterReader reads a candidate's already-drafted cover letter for one job.
// *coverletter.Store satisfies it; a nil *Stored means no letter yet, as in Store.Get.
type LetterReader interface {
	Get(ctx context.Context, userID, jobID int64) (*coverletter.Stored, error)
}

// coverLetterAnswer returns the candidate's existing letter for (userID, jobID) as a ready
// answer. ok is false when there is none, its body is blank, or the read fails (that
// degrades to drafting, like buildGroundingContext).
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

// answerFor prefers the candidate's existing cover letter over drafter for a
// cover-letter-semantic free-text field, and falls through to drafter otherwise. A
// select/radio the label heuristic matches also falls through: letter prose never equals
// an option label.
func answerFor(ctx context.Context, f MergedField, drafter Drafter, grounding GroundingContext, letters LetterReader, userID, jobID int64) (answer string, ok bool, err error) {
	if letters != nil && (f.Kind == "text" || f.Kind == "textarea") && isCoverLetterTextField(f) {
		if answer, ok := coverLetterAnswer(ctx, letters, userID, jobID); ok {
			return answer, true, nil
		}
	}
	return drafter.Draft(ctx, f, grounding)
}

// draftable reports whether a field may be drafted: required, labeled (an unlabeled field
// cannot be verified non-sensitive, so it fails closed), a free-text or single-choice kind
// (never file or checkbox_group), not sensitive, and not a geography question
// (geography.go), which parks as unverifiable.
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

// ResolveWithDrafting runs the deterministic Resolve pass, then offers drafter the
// required, non-sensitive, free-text-or-single-choice fields it left unmapped. A drafted
// answer is checked against the field's options like a deterministic one (matchOption);
// no match still parks.
//
// A nil drafter returns the deterministic Plan unchanged. A non-nil letters makes
// cover-letter fields prefer the candidate's existing letter for (userID, jobID);
// userID/jobID are used only for that lookup.
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
