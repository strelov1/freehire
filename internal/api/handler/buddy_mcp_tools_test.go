package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// A tool's error text reaches the agent verbatim, so a host's broken zone row must not carry
// the host's account id out the way the wrapped error does.
func TestMockToolError_HidesTheHostBehindABrokenZone(t *testing.T) {
	err := mockToolError(context.Background(), fmt.Errorf("%w: member %d has timezone %q", slots.ErrNoOwnerZone, 4242, "Mars/Olympus"))
	var fe *fiber.Error
	if !errors.As(err, &fe) || fe.Code != fiber.StatusInternalServerError {
		t.Fatalf("err = %v, want a 500 fiber.Error", err)
	}
	if strings.Contains(err.Error(), "4242") || strings.Contains(err.Error(), "Mars") {
		t.Fatalf("the tool error carries the host's detail: %q", err.Error())
	}
}

// An explicit from with no to gets a 30-day window after from, never one that ends before it.
func TestSlotWindowBounds_DefaultsToAfterFrom(t *testing.T) {
	from := time.Now().UTC().AddDate(0, 0, 45)
	gotFrom, gotTo := slotWindowBounds(&from, nil)
	if !gotFrom.Equal(from) || !gotTo.Equal(from.AddDate(0, 0, 30)) {
		t.Fatalf("window = %v..%v, want %v..+30d", gotFrom, gotTo, from)
	}
	far := from.AddDate(0, 0, 100)
	if _, to := slotWindowBounds(&from, &far); !to.Equal(from.AddDate(0, 0, maxSlotWindowDays)) {
		t.Fatalf("to = %v, want clamped to %d days", to, maxSlotWindowDays)
	}
}
