package handler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cvedit"
)

func TestMapCVErrorProjectLooksLikeJobIsUnprocessableWithItsOwnMessage(t *testing.T) {
	raw := fmt.Errorf("%w: project Acme Corp. Job roles with a start and end date belong "+
		"under experience[], not projects[]", cvedit.ErrProjectLooksLikeJob)

	mapped := mapCVError(raw)
	var fe *fiber.Error
	if !errors.As(mapped, &fe) {
		t.Fatalf("mapCVError = %T %v, want *fiber.Error", mapped, mapped)
	}
	if fe.Code != fiber.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", fe.Code, fiber.StatusUnprocessableEntity)
	}
	if fe.Message != raw.Error() {
		t.Fatalf("message = %q, want the error's own sentence %q", fe.Message, raw.Error())
	}
}
