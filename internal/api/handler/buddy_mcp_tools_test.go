package handler

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// A tool's error text reaches the agent verbatim, so a host's broken zone row must not carry
// the host's account id out the way the wrapped error does.
func TestMockToolError_HidesTheHostBehindABrokenZone(t *testing.T) {
	err := mockToolError(fmt.Errorf("%w: member %d has timezone %q", slots.ErrNoOwnerZone, 4242, "Mars/Olympus"))
	var fe *fiber.Error
	if !errors.As(err, &fe) || fe.Code != fiber.StatusInternalServerError {
		t.Fatalf("err = %v, want a 500 fiber.Error", err)
	}
	if strings.Contains(err.Error(), "4242") || strings.Contains(err.Error(), "Mars") {
		t.Fatalf("the tool error carries the host's detail: %q", err.Error())
	}
}
