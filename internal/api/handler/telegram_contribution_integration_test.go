//go:build integration

// Integration test for link-intake-from-Telegram: a linked user pastes a link into the bot
// chat and the webhook runs it through the SAME intake sequence as the website — look, import,
// record — replying with whichever of the four outcomes applies. Run with:
// go test -tags=integration ./internal/api/handler/
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

	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/platform/db"
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

	// Stub Bot API that streams each reply's text over a channel. The reply now happens in a
	// background goroutine (the webhook ACKs first), so tests wait on this rather than reading
	// a shared variable — race-free and async-aware.
	replies := make(chan string, 8)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		replies <- body.Text
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer stub.Close()

	waitReply := func(t *testing.T) string {
		t.Helper()
		select {
		case msg := <-replies:
			return msg
		case <-time.After(5 * time.Second):
			t.Fatal("no reply within 5s")
			return ""
		}
	}

	// A board we already crawl, with a resolved company — for the "already tracked" reply.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug)
		 VALUES ('greenhouse', 'acmeco:1', 'http://example.test', 'Go Dev', 'go-dev-acmeco', 'Acme Co', 'acme-co')`); err != nil {
		t.Fatalf("seed tracked job: %v", err)
	}

	queries := db.New(pool)
	contributionSvc := contribution.New(contribution.NewQueriesRepository(queries), nil)
	h := &telegramHandlers{
		queries:               queries,
		frontendOrigin:        "https://freehire.test",
		telegramLinks:         telegramnotify.NewLinkTokens("test-secret", 10*time.Minute),
		telegramBot:           telegramnotify.NewClientWithBase("bottoken", stub.URL),
		telegramWebhookSecret: "hook-secret",
		intake: &intakeService{
			queries:      queries,
			contribution: contributionSvc,
			// No page client and no ingest registry: this test is about the bot's replies, so
			// nothing is importable and every link takes the record-only path.
			imports: linkimport.New(pool, queries, nil, pagesClient{}, nil, nil),
		},
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
	// contributions counts the rows this user has recorded. It replaces the balance the
	// reward used to move: contributing earns nothing until add-invites pays it in days of
	// Pro, so what there is to assert is that the board was taken, once.
	contributions := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM boards WHERE submitted_by=$1`, userID).Scan(&n); err != nil {
			t.Fatalf("count contributions: %v", err)
		}
		return n
	}

	t.Run("the webhook ACKs fast, then records in the background", func(t *testing.T) {
		start := time.Now()
		post(chatID, "found this: https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7")
		// The webhook must return well before the reply is sent — that's the whole point of the
		// async fix (a slow ACK triggers Telegram's retry storm).
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("webhook took %v to ACK, want fast (reply is async)", d)
		}
		// Nothing is importable in this test, so the page itself cannot be read — but the board
		// behind it IS recognised and taken, and the reply must say so.
		reply := waitReply(t)
		if !strings.Contains(reply, "blitzy") {
			t.Errorf("reply = %q, want the accepted-board confirmation naming blitzy", reply)
		}
		var board string
		if err := pool.QueryRow(ctx, `SELECT board FROM boards WHERE submitted_by=$1`, userID).Scan(&board); err != nil || board != "blitzy" {
			t.Errorf("recorded board = %q (%v), want blitzy", board, err)
		}
	})

	t.Run("a second link on the same board records nothing further", func(t *testing.T) {
		post(chatID, "https://jobs.ashbyhq.com/blitzy") // the board listing this time
		waitReply(t)
		if got := contributions(); got != 1 {
			t.Errorf("contributions = %d, want still 1 (a repeat board records nothing)", got)
		}
		// The board is the unit: one live row holds its identity, so a repeat adds nothing.
		// Accepting a board already queued would buy no coverage and invite farming one board
		// from several accounts.
		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM boards WHERE provider='ashby' AND board='blitzy'`).Scan(&rows); err != nil {
			t.Fatalf("count board rows: %v", err)
		}
		if rows != 1 {
			t.Errorf("board rows = %d, want 1 — the board is the unit", rows)
		}
	})

	t.Run("a non-link message draws no reply", func(t *testing.T) {
		post(chatID, "hello bot how are you")
		select {
		case msg := <-replies:
			t.Errorf("reply = %q, want none for ordinary chatter", msg)
		case <-time.After(500 * time.Millisecond):
			// no reply — correct
		}
	})

	t.Run("a link from an unlinked chat prompts to link", func(t *testing.T) {
		post(999999, "https://jobs.ashbyhq.com/newco/uuid")
		if reply := waitReply(t); !strings.Contains(strings.ToLower(reply), "link your") {
			t.Errorf("reply = %q, want a link-your-account prompt", reply)
		}
	})

	t.Run("a posting we already carry is answered with its link", func(t *testing.T) {
		// The catalog lookup matches this greenhouse posting by its identity in the URL, so the
		// bot hands the user the job rather than talking about boards.
		post(chatID, "https://job-boards.greenhouse.io/acmeco/jobs/1")
		if reply := waitReply(t); !strings.Contains(reply, "https://freehire.test/jobs/go-dev-acmeco") {
			t.Errorf("reply = %q, want a link to the posting we already carry", reply)
		}
	})

	t.Run("an unrecognized valid link is queued for review", func(t *testing.T) {
		post(chatID, "https://example.com/careers/1")
		reply := waitReply(t)
		if !strings.Contains(reply, "check by hand") {
			t.Errorf("reply = %q, want the by-hand review message", reply)
		}
		// board_submissions has no status column — a row there IS the review state, since
		// it is deleted the moment triage resolves it into a boards row.
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM board_submissions WHERE submitted_by=$1`, userID).Scan(&n); err != nil {
			t.Fatalf("count board_submissions: %v", err)
		}
		if n != 1 {
			t.Errorf("board_submissions rows = %d, want 1 (the unrecognized link queued for review)", n)
		}
	})
}
