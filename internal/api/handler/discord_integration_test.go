//go:build integration

// Integration test for the Discord contribution surface: a linked user's /contribute resolves
// the account synchronously (a single indexed lookup, well inside Discord's 3-second budget),
// then runs the intake sequence — the same look/import/record sequence as the website and
// Telegram — in a background goroutine behind a deferred ack, replying via EditOriginalResponse
// once it finishes. The one property no unit test can prove without a real database is the "no
// anonymous contribution" constraint's more common branch: an interaction that DOES identify a
// Discord account, but whose account was never linked, must never reach intakeService.Resolve —
// see
// TestDiscordContribution/an_unlinked_identity's_.2Fcontribute_reaches_neither_intake_nor_the_reward_path
// below. Because the account lookup runs synchronously, that branch answers immediately (an
// ephemeral type-4 reply, not a deferred one) — the test asserts on the interaction response
// itself, plus directly against link_contributions, that nothing was
// written. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/discordbot"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestDiscordContribution(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('dc@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	const linkedDiscordID int64 = 111111111111111111
	if _, err := pool.Exec(ctx, `INSERT INTO discord_links (user_id, discord_id) VALUES ($1, $2)`, userID, linkedDiscordID); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Stub Discord API that captures each EditOriginalResponse PATCH's content over a channel.
	// The reply happens in a background goroutine after the interaction is deferred, so tests
	// wait on this rather than reading a shared variable.
	followups := make(chan string, 8)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content string `json:"content"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		followups <- body.Content
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer stub.Close()

	waitFollowup := func(t *testing.T) string {
		t.Helper()
		select {
		case msg := <-followups:
			return msg
		case <-time.After(5 * time.Second):
			t.Fatal("no follow-up within 5s")
			return ""
		}
	}

	queries := db.New(pool)
	contributionSvc := contribution.New(contribution.NewQueriesRepository(queries), nil)
	intake := &intakeService{
		queries:      queries,
		contribution: contributionSvc,
		// recruitee is a recognised ATS: a link on it imports for real, giving the "readable
		// vacancy" scenarios a genuine PublicSlug to link to, unlike an unrecognised board.
		imports: linkimport.New(pool, queries, nil, pagesClient{}, map[string]sources.Source{"recruitee": fakeRecruitee{}}, nil),
	}

	h := &discordHandlers{
		queries:              queries,
		discordBot:           discordbot.NewClientWithBase("bottoken", stub.URL),
		discordLinks:         discordbot.NewDiscordLinkTokens("jwt-secret", 10*time.Minute),
		discordPublicKey:     hex.EncodeToString(pub),
		discordApplicationID: "test-app-id",
		frontendOrigin:       "https://freehire.test",
		intake:               intake,
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/discord/interactions", h.DiscordInteraction)

	interact := func(t *testing.T, interaction discordbot.Interaction) discordbot.Response {
		t.Helper()
		body, err := json.Marshal(interaction)
		if err != nil {
			t.Fatal(err)
		}
		req := signedRequest(t, priv, "1700000000", body)
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("status = %d, want 200 (body %s)", res.StatusCode, body)
		}
		var out discordbot.Response
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	contribute := func(t *testing.T, discordID int64, url string) discordbot.Response {
		t.Helper()
		return interact(t, discordbot.Interaction{
			Type: discordbot.InteractionTypeApplicationCommand,
			Data: &discordbot.InteractionData{
				Name:    "contribute",
				Options: []discordbot.InteractionOption{{Name: "url", Value: url}},
			},
			Member: &discordbot.Member{User: &discordbot.User{ID: intToStr(discordID)}},
			Token:  "tok-" + intToStr(discordID) + "-" + url,
		})
	}

	// contributions counts what this user has had recorded. It replaces the balance the
	// reward used to move: contributing earns nothing until add-invites pays it in days of
	// Pro, so what there is to assert is that the board was taken, once.
	contributions := func(uid int64) int {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM link_contributions WHERE submitted_by=$1`, uid).Scan(&n); err != nil {
			t.Fatalf("count contributions: %v", err)
		}
		return n
	}

	t.Run("a linked user's /contribute on a readable vacancy defers then follows up with a link to the posting", func(t *testing.T) {
		out := contribute(t, linkedDiscordID, "https://newco1.recruitee.com/o/senior-go")
		if out.Type != discordbot.ResponseTypeDeferredChannelMessageWithSource {
			t.Fatalf("response type = %d, want deferred", out.Type)
		}
		reply := waitFollowup(t)
		if !strings.Contains(reply, "https://freehire.test/jobs/") {
			t.Errorf("follow-up = %q, want a link to the imported posting", reply)
		}
		// The board is novel: this is also the first recorded contribution.
		if got := contributions(userID); got != 1 {
			t.Errorf("contributions = %d, want 1", got)
		}
	})

	t.Run("a linked user's /contribute on a novel board records the contribution", func(t *testing.T) {
		contribute(t, linkedDiscordID, "https://newco2.recruitee.com/o/backend-eng")
		waitFollowup(t)
		if got := contributions(userID); got != 2 {
			t.Errorf("contributions = %d, want 2 (a second, distinct board)", got)
		}
		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM link_contributions WHERE source='recruitee' AND board='newco2' AND submitted_by=$1`,
			userID).Scan(&rows); err != nil {
			t.Fatalf("count board rows: %v", err)
		}
		if rows != 1 {
			t.Errorf("recorded rows for newco2 = %d, want 1", rows)
		}
	})

	t.Run("a second /contribute on an already-contributed board records nothing further", func(t *testing.T) {
		contribute(t, linkedDiscordID, "https://newco2.recruitee.com/o/another-role")
		waitFollowup(t)
		if got := contributions(userID); got != 2 {
			t.Errorf("contributions = %d, want still 2 (a repeat board records nothing)", got)
		}
		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM link_contributions WHERE source='recruitee' AND board='newco2'`).Scan(&rows); err != nil {
			t.Fatalf("count board rows: %v", err)
		}
		if rows != 1 {
			t.Errorf("board rows for newco2 = %d, want 1 — the board is the unit", rows)
		}
	})

	t.Run("an unlinked identity's /contribute reaches the intake at all", func(t *testing.T) {
		const unlinkedDiscordID int64 = 999999999999999999
		const url = "https://newco3.recruitee.com/o/never-imported"
		out := contribute(t, unlinkedDiscordID, url)
		// GetUserIDByDiscordID runs synchronously in handleContributeCommand, so an unlinked
		// account is answered immediately (ephemeral, type 4) rather than deferred — there is no
		// background goroutine, and so no follow-up PATCH to wait for.
		if out.Type != discordbot.ResponseTypeChannelMessageWithSource {
			t.Fatalf("response type = %d, want immediate channel message (not deferred)", out.Type)
		}
		if out.Data == nil || out.Data.Flags != discordbot.FlagEphemeral {
			t.Errorf("response data = %+v, want ephemeral flag set", out.Data)
		}
		reply := out.Data.Content
		if !strings.Contains(strings.ToLower(reply), "link your") {
			t.Errorf("reply = %q, want a link-your-account prompt", reply)
		}
		// The proof this is not just a wording check: no row was written for the link at all,
		// and nothing was recorded against any user — GetUserIDByDiscordID's ErrNoRows branch
		// really does return before intakeService.Resolve is ever called.
		var rows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM link_contributions WHERE url=$1`, url).Scan(&rows); err != nil {
			t.Fatalf("count contribution rows: %v", err)
		}
		if rows != 0 {
			t.Errorf("link_contributions rows for the unlinked attempt = %d, want 0", rows)
		}
		var jobs int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE source='recruitee' AND external_id LIKE 'newco3:%'`).Scan(&jobs); err != nil {
			t.Fatalf("count imported jobs: %v", err)
		}
		if jobs != 0 {
			t.Errorf("jobs imported for the unlinked attempt = %d, want 0", jobs)
		}
		if got := contributions(userID); got != 2 {
			t.Errorf("the LINKED user's contributions moved to %d off an UNLINKED attempt, want unchanged 2", got)
		}
	})

	link := func(t *testing.T, discordID int64, token string) discordbot.Response {
		t.Helper()
		return interact(t, discordbot.Interaction{
			Type: discordbot.InteractionTypeApplicationCommand,
			Data: &discordbot.InteractionData{
				Name:    "link",
				Options: []discordbot.InteractionOption{{Name: "token", Value: token}},
			},
			Member: &discordbot.Member{User: &discordbot.User{ID: intToStr(discordID)}},
		})
	}

	t.Run("/link with a valid token creates the discord_links row", func(t *testing.T) {
		var linkerID int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('linker@example.test') RETURNING id`).Scan(&linkerID); err != nil {
			t.Fatalf("seed linker user: %v", err)
		}
		token, err := h.discordLinks.Issue(linkerID)
		if err != nil {
			t.Fatal(err)
		}
		const newDiscordID int64 = 222222222222222222
		out := link(t, newDiscordID, token)
		if out.Type != discordbot.ResponseTypeChannelMessageWithSource {
			t.Fatalf("response type = %d, want channel message", out.Type)
		}
		if !strings.Contains(out.Data.Content, "Linked") {
			t.Errorf("reply = %q, want a success message", out.Data.Content)
		}
		var gotUserID int64
		if err := pool.QueryRow(ctx, `SELECT user_id FROM discord_links WHERE discord_id=$1`, newDiscordID).Scan(&gotUserID); err != nil {
			t.Fatalf("read discord_links: %v", err)
		}
		if gotUserID != linkerID {
			t.Errorf("linked user_id = %d, want %d", gotUserID, linkerID)
		}

		// Re-linking the same account under a new token updates the existing row rather than
		// duplicating it — one row per freehire user, per the table's PK.
		token2, err := h.discordLinks.Issue(linkerID)
		if err != nil {
			t.Fatal(err)
		}
		const relinkedDiscordID int64 = 333333333333333333
		link(t, relinkedDiscordID, token2)
		var rowCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM discord_links WHERE user_id=$1`, linkerID).Scan(&rowCount); err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if rowCount != 1 {
			t.Errorf("discord_links rows for the relinked user = %d, want 1", rowCount)
		}
	})

	t.Run("/link with garbage or expired token writes nothing and replies with failure", func(t *testing.T) {
		before := discordLinkRowCount(t, pool, ctx)

		const garbageDiscordID int64 = 444444444444444444
		out := link(t, garbageDiscordID, "not-a-real-token")
		if out.Data == nil || !strings.Contains(strings.ToLower(out.Data.Content), "invalid") {
			t.Errorf("garbage-token reply = %+v, want an invalid/expired message", out.Data)
		}

		var expiredID int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('expiredlink@example.test') RETURNING id`).Scan(&expiredID); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		// A second token minter over the same secret, but already-expired TTL: a validly-signed
		// token the handler's Parse still rejects on the expiry check.
		expiredLinks := discordbot.NewDiscordLinkTokens("jwt-secret", -1*time.Minute)
		expiredToken, err := expiredLinks.Issue(expiredID)
		if err != nil {
			t.Fatal(err)
		}
		const expiredDiscordID int64 = 555555555555555555
		out2 := link(t, expiredDiscordID, expiredToken)
		if out2.Data == nil || !strings.Contains(strings.ToLower(out2.Data.Content), "invalid") {
			t.Errorf("expired-token reply = %+v, want an invalid/expired message", out2.Data)
		}

		after := discordLinkRowCount(t, pool, ctx)
		if after != before {
			t.Errorf("discord_links row count changed %d -> %d, want unchanged (both tokens should be rejected)", before, after)
		}
		for _, id := range []int64{garbageDiscordID, expiredDiscordID} {
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM discord_links WHERE discord_id=$1`, id).Scan(&count); err != nil {
				t.Fatalf("count rows for %d: %v", id, err)
			}
			if count != 0 {
				t.Errorf("discord_links rows for discord_id=%d = %d, want 0", id, count)
			}
		}
	})
}

