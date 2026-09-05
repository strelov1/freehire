package autoapply

import "testing"

func TestDeriveStatus_SixStates(t *testing.T) {
	cases := []struct {
		name                              string
		hasTailoredCV, hasResolvedPreview bool
		reviewDecision                    string
		blocked, failed                   bool
		want                              Status
	}{
		{"no tailored CV yet", false, false, "", false, false, StatusTailoring},
		{"tailored but no preview yet", true, false, "", false, false, StatusTailoring},
		{"tailored and previewed, undecided", true, true, "", false, false, StatusPendingReview},
		{"approved, healthy", true, true, "approved", false, false, StatusApproved},
		{"approved but blocked on a form field", true, true, "approved", true, false, StatusBlocked},
		{"approved but dead-lettered", true, true, "approved", false, true, StatusFailed},
		{"declined", true, true, "declined", false, false, StatusDeclined},
		// Declining also sets blocked_at (DeclineAutoApplyReview reuses MarkAutoApplyBlocked's
		// own park vocabulary) — declined must still win, not read as an operational failure.
		{"declined entry still carries blocked_at internally", true, true, "declined", true, false, StatusDeclined},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeriveStatus(c.hasTailoredCV, c.hasResolvedPreview, c.reviewDecision, c.blocked, c.failed)
			if got != c.want {
				t.Errorf("DeriveStatus(%v, %v, %q, %v, %v) = %q, want %q",
					c.hasTailoredCV, c.hasResolvedPreview, c.reviewDecision, c.blocked, c.failed, got, c.want)
			}
		})
	}
}

func TestAssembleReviewInfo_NoAttemptIsNil(t *testing.T) {
	if got := AssembleReviewInfo(false, ResolvedAttempt{}); got != nil {
		t.Errorf("AssembleReviewInfo(false, ...) = %+v, want nil", got)
	}
}

func TestAssembleReviewInfo_PendingReviewCarriesThePreview(t *testing.T) {
	preview := &ResolvedPreview{Fields: []PreviewField{{Label: "First name", Value: "Ada"}}}
	got := AssembleReviewInfo(true, ResolvedAttempt{
		HasTailoredCV: true, ResolvedPreview: preview,
	})
	if got == nil || got.Status != StatusPendingReview {
		t.Fatalf("got = %+v, want pending_review", got)
	}
	if got.ResolvedPreview != preview {
		t.Errorf("ResolvedPreview = %v, want the same preview carried through", got.ResolvedPreview)
	}
	if got.Unmapped != nil {
		t.Errorf("Unmapped = %v, want nil for a pending_review attempt", got.Unmapped)
	}
}

func TestAssembleReviewInfo_BlockedCarriesUnmappedNeverLastError(t *testing.T) {
	unmapped := []UnmappedField{{ID: "why_us", Label: "Why us?", Required: true, Reason: "no known answer"}}
	got := AssembleReviewInfo(true, ResolvedAttempt{
		HasTailoredCV: true, Blocked: true, Unmapped: unmapped,
	})
	if got == nil || got.Status != StatusBlocked {
		t.Fatalf("got = %+v, want blocked", got)
	}
	if len(got.Unmapped) != 1 || got.Unmapped[0].ID != "why_us" {
		t.Errorf("Unmapped = %+v, want the one blocking field", got.Unmapped)
	}
	if got.ResolvedPreview != nil {
		t.Errorf("ResolvedPreview = %v, want nil for a blocked attempt", got.ResolvedPreview)
	}
}

func TestAssembleReviewInfo_DeclinedCarriesNeitherPreviewNorUnmapped(t *testing.T) {
	preview := &ResolvedPreview{Fields: []PreviewField{{Label: "First name", Value: "Ada"}}}
	got := AssembleReviewInfo(true, ResolvedAttempt{
		HasTailoredCV: true, ReviewDecision: "declined", Blocked: true, ResolvedPreview: preview,
	})
	if got == nil || got.Status != StatusDeclined {
		t.Fatalf("got = %+v, want declined", got)
	}
	if got.ResolvedPreview != nil || got.Unmapped != nil {
		t.Errorf("got = %+v, want neither preview nor unmapped surfaced for a declined attempt", got)
	}
}
