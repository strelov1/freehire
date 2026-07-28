// Unit tests for the inbox filter query-string parsing — the closed
// vocabularies (source, label, link state) and their rejection of unknown
// values. No database: parseInboxFilters is pure request parsing.
package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// parseFilters runs parseInboxFilters against a request built from rawQuery,
// returning the parsed filters and the HTTP status the handler would answer
// (200 when parsing succeeded).
func parseFilters(t *testing.T, rawQuery string) (inboxFilters, int) {
	t.Helper()
	var got inboxFilters
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/f", func(c *fiber.Ctx) error {
		f, err := parseInboxFilters(c)
		if err != nil {
			return err
		}
		got = f
		return c.SendStatus(fiber.StatusOK)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/f?"+rawQuery, nil), -1)
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

// An unknown link state is a 400, not a silently empty listing — the same
// contract the label filter already honours.
func TestParseInboxFilters_UnknownLinkStateRejected(t *testing.T) {
	if _, code := parseFilters(t, "link=bogus"); code != fiber.StatusBadRequest {
		t.Errorf("?link=bogus status = %d, want 400", code)
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
	case !f.IsUnread:
		t.Error("IsUnread = false, want true")
	case !f.IsUnclassified:
		t.Error("IsUnclassified = false, want true")
	case f.Q != "acme":
		t.Errorf("Q = %q, want acme", f.Q)
	}
}
