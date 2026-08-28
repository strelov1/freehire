package handler

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
)

func TestMapCVErrorListCapIsConflictWithSafeMessage(t *testing.T) {
	// The real error value, not a hand-rebuilt copy of its sentence: this test used to
	// reconstruct the format string, so it agreed with itself no matter what the package did.
	raw := error(&cvedit.ListCapError{Where: "Staff Engineer at Contoso", Max: cv.MaxBullets})

	mapped := mapCVError(raw)
	var fe *fiber.Error
	if !errors.As(mapped, &fe) {
		t.Fatalf("mapCVError = %T %v, want *fiber.Error", mapped, mapped)
	}
	if fe.Code != fiber.StatusConflict {
		t.Fatalf("status = %d, want %d", fe.Code, fiber.StatusConflict)
	}
	want := cvedit.UserListCapMessage(raw)
	if fe.Message != want {
		t.Fatalf("message = %q, want %q", fe.Message, want)
	}
	if want == "" || fe.Message == raw.Error() {
		t.Fatal("HTTP message must be the candidate-safe UserListCapMessage, not the raw ErrListCap")
	}
}
