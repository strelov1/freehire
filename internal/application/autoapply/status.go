package autoapply

// Status is the seven-value candidate-facing status for a live auto-apply attempt, richer
// than the job-detail overlay's three-value derivation (internal/api/handler's
// autoApplyEntryStatus), which is left untouched.
type Status string

const (
	// StatusTailoring means no tailored CV has been produced yet, or one has but its
	// answer preview has not been resolved yet — either way, nothing for the candidate to
	// act on.
	StatusTailoring Status = "tailoring"
	// StatusPendingReview means a tailored CV and a resolved answer preview both exist and
	// await the candidate's decision.
	StatusPendingReview Status = "pending_review"
	// StatusApproved means the candidate approved the attempt and it is queued for
	// unattended submission.
	StatusApproved Status = "approved"
	// StatusBlocked means an unattended submission attempt could not answer a required
	// question — regardless of whether the candidate had approved it.
	StatusBlocked Status = "blocked"
	// StatusDeclined means the candidate declined the tailored CV.
	StatusDeclined Status = "declined"
	// StatusFailed means an unattended submission attempt exhausted its retries.
	StatusFailed Status = "failed"
	// StatusTailorFailed means the tailoring run itself did not produce a CV, distinct from
	// StatusFailed, which is a SUBMISSION that gave up with a CV in hand. Without it a failed
	// tailoring run recorded nothing and the candidate was told preparation was under way
	// indefinitely.
	StatusTailorFailed Status = "tailor_failed"
)

// DerivedFrom is an attempt's own raw state, as DeriveStatus reads it. A struct because
// positional bool parameters give a caller no way to be wrong loudly.
type DerivedFrom struct {
	HasTailoredCV      bool
	HasResolvedPreview bool
	ReviewDecision     string
	Blocked            bool
	Failed             bool
	// TailorFailed is set when the tailoring run itself gave up (auto_apply_queue's own
	// tailor_failed_at, migration 0154).
	TailorFailed bool
}

// DeriveStatus derives the candidate-facing status from an attempt's raw state, mirroring
// internal/api/handler's autoApplyEntryStatus precedence. Declined is checked first:
// DeclineAutoApplyReview also sets blocked_at, so checking blocked/failed first would report
// a candidate's own decline as an operational failure.
//
// A tailoring failure is checked last of the operational states, below an existing tailored
// CV: a later success supersedes an earlier failure. The write path clears the marker on
// success; this does not rely on it.
func DeriveStatus(in DerivedFrom) Status {
	switch {
	case in.ReviewDecision == "declined":
		return StatusDeclined
	case in.Blocked:
		return StatusBlocked
	case in.Failed:
		return StatusFailed
	case in.ReviewDecision == "approved":
		return StatusApproved
	case in.HasTailoredCV && in.HasResolvedPreview:
		return StatusPendingReview
	case in.TailorFailed:
		return StatusTailorFailed
	default:
		return StatusTailoring
	}
}

// ResolvedAttempt is the raw state AssembleReviewInfo derives a status and a candidate-safe
// view from. LastError is deliberately absent: it is an internal diagnostic never meant for
// a candidate.
type ResolvedAttempt struct {
	QueueID         int64
	HasTailoredCV   bool
	ResolvedPreview *ResolvedPreview
	ReviewDecision  string
	Blocked         bool
	Failed          bool
	// TailorFailed is set when the tailoring run gave up without producing a CV.
	TailorFailed bool
	// SubmitUncertain is set when the attempt was dead-lettered because the employer may
	// already hold the application (auto_apply_queue.submit_uncertain_at, migration 0196).
	SubmitUncertain bool
	Unmapped        []UnmappedField
}

// AutoApplyReviewInfo is what a tracked job's own read path surfaces about its live
// auto-apply attempt — serialized directly onto the wire, the same convention
// jobtracking.StageSuggestion already follows for its own optional field.
type AutoApplyReviewInfo struct {
	Status Status `json:"status"`
	// QueueID addresses the attempt for POST /me/auto-apply/:queueId/review; carried on every
	// status so no second read is needed.
	QueueID int64 `json:"queue_id"`
	// ResolvedPreview is set only when Status is StatusPendingReview — the one state
	// where the candidate has a decision to make and needs to see what it covers.
	ResolvedPreview *ResolvedPreview `json:"resolved_preview,omitempty"`
	// Unmapped is set only when Status is StatusBlocked — the specific questions that
	// stopped the attempt, never the internal reason it stopped.
	Unmapped []UnmappedField `json:"unmapped,omitempty"`
	// MayHaveReachedEmployer is set only when Status is StatusFailed and the failure left
	// the employer possibly holding the application — the one failure a retry is refused
	// for, so a surface must not offer one.
	MayHaveReachedEmployer bool `json:"may_have_reached_employer,omitempty"`
}

// AssembleReviewInfo builds the candidate-facing view of a job's live auto-apply attempt,
// or nil when hasAttempt is false (no queue row for this (user, job) pair at all).
func AssembleReviewInfo(hasAttempt bool, a ResolvedAttempt) *AutoApplyReviewInfo {
	if !hasAttempt {
		return nil
	}
	status := DeriveStatus(DerivedFrom{
		HasTailoredCV:      a.HasTailoredCV,
		HasResolvedPreview: a.ResolvedPreview != nil,
		ReviewDecision:     a.ReviewDecision,
		Blocked:            a.Blocked,
		Failed:             a.Failed,
		TailorFailed:       a.TailorFailed,
	})
	info := &AutoApplyReviewInfo{Status: status, QueueID: a.QueueID}
	if status == StatusPendingReview {
		info.ResolvedPreview = a.ResolvedPreview
	}
	if status == StatusBlocked {
		info.Unmapped = a.Unmapped
	}
	if status == StatusFailed {
		info.MayHaveReachedEmployer = a.SubmitUncertain
	}
	return info
}
