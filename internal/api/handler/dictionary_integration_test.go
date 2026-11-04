//go:build integration

package handler

import (
	"context"
	"encoding/json"
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

func TestIndustryDictionaryGapIsDeduplicatedByTheExistingQueue(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, "industry-gap@example.test").Scan(&userID); err != nil {
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
	for i, term := range []string{"underwater basket weaving industry", "UNDERWATER BASKET WEAVING INDUSTRY"} {
		code, body := doDictionary(t, app, http.MethodPost, path, `{"kind":"industry","term":"`+term+`"}`, cookie)
		want := float64(i + 1)
		if code != http.StatusOK || body["data"].(map[string]any)["request_count"] != want {
			t.Fatalf("industry request %d = %d %#v, want count %v", i+1, code, body, want)
		}
	}
	var kind, raw string
	var count, writes int32
	if err := pool.QueryRow(ctx, `SELECT kind, raw_term, request_count FROM dictionary_gaps WHERE kind = 'industry'`).Scan(&kind, &raw, &count); err != nil {
		t.Fatal(err)
	}
	if kind != "industry" || raw != "underwater basket weaving industry" || count != 2 {
		t.Fatalf("stored kind=%q raw=%q count=%d", kind, raw, count)
	}
	if err := pool.QueryRow(ctx, `SELECT writes FROM dictionary_gap_daily WHERE user_id = $1`, userID).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatalf("daily writes = %d, want 2", writes)
	}
}

func TestDictionaryMCPAndWebsiteShareQueueAndDailyCap(t *testing.T) {
	pool := startPostgres(t)
	ctx := t.Context()
	queries := db.New(pool)
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('dictionary-mcp@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	h := &dictionaryHandlers{pool: pool, queries: queries}
	issuer := auth.NewIssuer("test-secret", time.Hour)
	token, err := issuer.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(issuer, testVersions)})
	cookie := &http.Cookie{Name: auth.CookieName, Value: token}
	tool := (&assistantHandlers{dictionary: h}).dictionaryMCPTools()[0]
	body := json.RawMessage(`{"kind":"skill","term":"Unlisted Quasar Tech"}`)
	if _, err := tool.Run(ctx, userID, body); err != nil {
		t.Fatal(err)
	}
	code, response := doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", `{"kind":"skill","term":"unlisted-quasar-tech"}`, cookie)
	if code != http.StatusOK || response["data"].(map[string]any)["request_count"] != float64(2) {
		t.Fatalf("HTTP did not share MCP queue: %d %#v", code, response)
	}
	var owner int64
	var writes int32
	if err := pool.QueryRow(ctx, `SELECT submitted_by FROM dictionary_gaps WHERE kind='skill'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != userID {
		t.Fatalf("queue owner=%d want%d", owner, userID)
	}
	if err := pool.QueryRow(ctx, `SELECT writes FROM dictionary_gap_daily WHERE user_id=$1`, userID).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatalf("shared writes=%d", writes)
	}
	if _, err := pool.Exec(ctx, `UPDATE dictionary_gap_daily SET writes=20 WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Run(ctx, userID, body); err == nil {
		t.Fatal("MCP bypassed website cap")
	}
	code, _ = doDictionary(t, app, http.MethodPost, "/api/v1/dictionary/gaps", string(body), cookie)
	if code != http.StatusTooManyRequests {
		t.Fatalf("HTTP bypassed MCP cap: %d", code)
	}
	var count int32
	if err := pool.QueryRow(ctx, `SELECT request_count FROM dictionary_gaps WHERE kind='skill'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("refused writes changed queue: %d", count)
	}
}
