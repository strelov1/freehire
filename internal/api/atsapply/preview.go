package atsapply

import "github.com/strelov1/freehire/internal/application/autoapply"

// PreviewAnswers runs the same deterministic resolution a real submission starts from
// (Resolve, not ResolveWithDrafting; see autoapply.ResolvedPreview) and shapes it for
// display.
//
// The résumé file field is omitted: resolveOne leaves its Value blank (the tailored CV is
// rendered only at submit time) and the drawer shows the CV separately. Any other file
// field (a cover letter) has no resolution path and is reported as pending.
func PreviewAnswers(fields []MergedField, answers map[string]string, hasApprovedCV bool) autoapply.ResolvedPreview {
	plan := Resolve(fields, answers, hasApprovedCV)
	byID := make(map[string]MergedField, len(fields))
	for _, f := range fields {
		byID[f.ID] = f
	}

	preview := autoapply.ResolvedPreview{Fields: make([]autoapply.PreviewField, 0, len(plan.Fields))}
	for _, resolved := range plan.Fields {
		f := byID[resolved.ID]
		if f.Kind == "file" {
			continue
		}
		preview.Fields = append(preview.Fields, autoapply.PreviewField{
			Label: questionText(f), Value: resolved.Value,
		})
	}

	// questionText, not an inline label-or-id: what this screen titles a question is what the
	// candidate answers and what is banked under its topic, so this and matchBankAnswerKey
	// must share one function.
	for _, u := range plan.Unmapped {
		f := byID[u.ID]
		preview.Pending = append(preview.Pending, autoapply.PreviewPending{
			Label:                 questionText(f),
			WillDraftAtSubmission: draftable(f),
		})
	}
	return preview
}
