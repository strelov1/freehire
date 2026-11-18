//go:build integration

package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

// TestJobCopies_ExactShape pins the wire form the contract must keep: an always-present
// posted_at (null when unknown), the apply URL, and meta carrying only the total.
func TestJobCopies_ExactShape(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	truncateApplyFormJobs(t, pool)
	seedApplyFormJob(t, q, "acme:1", "staff-engineer-acme-1")

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountJobsContract(app, &jobsHandlers{queries: q}, middleware{})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/jobs/staff-engineer-acme-1/copies", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)
	want := `{"data":[{"public_slug":"staff-engineer-acme-1","location":"Remote","apply_url":"https://ex.test/acme:1?utm_source=freehire.me","posted_at":null}],"meta":{"total":1}}`
	if resp.StatusCode != fiber.StatusOK || body != want {
		t.Fatalf("status %d, body %s\nwant %s", resp.StatusCode, body, want)
	}
}
