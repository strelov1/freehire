package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/submission"
)

// submissionError maps the submission sentinels onto HTTP statuses; assert each mapping
// through RenderError (the errorApp/errorStatus helpers live in errors_test.go).
func TestSubmissionError_Mapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", submission.ErrSubmissionNotFound, fiber.StatusNotFound},
		{"duplicate pending", submission.ErrDuplicatePending, fiber.StatusConflict},
		{"already decided", submission.ErrAlreadyDecided, fiber.StatusConflict},
		{"invalid content", fmt.Errorf("%w: url is required", moderation.ErrInvalid), fiber.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := errorApp(func(*fiber.Ctx) error { return submissionError(tc.err) })
			if got := errorStatus(t, app); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// The user wire shape carries role so the SPA can gate moderator-only UI.
func TestToUserResponse_IncludesRole(t *testing.T) {
	got := toUserResponse(accounts.User{ID: 1, Email: "a@b.test", Role: "moderator"})
	if got.Role != "moderator" {
		t.Errorf("role = %q, want moderator", got.Role)
	}
}

// beta_tester is a separate group from role, carried on the wire so the SPA can
// gate the assistant independently of moderator/admin.
func TestToUserResponse_IncludesBetaTester(t *testing.T) {
	got := toUserResponse(accounts.User{ID: 1, Email: "a@b.test", Role: "user", BetaTester: true})
	if !got.BetaTester {
		t.Errorf("beta_tester = %v, want true", got.BetaTester)
	}
	// role and beta_tester are independent — a plain user can be a beta tester.
	if got.Role != "user" {
		t.Errorf("role = %q, want user", got.Role)
	}
}

// onboarding_completed_at rides on the user read because the root layout's gate needs it
// on the same request it already makes; a nil means "never been through the wizard", which
// is what routes the account there.
func TestToUserResponse_IncludesOnboardingCompletedAt(t *testing.T) {
	if got := toUserResponse(accounts.User{ID: 1, Email: "a@b.test", Role: "user"}); got.OnboardingCompletedAt != nil {
		t.Errorf("onboarding_completed_at = %v, want nil for an account that has never onboarded", got.OnboardingCompletedAt)
	}

	done := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	got := toUserResponse(accounts.User{ID: 1, Email: "a@b.test", Role: "user", OnboardingCompletedAt: &done})
	if got.OnboardingCompletedAt == nil || !got.OnboardingCompletedAt.Equal(done) {
		t.Errorf("onboarding_completed_at = %v, want %v", got.OnboardingCompletedAt, done)
	}
}
