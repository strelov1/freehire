package atsapply

// PreviewField is one resolved question/answer pair, shaped for a candidate to read —
// deliberately not ResolvedField, which carries option ids and Kind/Multi flags nobody
// outside this package needs.
type PreviewField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PreviewPending is a required question the preview has no answer for yet.
// WillDraftAtSubmission distinguishes "the real submission will draft one for you" from
// "nothing here can answer this" — PreviewAnswers never calls a Drafter itself (no LLM
// spend for an attempt the candidate has not approved yet); it only reports whether the
// field is ELIGIBLE for drafting, the same eligibility check ResolveWithDrafting applies.
type PreviewPending struct {
	Label                 string `json:"label"`
	WillDraftAtSubmission bool   `json:"will_draft_at_submission"`
}

// ResolvedPreview is the candidate-facing snapshot of what an unattended submission would
// send: the answers it already has, and the questions it does not (openspec/changes/
// auto-apply-review-tracking). It is computed once, before the candidate reviews an
// attempt, and persisted verbatim — never recomputed or approximated when they open it.
type ResolvedPreview struct {
	Fields  []PreviewField   `json:"fields"`
	Pending []PreviewPending `json:"pending,omitempty"`
}

// PreviewAnswers runs the same deterministic resolution the real submission starts from
// (Resolve, not ResolveWithDrafting — drafting is deliberately excluded here, see
// ResolvedPreview's doc comment) and shapes the result for display.
//
// The résumé file field is omitted rather than shown with an empty value: resolveOne
// leaves Value blank for it deliberately (Client.Submit renders the tailored CV only at
// submit time), and the drawer surfaces the tailored CV as its own reference alongside
// this preview, not as a row inside it. Any OTHER file field (a cover letter, for
// instance) still has no resolution path and is reported like any other pending field.
func PreviewAnswers(fields []MergedField, answers map[string]string, hasApprovedCV bool) ResolvedPreview {
	plan := Resolve(fields, answers, hasApprovedCV)
	byID := make(map[string]MergedField, len(fields))
	for _, f := range fields {
		byID[f.ID] = f
	}

	preview := ResolvedPreview{Fields: make([]PreviewField, 0, len(plan.Fields))}
	for _, resolved := range plan.Fields {
		f := byID[resolved.ID]
		if f.Kind == "file" {
			continue
		}
		label := f.Label
		if label == "" {
			label = f.ID
		}
		preview.Fields = append(preview.Fields, PreviewField{Label: label, Value: resolved.Value})
	}

	for _, u := range plan.Unmapped {
		label := u.Label
		if label == "" {
			label = u.ID
		}
		preview.Pending = append(preview.Pending, PreviewPending{
			Label:                 label,
			WillDraftAtSubmission: draftable(byID[u.ID]),
		})
	}
	return preview
}
