package handler

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/savedsearch"
)

// Every saved-search sentinel the service can return to a handler must map to the
// status its own documentation promises. One that falls through reaches RenderError,
// which has no name for it: the caller gets "internal server error" for something
// they can fix, and the error inbox gets a fault report for ordinary traffic. That is
// how ErrQueryTooLong — documented "(mapped to 400)" and mapped nowhere — reached
// production as a 500.
func TestSavedSearchError_MapsEverySentinel(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid name", savedsearch.ErrInvalidName, fiber.StatusBadRequest},
		{"invalid author label", savedsearch.ErrInvalidAuthorLabel, fiber.StatusBadRequest},
		{"query too long", savedsearch.ErrQueryTooLong, fiber.StatusBadRequest},
		{"duplicate name", savedsearch.ErrDuplicateName, fiber.StatusConflict},
		{"cap exceeded", savedsearch.ErrCapExceeded, fiber.StatusConflict},
		{"profile search exists", savedsearch.ErrProfileSearchExists, fiber.StatusConflict},
		{"not found", savedsearch.ErrNotFound, fiber.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var fe *fiber.Error
			if !errors.As(savedSearchError(tc.err), &fe) {
				t.Fatalf("savedSearchError(%v) did not map to a *fiber.Error; it would render as a 500", tc.err)
			}
			if fe.Code != tc.want {
				t.Errorf("status = %d, want %d", fe.Code, tc.want)
			}
		})
	}
}
