package autoapply

// Status is the seven-value candidate-facing status for a live auto-apply attempt, richer
// than the job-detail overlay's own three-value derivation (which has its own consumers
// and its own reasons — see internal/api/handler's autoApplyEntryStatus doc comment — and
// is left untouched by this).
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
	// StatusTailorFailed means the tailoring run itself did not produce a CV — distinct
	// from StatusFailed, which is a SUBMISSION that gave up with a tailored CV in hand.
	// The two are different things to be told: one attempt has a CV the candidate can
	// look at, the other has nothing at all.
	//
	// Before it existed, a failed tailoring run recorded nothing, so the derivation below
	// fell through to StatusTailoring and the candidate was told preparation was under way
	// — indefinitely, about work nobody was doing (freehire, 2026-09-08: three entries,
	// one of them for over a day).
	StatusTailorFailed Status = "tailor_failed"
)

// DerivedFrom is an attempt's own raw state, as DeriveStatus reads it.
//
// A struct rather than the positional parameters this replaces: they were five in a row,
// four of them bool, and a caller had no way to be wrong loudly — the state that made this
// type worth having would have been the sixth.
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

// DeriveStatus derives the candidate-facing status from an attempt's own raw state. Mirrors
// internal/api/handler's autoApplyEntryStatus precedence: declined is checked first,
// because DeclineAutoApplyReview also sets blocked_at (it reuses MarkAutoApplyBlocked's own
// park vocabulary) — checking blocked/failed before the review decision would misreport a
// candidate's own decline as an operational failure.
//
// A tailoring failure is checked LAST of the operational states, below a tailored CV that
// exists: a later run that succeeded supersedes an earlier one that did not, and telling
// somebody their CV could not be prepared while it sits there ready is worse than saying
// nothing. The write path clears the marker on success; this does not rely on it having.
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
// view from. LastError is deliberately not a field here — it is an internal diagnostic
// string never intended for a candidate to read, so the type that carries what a tracked
// job reports has no place to put it.
type ResolvedAttempt struct {
	QueueID         int64
	HasTailoredCV   bool
	ResolvedPreview *ResolvedPreview
	ReviewDecision  string
	Blocked         bool
	Failed          bool
	// TailorFailed is set when the tailoring run gave up without producing a CV.
	TailorFailed bool
	Unmapped     []UnmappedField
}

// AutoApplyReviewInfo is what a tracked job's own read path surfaces about its live
// auto-apply attempt — serialized directly onto the wire, the same convention
// jobtracking.StageSuggestion already follows for its own optional field.
type AutoApplyReviewInfo struct {
	Status Status `json:"status"`
	// QueueID addresses the attempt for the existing POST /me/auto-apply/:queueId/review
	// call the drawer's approve/decline banner makes — carried on every status, not just
	// pending_review, so it costs nothing to include and never needs a second read later.
	QueueID int64 `json:"queue_id"`
	// ResolvedPreview is set only when Status is StatusPendingReview — the one state
	// where the candidate has a decision to make and needs to see what it covers.
	ResolvedPreview *ResolvedPreview `json:"resolved_preview,omitempty"`
	// Unmapped is set only when Status is StatusBlocked — the specific questions that
	// stopped the attempt, never the internal reason it stopped.
	Unmapped []UnmappedField `json:"unmapped,omitempty"`
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
	return info
}
