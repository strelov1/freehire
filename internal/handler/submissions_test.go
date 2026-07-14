package handler

import (
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/submission"
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
