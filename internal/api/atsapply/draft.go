package atsapply

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/application/autoapply"
)

// Drafter drafts a free-text answer for one question, grounded in grounding. ok is false
// when no groundable answer exists — a legitimate outcome, not an error: the model found
// nothing in the candidate's own data to say, so the question still parks rather than
// receiving an invented answer.
type Drafter interface {
	Draft(ctx context.Context, question MergedField, grounding GroundingContext) (answer string, ok bool, err error)
}

// draftable reports whether a field is even a candidate for drafting: required (an
// optional field with no answer is already a fine outcome, nothing to fix), labeled (a
// field with no label — the DOM-only shape reconcile.go's own tests measure, e.g. an
// undeclared EEOC control — can never be VERIFIED non-sensitive, since isSensitiveLabel
// has no text to check; failing closed here is what found and fixed by code review after
// the empty-string case slipped through isSensitiveLabel's Contains checks), a kind a
// free-text or single-choice answer actually fits (never a file — see resolveOne's file
// case — and never a checkbox_group, which resolveOne's own Multi note already scopes
// out), and not sensitive.
func draftable(f MergedField) bool {
	if !f.Required || f.Label == "" {
		return false
	}
	switch f.Kind {
	case "text", "textarea", "select":
	default:
		return false
	}
	return !isSensitiveLabel(f.Label)
}

// ResolveWithDrafting runs the deterministic Resolve pass first, then offers the drafter
// exactly the required, non-sensitive, free-text-or-single-choice fields it left unmapped
// — never a field Resolve already answered, never an optional field, never a sensitive
// one. A drafted answer is checked against the field's own offered options exactly as a
// deterministic one is (matchOption): a draft that matches no option still parks.
//
// drafter may be nil (an unconfigured deployment, or a caller that has not wired one in
// yet) — the deterministic Plan is returned unchanged, the same outcome as today.
func ResolveWithDrafting(ctx context.Context, fields []MergedField, answers map[string]string, drafter Drafter, grounding GroundingContext, hasApprovedCV bool) (Plan, error) {
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

		answer, ok, err := drafter.Draft(ctx, f, grounding)
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
