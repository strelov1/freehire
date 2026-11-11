//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A link to a slug that no longer exists still reaches its posting through the slug's
// shortcode, which nothing can move (freehire#3284) — and an ambiguous code reaches
// nothing, because a redirect to somebody else's posting is worse than a 404.
func TestGetJob_ResolvesMovedSlugByCode(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	seed := func(extID, slug string) {
		t.Helper()
		p := db.UpsertJobParams{
			Source: "greenhouse", ExternalID: extID, URL: "https://ex.test/" + extID,
			Title: "Forward Deployed Engineer", Company: "Toloka", CompanySlug: "toloka",
			PublicSlug: slug, Location: "Remote", Description: "x",
		}
		if extID == "collision-224028" || extID == "collision-1908612" {
			p.Source = "lever"
		}
		p.RoleFingerprint = pgtype.Text{String: "fp-" + extID, Valid: true}
		if _, err := q.UpsertJob(ctx, p); err != nil {
			t.Fatalf("seed %s: %v", extID, err)
		}
	}
	seed("1", "forward-deployed-engineer-toloka-cqrc7g26")
	seed("2", "forward-deployed-engineer-toloka-aaaaaaaa")
	seed("3", "staff-engineer-toloka-aaaaaaaa-dup") // not a code suffix: must not count
	seed("4", "data-engineer-acme-aaaaaaaa")

	h := &jobsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountJobsContract(app, h, middleware{})

	get := func(slug string) (int, string) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, "GET", "/api/v1/jobs/"+slug, nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		var out struct {
			Data struct {
				PublicSlug string `json:"public_slug"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.Data.PublicSlug
	}

	if status, slug := get("senior-solution-engineer-eu-remote-toloka-cqrc7g26"); status != fiber.StatusOK ||
		slug != "forward-deployed-engineer-toloka-cqrc7g26" {
		t.Errorf("moved slug: status=%d slug=%q, want 200 and the current slug", status, slug)
	}
	if status, _ := get("old-title-acme-aaaaaaaa"); status != fiber.StatusNotFound {
		t.Errorf("ambiguous code: status=%d, want 404", status)
	}
	// These identities really share their shortcode, so the second posting uses the
	// full identity suffix. Neither current title matches the old links below.
	seed("collision-224028", "current-original-75jwd7n2")
	fallback := normalize.JobSlugWithFullHash("Current fallback", "Acme", "lever", "collision-1908612")
	seed("collision-1908612", fallback)
	for _, request := range []struct{ old, current string }{
		{"old-original-75jwd7n2", "current-original-75jwd7n2"},
		{normalize.JobSlugWithFullHash("Old fallback", "Acme", "lever", "collision-1908612"), fallback},
	} {
		if status, slug := get(request.old); status != fiber.StatusOK || slug != request.current {
			t.Errorf("collision redirect %q: status=%d slug=%q; want %q", request.old, status, slug, request.current)
		}
	}
	if status, slug := get("old-title-" + strings.Repeat("a", 44) + "-75jwd7n2"); status != fiber.StatusOK || slug != "current-original-75jwd7n2" {
		t.Errorf("hash-shaped legacy company: status=%d slug=%q, want original posting", status, slug)
	}
	if status, _ := get("no-such-job-zzzzzzzz"); status != fiber.StatusNotFound {
		t.Errorf("unknown code: status=%d, want 404", status)
	}
}
