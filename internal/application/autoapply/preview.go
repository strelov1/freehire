package autoapply

import "context"

// PreviewField is one resolved question/answer pair, shaped for a candidate to read.
type PreviewField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PreviewPending is a required question a preview has no answer for yet.
// WillDraftAtSubmission distinguishes "the real submission will draft one for you" from
// "nothing here can answer this". A preview never calls a Drafter itself (no LLM spend for an
// unapproved attempt); it only reports whether the field is eligible for drafting, by the same
// check the real submission's drafting pass applies.
type PreviewPending struct {
	Label                 string `json:"label"`
	WillDraftAtSubmission bool   `json:"will_draft_at_submission"`
}

// ResolvedPreview is the candidate-facing snapshot of what an unattended submission would
// send: the answers it already has, and the questions it does not. It is computed once,
// before the candidate reviews an attempt, and persisted verbatim, never recomputed when they
// open it.
//
// Defined here rather than in internal/api/atsapply (which computes it) because this package
// may not import atsapply: the api block sits above application in the layering table.
type ResolvedPreview struct {
	Fields  []PreviewField   `json:"fields"`
	Pending []PreviewPending `json:"pending,omitempty"`
}

// PreviewResult is what a PreviewSidecar returns for one attempt.
type PreviewResult struct {
	Preview ResolvedPreview
	// Parked is set when the attempt's form cannot be previewed at all (a captcha-gated
	// provider, an unscannable page): the outcome StatusParked describes for a real submission.
	Parked bool
	Reason string
}

// PreviewSidecar resolves what an unattended submission would currently send, without
// submitting anything and without spending an LLM call. The real implementation is
// internal/api/atsapply's PreviewClient; tests use a fake.
type PreviewSidecar interface {
	Preview(ctx context.Context, c Claimed, answers map[string]string) (PreviewResult, error)
}
