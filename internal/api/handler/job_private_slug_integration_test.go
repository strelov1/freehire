//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func seedPrivateSlugJob(t *testing.T, pool *pgxpool.Pool, ownerID int64) string {
	t.Helper()
	const slug = "private-role-acme-zzzz9999"
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, description, public_slug, created_by, is_private)
		 VALUES ('pasted', 'private:acme:1', '', 'Private Role', 'Acme', 'Confidential body', $1, $2, true)`,
		slug, ownerID); err != nil {
		t.Fatalf("seed private job: %v", err)
	}
	return slug
}

func TestPrivateSlugIs404ExceptForItsCreator(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	var ownerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('owner@example.test') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	slug := seedPrivateSlugJob(t, pool, ownerID)

	h := &jobsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// ?as=<id> stands in for the signed-in caller the optional-auth middleware attaches.
	mountJobsContract(app, h, middleware{optional: func(c *fiber.Ctx) error {
		if id, err := strconv.Atoi(c.Query("as")); err == nil {
			c.Locals(auth.LocalsUserID, int64(id))
		}
		return c.Next()
	}})

	get := func(path string) int {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body)
		return resp.StatusCode
	}

	if code := get("/api/v1/jobs/" + slug); code != fiber.StatusNotFound {
		t.Errorf("anonymous GetJob status = %d, want 404", code)
	}
	if code := get("/api/v1/jobs/" + slug + "/copies"); code != fiber.StatusNotFound {
		t.Errorf("copies status = %d, want 404", code)
	}
	if code := get("/api/v1/jobs/" + slug + "/apply-form"); code != fiber.StatusNotFound {
		t.Errorf("apply-form status = %d, want 404", code)
	}
	if code := get("/api/v1/jobs/" + slug + "?as=999"); code != fiber.StatusNotFound {
		t.Errorf("stranger GetJob status = %d, want 404", code)
	}

	resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, fmt.Sprintf("/api/v1/jobs/%s?as=%d", slug, ownerID), nil))
	if err != nil {
		t.Fatalf("owner request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("owner GetJob status = %d, want 200 (%s)", resp.StatusCode, body)
	}
	var out struct {
		Data struct {
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Description != "Confidential body" {
		t.Errorf("owner description = %q", out.Data.Description)
	}
}
