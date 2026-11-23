package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

func TestExcludeCompany_HidesRestoresAndMapsErrors(t *testing.T) {
	repo := &fakeProfileRepo{excludeRet: userprofile.Profile{UserID: 1, ExcludedCompanies: []string{"acme"}}}
	app, token := profileApp(t, repo)
	type response struct {
		status int
		body   []byte
	}
	do := func(method, path, body, tok string) response {
		t.Helper()
		var r *http.Request
		if body == "" {
			r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
		} else {
			r = httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
		}
		if tok != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		}
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		payload, err := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("close body: %v", closeErr)
		}
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		return response{status: resp.StatusCode, body: payload}
	}
	const path = "/api/v1/me/profile/excluded-companies"

	if got := do(fiber.MethodPost, path, `{"company_slug":"Acme"}`, "").status; got != fiber.StatusUnauthorized {
		t.Errorf("anonymous POST = %d, want 401", got)
	}
	resp := do(fiber.MethodPost, path, `{"company_slug":" Acme "}`, token)
	if resp.status != fiber.StatusOK || strings.Join(repo.excluded, ",") != "acme" {
		t.Fatalf("POST status=%d slugs=%v, want 200 and the normalized slug", resp.status, repo.excluded)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.Unmarshal(resp.body, &got); err != nil || strings.Join(got.Data.ExcludedCompanies, ",") != "acme" {
		t.Errorf("POST body = %+v err=%v, want the saved profile", got, err)
	}
	if got := do(fiber.MethodPost, path, `{"company_slug":"  "}`, token).status; got != fiber.StatusBadRequest {
		t.Errorf("blank slug = %d, want 400", got)
	}

	repo.excludeErr = userprofile.ErrCompaniesFull
	if got := do(fiber.MethodPost, path, `{"company_slug":"umbrella"}`, token).status; got != fiber.StatusConflict {
		t.Errorf("full list = %d, want 409", got)
	}

	repo.excludeErr = userprofile.ErrConflict
	if got := do(fiber.MethodPost, path, `{"company_slug":"umbrella"}`, token).status; got != fiber.StatusServiceUnavailable {
		t.Errorf("lost race = %d, want 503 (retry), distinct from the 409 of a full list", got)
	}

	repo.excludeErr = nil
	if got := do(fiber.MethodDelete, path+"/acme", "", token).status; got != fiber.StatusOK || strings.Join(repo.unexcluded, ",") != "acme" {
		t.Errorf("DELETE = %d slugs=%v, want 200 and acme", got, repo.unexcluded)
	}
	repo.excludeErr = userprofile.ErrNotFound
	resp = do(fiber.MethodDelete, path+"/acme", "", token)
	if resp.status != fiber.StatusOK || !strings.Contains(string(resp.body), `"data":null`) {
		t.Errorf("DELETE without profile = %d %s, want 200 with null data", resp.status, resp.body)
	}
	if got := do(fiber.MethodDelete, path+"/acme", "", "").status; got != fiber.StatusUnauthorized {
		t.Errorf("anonymous DELETE = %d, want 401", got)
	}
}
