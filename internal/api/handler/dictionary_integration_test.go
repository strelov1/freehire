//go:build integration

package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestDictionaryRecordWritesInsideTheDailyCap(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, "dict-gap@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: auth.CookieName, Value: token}
	h := &dictionaryHandlers{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h.register(api, middleware{cookie: auth.RequireAuth(iss, testVersions)})

	const path = "/api/v1/dictionary/gaps"
	code, body := doDictionary(t, app, http.MethodPost, path, `{"kind":"specialization","term":"Underwater Basket Weaving"}`, cookie)
	if code != http.StatusOK || body["data"].(map[string]any)["request_count"] != float64(1) {
		t.Fatalf("first = %d %#v", code, body)
	}
	code, body = doDictionary(t, app, http.MethodPost, path, `{"kind":"specialization","term":"underwater basket weaving"}`, cookie)
	if code != http.StatusOK || body["data"].(map[string]any)["request_count"] != float64(2) {
		t.Fatalf("second = %d %#v", code, body)
	}
	var raw string
	var count, writes int32
	if err := pool.QueryRow(ctx, `SELECT raw_term, request_count FROM dictionary_gaps WHERE kind = 'specialization'`).Scan(&raw, &count); err != nil {
		t.Fatal(err)
	}
	if raw != "Underwater Basket Weaving" || count != 2 {
		t.Fatalf("stored raw=%q count=%d", raw, count)
	}
	if err := pool.QueryRow(ctx, `SELECT writes FROM dictionary_gap_daily WHERE user_id = $1`, userID).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatalf("writes = %d", writes)
	}

	if _, err := pool.Exec(ctx, `UPDATE dictionary_gap_daily SET writes = 20 WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	code, _ = doDictionary(t, app, http.MethodPost, path, `{"kind":"specialization","term":"underwater basket weaving"}`, cookie)
	if code != http.StatusTooManyRequests {
		t.Fatalf("cap = %d", code)
	}
	if err := pool.QueryRow(ctx, `SELECT request_count FROM dictionary_gaps WHERE kind = 'specialization'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("capped write changed the count to %d", count)
	}
	if err := pool.QueryRow(ctx, `SELECT writes FROM dictionary_gap_daily WHERE user_id = $1`, userID).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 20 {
		t.Fatalf("capped write changed writes to %d", writes)
	}

	code, _ = doDictionary(t, app, http.MethodPost, path, `{"kind":"specialization","term":"   "}`, cookie)
	if code != http.StatusBadRequest {
		t.Fatalf("blank = %d", code)
	}
	if err := pool.QueryRow(ctx, `SELECT writes FROM dictionary_gap_daily WHERE user_id = $1`, userID).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 20 {
		t.Fatalf("blank consumed a write: %d", writes)
	}
}
