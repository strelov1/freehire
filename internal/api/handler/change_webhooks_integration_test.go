//go:build integration

// Integration test for the change webhook subscriptions: auth, API-plan gating, the limits,
// the one-time secret and ownership.
// Run with: go test -tags=integration -run ChangeWebhooks ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

type recordingChangeTester struct {
	urls []string
	err  error
}

func (r *recordingChangeTester) Test(_ context.Context, url, secret string) error {
	if !strings.HasPrefix(secret, "whsec_") {
		return errors.New("tester got no secret")
	}
	r.urls = append(r.urls, url)
	return r.err
}

func TestChangeWebhooks(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	seed := func(email, planCol string) string {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if planCol != "" {
			mustExec(t, pool, `UPDATE users SET `+planCol+` = now() + interval '30 days' WHERE id = $1`, id)
		}
		c, _ := auth.NewIssuer("test-secret", time.Hour).Issue(id, testTokenVersion)
		return c
	}
	apiUser, dataUser, freeUser := seed("api@example.test", "api_until_stripe"), seed("data@example.test", "api_data_until_stripe"), seed("free@example.test", "")

	tester := &recordingChangeTester{}
	h := newChangeWebhookHandlers(db.New(pool), tester)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{key: auth.RequireAuth(auth.NewIssuer("test-secret", time.Hour), testVersions)},
		newContract(contractHandlers{changeWebhookHandlers: h}))

	call := func(cookie, method, path, body string) (int, []byte) {
		req := httptest.NewRequestWithContext(ctx, method, "/api/v1/me/change-webhooks"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body) //nolint:errcheck
		return resp.StatusCode, buf.Bytes()
	}
	type sub struct {
		ID           int64    `json:"id"`
		CompanySlugs []string `json:"company_slugs"`
		Kinds        []string `json:"kinds"`
		Secret       string   `json:"secret"`
	}
	decode := func(b []byte) (s sub) {
		var env struct{ Data sub }
		if err := json.Unmarshal(b, &env); err != nil {
			t.Fatalf("decode %s: %v", b, err)
		}
		return env.Data
	}
	const body = `{"url":"https://mirror.example.com/hook","company_slugs":[" Acme ","acme","beta"],"kinds":["closed"]}`

	t.Run("needs a session or key", func(t *testing.T) {
		if code, _ := call("", fiber.MethodPost, "", body); code != fiber.StatusUnauthorized {
			t.Fatalf("anonymous create = %d", code)
		}
		if code, _ := call("", fiber.MethodGet, "", ""); code != fiber.StatusUnauthorized {
			t.Fatalf("anonymous list = %d", code)
		}
	})
	t.Run("needs an API plan", func(t *testing.T) {
		if code, _ := call(freeUser, fiber.MethodPost, "", body); code != fiber.StatusPaymentRequired {
			t.Fatalf("free create = %d, want 402", code)
		}
		if len(tester.urls) != 0 {
			t.Fatal("a refused account's URL was probed")
		}
	})
	var created sub
	t.Run("api tier creates, normalised, secret shown once", func(t *testing.T) {
		code, b := call(apiUser, fiber.MethodPost, "", body)
		if code != fiber.StatusCreated {
			t.Fatalf("create = %d %s", code, b)
		}
		created = decode(b)
		if len(tester.urls) != 0 {
			t.Fatal("creation probed the endpoint: a verifying receiver has no secret yet")
		}
		if !strings.HasPrefix(created.Secret, "whsec_") || strings.Join(created.CompanySlugs, ",") != "acme,beta" || strings.Join(created.Kinds, ",") != "closed" {
			t.Fatalf("created = %+v", created)
		}
		code, b = call(apiUser, fiber.MethodGet, "", "")
		if code != fiber.StatusOK || strings.Contains(string(b), "secret") || decodeList(t, b) != 1 {
			t.Fatalf("list = %d %s", code, b)
		}
	})
	t.Run("data tier may create; defaults to every kind", func(t *testing.T) {
		code, b := call(dataUser, fiber.MethodPost, "", `{"url":"https://mirror.example.com/other"}`)
		if code != fiber.StatusCreated {
			t.Fatalf("create = %d %s", code, b)
		}
		if s := decode(b); len(s.Kinds) != 3 || len(s.CompanySlugs) != 0 {
			t.Fatalf("defaults = %+v", s)
		}
	})
	t.Run("refuses a bad url and a sixth subscription", func(t *testing.T) {
		if code, _ := call(apiUser, fiber.MethodPost, "", `{"url":"ftp://x.test/h"}`); code != fiber.StatusBadRequest {
			t.Fatalf("ftp url = %d", code)
		}
		for range 4 {
			if code, b := call(apiUser, fiber.MethodPost, "", `{"url":"https://m.example.com/h"}`); code != fiber.StatusCreated {
				t.Fatalf("fill up = %d %s", code, b)
			}
		}
		if code, _ := call(apiUser, fiber.MethodPost, "", `{"url":"https://m.example.com/h"}`); code != fiber.StatusConflict {
			t.Fatalf("sixth = %d, want 409", code)
		}
	})
	idPath := func(id int64) string { return "/" + strconv.FormatInt(id, 10) }
	t.Run("test and delete are the owner's", func(t *testing.T) {
		if code, _ := call(dataUser, fiber.MethodPost, idPath(created.ID)+"/test", ""); code != fiber.StatusNotFound {
			t.Fatalf("another account's test = %d, want 404", code)
		}
		if code, _ := call(dataUser, fiber.MethodDelete, idPath(created.ID), ""); code != fiber.StatusNotFound {
			t.Fatalf("another account's delete = %d, want 404", code)
		}
		if code, _ := call(apiUser, fiber.MethodPost, idPath(created.ID)+"/test", ""); code != fiber.StatusNoContent {
			t.Fatalf("test = %d", code)
		}
		if code, _ := call(apiUser, fiber.MethodDelete, idPath(created.ID), ""); code != fiber.StatusNoContent {
			t.Fatalf("delete = %d", code)
		}
		if code, _ := call(apiUser, fiber.MethodDelete, idPath(created.ID), ""); code != fiber.StatusNotFound {
			t.Fatalf("second delete = %d, want 404", code)
		}
	})
}

func decodeList(t *testing.T, b []byte) int {
	t.Helper()
	var env struct{ Data []json.RawMessage }
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("decode list %s: %v", b, err)
	}
	return len(env.Data)
}
