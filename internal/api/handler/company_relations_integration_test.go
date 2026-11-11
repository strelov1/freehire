//go:build integration

// The company-correction flow over HTTP: a person proposes, a moderator approves, and the
// company page then names the parent and the subsidiary (freehire#3895). Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/companyrelation"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestCompanyRelationProposeApproveAndShow(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('relations@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, job_count) VALUES
		   ('novartis', 'Novartis', 40), ('novartis-slovenija', 'Novartis Slovenija', 3)`); err != nil {
		t.Fatalf("seed companies: %v", err)
	}

	h := &companyRelationHandlers{relations: companyrelation.New(pool)}
	companies := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	signedIn := func(c *fiber.Ctx) error { c.Locals(auth.LocalsUserID, userID); return c.Next() }
	h.register(app.Group("/api/v1"), middleware{
		cookie: signedIn, moderator: func(c *fiber.Ctx) error { return c.Next() },
	}, newContract(contractHandlers{companyRelationHandlers: h}))
	app.Get("/api/v1/companies/:slug", companies.GetCompany)

	do := func(t *testing.T, method, url, body string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, url, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, url, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}

	status, body := do(t, "POST", "/api/v1/companies/novartis-slovenija/relations",
		`{"kind":"subsidiary","target_slug":"novartis","note":"local entity"}`)
	if status != fiber.StatusCreated {
		t.Fatalf("propose: status %d, body %s", status, body)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Data.ID == 0 {
		t.Fatalf("propose body %s: %v", body, err)
	}

	status, body = do(t, "POST", "/api/v1/companies/novartis-slovenija/relations",
		`{"kind":"subsidiary","target_slug":"novartis"}`)
	if status != fiber.StatusConflict {
		t.Errorf("duplicate proposal: status %d, want 409 (%s)", status, body)
	}
	status, _ = do(t, "POST", "/api/v1/companies/novartis-slovenija/relations", `{"kind":"merge","target_slug":"novartis"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("unknown kind: status %d, want 400", status)
	}

	status, body = do(t, "GET", "/api/v1/company-relations", "")
	if status != fiber.StatusOK || !strings.Contains(string(body), `"target_name":"Novartis"`) {
		t.Fatalf("queue: status %d, body %s", status, body)
	}

	if status, body = do(t, "POST", "/api/v1/company-relations/"+strconv.FormatInt(created.Data.ID, 10)+"/approve", ""); status != fiber.StatusOK {
		t.Fatalf("approve: status %d, body %s", status, body)
	}

	status, body = do(t, "GET", "/api/v1/companies/novartis-slovenija", "")
	if status != fiber.StatusOK || !strings.Contains(string(body), `"parent":{"slug":"novartis","name":"Novartis","job_count":40}`) {
		t.Errorf("subsidiary page: status %d, body %s", status, body)
	}
	status, body = do(t, "GET", "/api/v1/companies/novartis", "")
	if status != fiber.StatusOK || !strings.Contains(string(body), `"subsidiaries":[{"slug":"novartis-slovenija"`) {
		t.Errorf("parent page: status %d, body %s", status, body)
	}
}
