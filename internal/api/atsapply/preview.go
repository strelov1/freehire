package atsapply

import "github.com/strelov1/freehire/internal/application/autoapply"

// PreviewAnswers runs the same deterministic resolution the real submission starts from
// (Resolve, not ResolveWithDrafting — drafting is deliberately excluded here, see
// autoapply.ResolvedPreview's doc comment) and shapes the result for display.
//
// The résumé file field is omitted rather than shown with an empty value: resolveOne
// leaves Value blank for it deliberately (Client.Submit renders the tailored CV only at
// submit time), and the drawer surfaces the tailored CV as its own reference alongside
// this preview, not as a row inside it. Any OTHER file field (a cover letter, for
// instance) still has no resolution path and is reported like any other pending field.
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
		label := f.Label
		if label == "" {
			label = f.ID
		}
		preview.Fields = append(preview.Fields, autoapply.PreviewField{Label: label, Value: resolved.Value})
	}

	for _, u := range plan.Unmapped {
		label := u.Label
		if label == "" {
			label = u.ID
		}
		preview.Pending = append(preview.Pending, autoapply.PreviewPending{
			Label:                 label,
			WillDraftAtSubmission: draftable(byID[u.ID]),
		})
	}
	return preview
}
