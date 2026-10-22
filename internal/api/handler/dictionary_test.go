package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

func dictionaryApp(t *testing.T, write gapWrite) *fiber.App {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &dictionaryHandlers{write: write}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h.register(api, middleware{cookie: auth.RequireAuth(iss, testVersions)})
	return app
}

func dictionaryCookie(t *testing.T) *http.Cookie {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: token}
}

func doDictionary(t *testing.T, app *fiber.App, method, path, body string, cookie *http.Cookie) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	return resp.StatusCode, payload
}

func TestDictionaryRecord(t *testing.T) {
	var calls int
	app := dictionaryApp(t, func(context.Context, int64, string, string, string) (int32, string, error) {
		calls++
		return 4, "pending", nil
	})
	cookie := dictionaryCookie(t)

	code, _ := doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"skill","term":"   "}`, cookie)
	if code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("blank = %d calls %d", code, calls)
	}
	long := strings.Repeat("a", 81)
	code, _ = doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"skill","term":"`+long+`"}`, cookie)
	if code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("long = %d calls %d", code, calls)
	}

	code, _ = doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"skill","term":"underwater basket weaving"}`, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", code)
	}
	if calls != 0 {
		t.Fatal("anonymous write reached the store")
	}

	for _, known := range []string{
		`{"kind":"specialization","term":"Backend"}`,
		`{"kind":"specialization","term":"Senior Backend Engineer"}`,
		`{"kind":"skill","term":"k8s"}`,
	} {
		code, _ = doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", known, cookie)
		if code != http.StatusConflict || calls != 0 {
			t.Fatalf("known term %s = %d calls %d", known, code, calls)
		}
	}

	code, _ = doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"role","term":"underwater basket weaving"}`, cookie)
	if code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("unknown kind = %d calls %d", code, calls)
	}

	code, body := doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"skill","term":"underwater basket weaving"}`, cookie)
	if code != http.StatusOK || calls != 1 {
		t.Fatalf("miss = %d calls %d body %#v", code, calls, body)
	}
	data := body["data"].(map[string]any)
	if data["request_count"] != float64(4) || data["status"] != "pending" {
		t.Fatalf("recorded = %#v", data)
	}

	capped := dictionaryApp(t, func(context.Context, int64, string, string, string) (int32, string, error) {
		return 0, "", errGapCap
	})
	code, _ = doDictionary(t, capped, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"specialization","term":"underwater basket weaving"}`, cookie)
	if code != http.StatusTooManyRequests {
		t.Fatalf("cap = %d", code)
	}
}
