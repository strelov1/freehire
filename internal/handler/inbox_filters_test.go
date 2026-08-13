// Unit tests for the inbox filter query-string parsing and the closed
// vocabularies (source, label, link state) it is checked against. No database:
// parsing is pure, and the vocabulary check is the service's pure Validate.
package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/inbox"
)

// parseFilters runs the listing's query parsing and validation against a request
// built from rawQuery, returning the parsed filters and the HTTP status the
// handler would answer (200 when both succeeded).
func parseFilters(t *testing.T, rawQuery string) (inbox.Query, int) {
	t.Helper()
	var got inbox.Query
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/f", func(c *fiber.Ctx) error {
		q := parseInboxQuery(c)
		if err := q.Validate(); err != nil {
			return err
		}
		got = q
		return c.SendStatus(fiber.StatusOK)
	})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/f?"+rawQuery, nil), -1)
	if err != nil {
		t.Fatalf("request ?%s: %v", rawQuery, err)
	}
	defer resp.Body.Close()
	return got, resp.StatusCode
}

// Each of the three link states parses to itself, and an absent filter parses to
// the empty string meaning "no link filter".
func TestParseInboxFilters_LinkState(t *testing.T) {
	for _, want := range []string{"linked", "suggested", "unlinked"} {
		f, code := parseFilters(t, "link="+want)
		if code != fiber.StatusOK {
			t.Errorf("?link=%s status = %d, want 200", want, code)
		}
		if f.Link != want {
			t.Errorf("?link=%s parsed Link = %q, want %q", want, f.Link, want)
		}
	}
	f, code := parseFilters(t, "")
	if code != fiber.StatusOK || f.Link != "" {
		t.Errorf("no link filter = (%q, %d), want (\"\", 200)", f.Link, code)
	}
}

// A value outside a vocabulary is a 400, not a silently empty listing. The service
// makes that call once, so the HTTP surface and the assistant's tools refuse the
// same values.
func TestParseInboxFilters_UnknownValuesRejected(t *testing.T) {
	for _, raw := range []string{"link=bogus", "source=imap", "status=ghosted"} {
		if _, code := parseFilters(t, raw); code != fiber.StatusBadRequest {
			t.Errorf("?%s status = %d, want 400", raw, code)
		}
	}
}

// The link filter composes with the other filters rather than replacing them:
// parsing one must not disturb the rest.
func TestParseInboxFilters_LinkComposesWithOthers(t *testing.T) {
	f, code := parseFilters(t, "link=unlinked&status=rejection&source=gmail&unread=1&unclassified=1&q=acme")
	if code != fiber.StatusOK {
		t.Fatalf("composed filters status = %d, want 200", code)
	}
	switch {
	case f.Link != "unlinked":
		t.Errorf("Link = %q, want unlinked", f.Link)
	case f.Status != "rejection":
		t.Errorf("Status = %q, want rejection", f.Status)
	case f.Source != "gmail":
		t.Errorf("Source = %q, want gmail", f.Source)
	case !f.Unread:
		t.Error("Unread = false, want true")
	case !f.Unclassified:
		t.Error("Unclassified = false, want true")
	case f.Q != "acme":
		t.Errorf("Q = %q, want acme", f.Q)
	}
}
