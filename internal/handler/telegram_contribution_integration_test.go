//go:build integration

// Integration test for contribution-from-Telegram: a linked user pastes a board link into
// the bot chat and the webhook runs it through the same contribution flow as the site —
// recording the board, awarding a point, and replying. Run with:
// go test -tags=integration ./internal/handler/
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

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

func TestTelegramContribution(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('tgc@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	const chatID = 424242
	if _, err := pool.Exec(ctx, `INSERT INTO telegram_links (user_id, chat_id) VALUES ($1, $2)`, userID, chatID); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	// Stub Bot API that captures each reply's text.
	var lastReply string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		lastReply = body.Text
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer stub.Close()

	queries := db.New(pool)
	h := &API{
		pool:                  pool,
		queries:               queries,
		telegramLinks:         telegramnotify.NewLinkTokens("test-secret", 10*time.Minute),
		telegramBot:           telegramnotify.NewClientWithBase("bottoken", stub.URL),
		telegramWebhookSecret: "hook-secret",
		contribution:          contribution.New(contribution.NewQueriesRepository(queries, pool)),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/telegram/webhook", h.TelegramWebhook)

	post := func(chat int64, text string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"message": map[string]any{"chat": map[string]any{"id": chat}, "text": text}})
		rq := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook", bytes.NewReader(body))
		rq.Header.Set("Content-Type", "application/json")
		rq.Header.Set("X-Telegram-Bot-Api-Secret-Token", "hook-secret")
		res, err := app.Test(rq, -1)
		if err != nil {
			t.Fatalf("webhook: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("webhook status = %d, want 200", res.StatusCode)
		}
	}
	points := func() int {
		var p int
		if err := pool.QueryRow(ctx, `SELECT points FROM users WHERE id=$1`, userID).Scan(&p); err != nil {
			t.Fatalf("read points: %v", err)
		}
		return p
	}

	t.Run("a linked user's board link is recorded and rewarded", func(t *testing.T) {
		lastReply = ""
		post(chatID, "found this: https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7")
		if points() != 1 {
			t.Errorf("points = %d, want 1", points())
		}
		if !strings.Contains(lastReply, "blitzy") || !strings.Contains(lastReply, "new board") {
			t.Errorf("reply = %q, want a new-board confirmation naming blitzy", lastReply)
		}
		var board string
		if err := pool.QueryRow(ctx, `SELECT board FROM link_contributions WHERE submitted_by=$1`, userID).Scan(&board); err != nil || board != "blitzy" {
			t.Errorf("recorded board = %q (%v), want blitzy", board, err)
		}
	})

	t.Run("a second link on the same board earns no point", func(t *testing.T) {
		lastReply = ""
		post(chatID, "https://jobs.ashbyhq.com/blitzy") // the board listing this time
		if points() != 1 {
			t.Errorf("points = %d, want still 1", points())
		}
		if !strings.Contains(lastReply, "already contributed") {
			t.Errorf("reply = %q, want already-contributed", lastReply)
		}
	})

	t.Run("a non-link message draws no reply", func(t *testing.T) {
		lastReply = ""
		post(chatID, "hello bot how are you")
		if lastReply != "" {
			t.Errorf("reply = %q, want none for ordinary chatter", lastReply)
		}
	})

	t.Run("a link from an unlinked chat prompts to link", func(t *testing.T) {
		lastReply = ""
		post(999999, "https://jobs.ashbyhq.com/newco/uuid")
		if !strings.Contains(strings.ToLower(lastReply), "link your") {
			t.Errorf("reply = %q, want a link-your-account prompt", lastReply)
		}
	})
}
