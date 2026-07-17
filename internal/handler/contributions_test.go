package handler

import (
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/contribution"
)

// contributionError maps the contribution sentinels onto HTTP statuses; assert each mapping
// through RenderError (the errorApp/errorStatus helpers live in errors_test.go).
func TestContributionError_Mapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unsupported ATS", contribution.ErrUnsupportedATS, fiber.StatusUnprocessableEntity},
		{"board already tracked", contribution.ErrBoardAlreadyTracked, fiber.StatusConflict},
		{"board already contributed", contribution.ErrBoardAlreadyContributed, fiber.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := errorApp(func(*fiber.Ctx) error { return contributionError(tc.err) })
			if got := errorStatus(t, app); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// The user wire shape carries the contribution points balance so the account UI can show it.
func TestToUserResponse_IncludesPoints(t *testing.T) {
	got := toUserResponse(accounts.User{ID: 1, Email: "a@b.test", Role: "user", Points: 3})
	if got.Points != 3 {
		t.Errorf("points = %d, want 3", got.Points)
	}
}

// A recorded contribution's wire shape exposes the source and board it discovered.
func TestToContributionResponse_Shape(t *testing.T) {
	got := toContributionResponse(contribution.Contribution{
		ID: 9, URL: "https://jobs.ashbyhq.com/blitzy", Source: "ashby",
		Board: "blitzy", Status: "pending",
	})
	if got.Source != "ashby" || got.Board != "blitzy" {
		t.Errorf("response = %+v, want source + board surfaced", got)
	}
}
