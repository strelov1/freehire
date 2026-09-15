package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// windowProbe drives pageParamsWindowed through a real Fiber request, because the thing under
// test is how a QUERY STRING becomes a bound — parsing included. Calling the helper with
// already-parsed ints would test the comparison and skip the half that has bitten before (the
// int32 wrap pageParamsBounded's doc comment argues).
func windowProbe(t *testing.T, query string) (status int, limit, offset int) {
	t.Helper()

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/probe", func(c *fiber.Ctx) error {
		l, o, err := pageParamsWindowed(c, defaultLimit, maxLimit)
		if err != nil {
			return err
		}
		limit, offset = l, o
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe?"+query, nil))
	if err != nil {
		t.Fatalf("probe %q: %v", query, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, limit, offset
}

// TestPageParamsWindowedServesTheLastPageInsideTheWindow pins the boundary from the serving
// side. A window that refuses its own last page would be a silent off-by-one: every caller
// paging to the end would see a 400 where the spec promises a page, and nothing else in the
// suite would notice.
func TestPageParamsWindowedServesTheLastPageInsideTheWindow(t *testing.T) {
	// offset+limit == maxPageWindow exactly: the deepest page the window admits.
	status, limit, offset := windowProbe(t, "limit=100&offset=9900")

	if status != fiber.StatusOK {
		t.Fatalf("offset+limit == maxPageWindow (%d): status = %d, want 200", maxPageWindow, status)
	}
	if limit != 100 || offset != 9900 {
		t.Errorf("limit, offset = %d, %d; want 100, 9900 — the helper must pass the page through unchanged", limit, offset)
	}
}

// TestPageParamsWindowedRefusesOnePastTheWindow is the defect this change exists for, at its
// smallest: one row past the last admissible page.
func TestPageParamsWindowedRefusesOnePastTheWindow(t *testing.T) {
	status, _, _ := windowProbe(t, "limit=100&offset=9901")

	if status != fiber.StatusBadRequest {
		t.Fatalf("offset+limit one past maxPageWindow (%d): status = %d, want 400", maxPageWindow, status)
	}
}

// TestPageParamsWindowedRefusesTheOutageOffset uses the figure production actually served.
//
// A crawler reached offset=179500 on /api/v1/jobs on 2026-09-14 and each such request walked
// ~180,000 heap tuples, pinning a pooled connection for minutes. The constant in the test is
// the incident, not a round number.
func TestPageParamsWindowedRefusesTheOutageOffset(t *testing.T) {
	status, _, _ := windowProbe(t, "limit=100&offset=179500")

	if status != fiber.StatusBadRequest {
		t.Fatalf("the offset that caused the 2026-09-14 outage: status = %d, want 400", status)
	}
}

// TestPageParamsWindowedRefusesAnOffsetPastInt32 keeps the older defect covered through the
// new door. pageParamsBounded clamps such an offset to MaxInt32 so Postgres does not receive a
// wrapped negative; that clamped value is far past the window, so the caller must now see the
// window's 400 — never a 200, and never the 500 the clamp was introduced to prevent.
func TestPageParamsWindowedRefusesAnOffsetPastInt32(t *testing.T) {
	status, _, _ := windowProbe(t, "offset=3000000000")

	if status != fiber.StatusBadRequest {
		t.Fatalf("offset past int32: status = %d, want 400", status)
	}
}

// TestPageParamsWindowedAllowsTheDefaultFirstPage guards against the window being applied so
// eagerly that an ordinary request — no pagination params at all — is refused.
func TestPageParamsWindowedAllowsTheDefaultFirstPage(t *testing.T) {
	status, limit, offset := windowProbe(t, "")

	if status != fiber.StatusOK {
		t.Fatalf("no pagination params: status = %d, want 200", status)
	}
	if limit != defaultLimit || offset != 0 {
		t.Errorf("limit, offset = %d, %d; want %d, 0", limit, offset, defaultLimit)
	}
}