func discordLinkRowCount(t *testing.T, pool *pgxpool.Pool, ctx context.Context) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM discord_links`).Scan(&count); err != nil {
		t.Fatalf("count discord_links: %v", err)
	}
	return count
}

func intToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

// TestDiscordFeatureDisabled mirrors TestTelegramDisabledWhenUnconfigured's shape for Discord:
// with any of the four config values missing, /me/discord must report the feature off (while
// still answering, since it needs no bot config to read a link row) and the interaction webhook
// must answer 404 rather than accept an unsigned or wrongly-signed request.
func TestDiscordFeatureDisabled(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('dcdisabled@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	queries := db.New(pool)
	// newDiscordHandlers with an empty bot token leaves the feature disabled, per its
	// all-or-nothing gate — queries is still wired because DiscordLinkStatus reads the DB
	// regardless of whether the bot itself is configured.
	h := newDiscordHandlers(queries, "jwt-secret", "", "app-id", "pub-key", "guild-id", "https://freehire.test", nil)

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/discord", auth.RequireAuth(iss, testVersions), h.DiscordLinkStatus)
	app.Post("/api/v1/discord/interactions", h.DiscordInteraction)

	t.Run("/me/discord reports the feature disabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/discord", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var out struct {
			Data struct {
				Enabled bool `json:"enabled"`
				Linked  bool `json:"linked"`
			} `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out.Data.Enabled {
			t.Error("enabled = true, want false with the bot token missing")
		}
		if out.Data.Linked {
			t.Error("linked = true, want false — no discord_links row exists")
		}
	})

	t.Run("the interaction webhook answers 404 rather than accept a request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/discord/interactions", strings.NewReader(`{"type":1}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", res.StatusCode)
		}
	})
}
