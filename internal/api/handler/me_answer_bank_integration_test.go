//go:build integration

// Integration tests for the answer bank's own routes. Run with:
//
//	go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// newAnswerBankApp wires only the answer bank's own three routes over a real Postgres pool.
func newAnswerBankApp(pool *pgxpool.Pool, iss *auth.Issuer) (*fiber.App, *answerBankHandlers) {
	queries := db.New(pool)
	h := &answerBankHandlers{bank: answerbank.NewStore(answerbank.NewQueriesRepository(queries))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mw := middleware{
		key: auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}
	h.register(api, mw)
	return app, h
}

func answerBankUser(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, email string) (int64, string) {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	token, err := iss.Issue(id, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return id, token
}

func answerBankRequest(t *testing.T, app *fiber.App, method, path, cookie string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	req.Header.Set(fiber.HeaderContentType, "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	return resp
}

// Saving an answer, then reading it back, is the whole loop the review screen performs.
func TestAnswerBank_SavedAnswerIsListedBack(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAnswerBankApp(pool, iss)
	_, cookie := answerBankUser(t, pool, iss, "bank@example.test")

	save := answerBankRequest(t, app, fiber.MethodPut, "/api/v1/me/answer-bank", cookie,
		map[string]string{"question": "Which state do you currently reside in?", "answer": "Santa Catarina"})
	defer save.Body.Close()
	if save.StatusCode != fiber.StatusOK {
		t.Fatalf("save status = %d, want 200", save.StatusCode)
	}

	list := answerBankRequest(t, app, fiber.MethodGet, "/api/v1/me/answer-bank", cookie, nil)
	defer list.Body.Close()
	if list.StatusCode != fiber.StatusOK {
		t.Fatalf("list status = %d, want 200", list.StatusCode)
	}
	var out struct {
		Data []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"data"`
	}
	decodeJSON(t, list, &out)
	if len(out.Data) != 1 || out.Data[0].Answer != "Santa Catarina" {
		t.Fatalf("data = %+v, want the saved answer", out.Data)
	}
}

// The same question worded differently updates the answer rather than adding a second one.
func TestAnswerBank_ARephrasedQuestionUpdatesRatherThanDuplicates(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAnswerBankApp(pool, iss)
	_, cookie := answerBankUser(t, pool, iss, "rephrase@example.test")

	for _, body := range []map[string]string{
		{"question": "What is your desired salary?", "answer": "5000 USD per year"},
		{"question": "Salary expectations", "answer": "6000 USD per year"},
	} {
		resp := answerBankRequest(t, app, fiber.MethodPut, "/api/v1/me/answer-bank", cookie, body)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("save status = %d, want 200", resp.StatusCode)
		}
	}

	list := answerBankRequest(t, app, fiber.MethodGet, "/api/v1/me/answer-bank", cookie, nil)
	defer list.Body.Close()
	var out struct {
		Data []struct {
			Answer string `json:"answer"`
		} `json:"data"`
	}
	decodeJSON(t, list, &out)
	if len(out.Data) != 1 {
		t.Fatalf("bank holds %d answers, want 1 — the two phrasings are one question", len(out.Data))
	}
	if out.Data[0].Answer != "6000 USD per year" {
		t.Errorf("answer = %q, want the newer one", out.Data[0].Answer)
	}
}

// Another candidate's answer is reported missing, never forbidden — the same posture every
// other owned resource here takes, so a probing caller learns nothing.
func TestAnswerBank_AForeignAnswerIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAnswerBankApp(pool, iss)
	_, ownerCookie := answerBankUser(t, pool, iss, "owner@example.test")
	_, otherCookie := answerBankUser(t, pool, iss, "other@example.test")

	save := answerBankRequest(t, app, fiber.MethodPut, "/api/v1/me/answer-bank", ownerCookie,
		map[string]string{"question": "Which state?", "answer": "SC"})
	save.Body.Close()

	list := answerBankRequest(t, app, fiber.MethodGet, "/api/v1/me/answer-bank", ownerCookie, nil)
	defer list.Body.Close()
	var out struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, list, &out)

	del := answerBankRequest(t, app, fiber.MethodDelete,
		"/api/v1/me/answer-bank/"+itoa(out.Data[0].ID), otherCookie, nil)
	defer del.Body.Close()
	if del.StatusCode != http.StatusNotFound {
		t.Errorf("delete status = %d, want 404", del.StatusCode)
	}
}
