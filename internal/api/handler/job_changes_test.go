package handler

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestChangesCursorRoundTrip(t *testing.T) {
	for _, in := range []changesCursor{{0, 0}, {42, 7}, {1 << 40, 1 << 50}} {
		out, err := decodeChangesCursor(in.encode())
		if err != nil || out != in {
			t.Fatalf("round trip of %+v = %+v, %v", in, out, err)
		}
	}
}

func TestChangesCursorRejectsGarbage(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, in := range map[string]string{
		"not base64":    "!!!",
		"wrong version": enc("v2:1:2"),
		"too few parts": enc("v1:1"),
		"too many":      enc("v1:1:2:3"),
		"not numbers":   enc("v1:a:b"),
		"negative":      enc("v1:-1:2"),
		"empty":         "",
	} {
		if _, err := decodeChangesCursor(in); err == nil {
			t.Errorf("%s: %q was accepted", name, in)
		}
	}
}

// A refused call must cost no database work: the handler here is zero-valued, so reaching
// it past validation would nil-dereference into a 500.
func TestListJobChangesValidation(t *testing.T) {
	app := mountWindowed(jobsRegister)
	valid := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	old := time.Now().Add(-maxChangesLookback - time.Hour).UTC().Format(time.RFC3339)
	for name, q := range map[string]string{
		"neither since nor cursor": "limit=10",
		"unparseable since":        "since=yesterday",
		"since past the cap":       "since=" + old,
		"bad cursor":               "cursor=zzz",
	} {
		if got := windowedGet(t, app, "/api/v1/jobs/changes", q); got != fiber.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, got)
		}
	}
	// Control: a valid call is routed to the handler (500 from the zero value), not captured
	// as a /jobs/:slug lookup or lost to a 404.
	if got := windowedGet(t, app, "/api/v1/jobs/changes", "since="+valid); got == fiber.StatusBadRequest || got == fiber.StatusNotFound {
		t.Errorf("valid since: status %d, want the handler to be reached", got)
	}
}
