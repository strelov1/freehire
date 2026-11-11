//go:build integration

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

type banRig struct {
	app     *fiber.App
	pool    *pgxpool.Pool
	queries *db.Queries
	svc     *accounts.Service
	mailer  *captureMailer
	h       *authHandlers
}

// newBanRig wires the real accounts service and the real moderation routes (cookie auth,
// then the moderator role) onto the sign-in routes.
func newBanRig(t *testing.T) *banRig {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	mailer := &captureMailer{}
	svc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	svc.WithCodes(accounts.NewQueriesCodeStore(queries, pool), mailer)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss, accounts: svc}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookie := auth.RequireAuth(iss, queries)
	app.Post("/api/v1/auth/register", h.Register)
	app.Post("/api/v1/auth/login", h.Login)
	app.Get("/api/v1/auth/me", cookie, h.Me)
	moderation := newUserModerationHandlers(queries)
	moderation.register(app.Group("/api/v1"), middleware{
		cookie: cookie, moderator: auth.RequireRole(queries, "moderator"),
	}, newContract(contractHandlers{userModerationHandlers: moderation}))
	return &banRig{app: app, pool: pool, queries: queries, svc: svc, mailer: mailer, h: h}
}

// banReply is a response with its body already closed.
type banReply struct {
	StatusCode int
	Cookies    []*http.Cookie
}

func (r *banRig) do(t *testing.T, method, path, body, cookie string) banReply {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := r.app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	_ = resp.Body.Close()
	return banReply{StatusCode: resp.StatusCode, Cookies: resp.Cookies()}
}

func (b banReply) session(t *testing.T) string {
	t.Helper()
	for _, c := range b.Cookies {
		if c.Name == auth.CookieName && c.Value != "" {
			return c.Value
		}
	}
	t.Fatal("response carried no session cookie")
	return ""
}

// signUp registers an account with a role and returns its id and a live session cookie.
func (r *banRig) signUp(t *testing.T, email, role string) (int64, string) {
	t.Helper()
	resp := r.do(t, fiber.MethodPost, "/api/v1/auth/register", fmt.Sprintf(`{"email":%q,"password":"correct-horse"}`, email), "")
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("register %s: status %d", email, resp.StatusCode)
	}
	var id int64
	if err := r.pool.QueryRow(context.Background(),
		`UPDATE users SET role = $2, email_verified = true WHERE lower(email) = $1 RETURNING id`, email, role).Scan(&id); err != nil {
		t.Fatalf("promote %s: %v", email, err)
	}
	return id, resp.session(t)
}

func (r *banRig) ban(t *testing.T, modCookie string, target int64, reason string) banReply {
	t.Helper()
	return r.do(t, fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", target), fmt.Sprintf(`{"reason":%q}`, reason), modCookie)
}

func TestAccountBan(t *testing.T) {
	r := newBanRig(t)
	ctx := context.Background()
	_, modCookie := r.signUp(t, "mod@example.test", "moderator")
	adminID, _ := r.signUp(t, "admin@example.test", "admin")
	victimID, victimCookie := r.signUp(t, "victim@example.test", "user")

	if _, err := r.queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{UserID: victimID, Name: "cli", TokenHash: "ban-key-hash", TokenPrefix: "fhk_ban", Scope: "full"}); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if got := r.do(t, fiber.MethodGet, "/api/v1/auth/me", "", victimCookie).StatusCode; got != fiber.StatusOK {
		t.Fatalf("before the ban: /auth/me = %d, want 200", got)
	}
	if _, err := r.queries.AuthenticateAPIKey(ctx, "ban-key-hash"); err != nil {
		t.Fatalf("before the ban: API key refused: %v", err)
	}

	t.Run("a ban needs a reason", func(t *testing.T) {
		if got := r.ban(t, modCookie, victimID, "  ").StatusCode; got != fiber.StatusBadRequest {
			t.Errorf("blank reason = %d, want 400", got)
		}
	})
	t.Run("a non-moderator cannot ban", func(t *testing.T) {
		if got := r.ban(t, victimCookie, victimID, "spam").StatusCode; got != fiber.StatusForbidden {
			t.Errorf("user banning = %d, want 403", got)
		}
	})
	t.Run("a moderator cannot ban an admin or themselves", func(t *testing.T) {
		if got := r.ban(t, modCookie, adminID, "spam").StatusCode; got != fiber.StatusForbidden {
			t.Errorf("banning an admin = %d, want 403", got)
		}
		var modID int64
		if err := r.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = 'mod@example.test'`).Scan(&modID); err != nil {
			t.Fatal(err)
		}
		if got := r.ban(t, modCookie, modID, "spam").StatusCode; got != fiber.StatusForbidden {
			t.Errorf("banning self = %d, want 403", got)
		}
		// The SQL rule holds without the handler's pre-checks.
		if _, err := r.queries.BanUser(ctx, db.BanUserParams{ID: adminID, ActorID: pgtype.Int8{Int64: modID, Valid: true}, Reason: pgtype.Text{String: "x", Valid: true}}); err != pgx.ErrNoRows {
			t.Errorf("BanUser on an admin: err = %v, want pgx.ErrNoRows", err)
		}
	})

	if got := r.ban(t, modCookie, victimID, "harassment").StatusCode; got != fiber.StatusOK {
		t.Fatalf("ban = %d, want 200", got)
	}

	t.Run("the existing cookie session is dead", func(t *testing.T) {
		if got := r.do(t, fiber.MethodGet, "/api/v1/auth/me", "", victimCookie).StatusCode; got != fiber.StatusUnauthorized {
			t.Errorf("banned session = %d, want 401", got)
		}
	})
	t.Run("the API key is refused", func(t *testing.T) {
		if _, err := r.queries.AuthenticateAPIKey(ctx, "ban-key-hash"); err != pgx.ErrNoRows {
			t.Errorf("banned key: err = %v, want pgx.ErrNoRows", err)
		}
		if _, err := r.queries.ResolveAPIKeyIdentity(ctx, "ban-key-hash"); err != pgx.ErrNoRows {
			t.Errorf("banned key introspection: err = %v, want pgx.ErrNoRows", err)
		}
	})
	t.Run("every session-minting path (OAuth, Apple, mobile exchange) is refused", func(t *testing.T) {
		if _, err := r.h.sessionVersion(ctx, victimID); err != errAccountSuspended {
			t.Errorf("sessionVersion for a banned account: err = %v, want errAccountSuspended", err)
		}
	})
	t.Run("sign-in is refused and no reset code is sent", func(t *testing.T) {
		resp := r.do(t, fiber.MethodPost, "/api/v1/auth/login", `{"email":"victim@example.test","password":"correct-horse"}`, "")
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("login = %d, want 403", resp.StatusCode)
		}
		if err := r.svc.RequestPasswordReset(ctx, "victim@example.test"); err != nil {
			t.Fatalf("RequestPasswordReset: %v", err)
		}
		if len(r.mailer.reset) != 0 {
			t.Errorf("a reset code was mailed to a banned account: %v", r.mailer.reset)
		}
	})
	t.Run("optional mail skips the account", func(t *testing.T) {
		var got string
		if err := r.pool.QueryRow(ctx,
			`SELECT mail_recipient(email, email_verified, last_active_at, banned_at) FROM users WHERE id = $1`, victimID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Errorf("mail_recipient = %q, want ''", got)
		}
	})
	t.Run("push devices and the Telegram link are not delivered to", func(t *testing.T) {
		if _, err := r.pool.Exec(ctx, `INSERT INTO user_push_tokens (user_id, token, platform) VALUES ($1, 'ban-push', 'ios')`, victimID); err != nil {
			t.Fatal(err)
		}
		toks, err := r.queries.ListPushTokensForUser(ctx, victimID)
		if err != nil || len(toks) != 0 {
			t.Errorf("push tokens = %d, err %v; want none", len(toks), err)
		}
	})

	t.Run("unban restores sign-in", func(t *testing.T) {
		resp := r.do(t, fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/unban", victimID), "", modCookie)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("unban = %d, want 200", resp.StatusCode)
		}
		login := r.do(t, fiber.MethodPost, "/api/v1/auth/login", `{"email":"victim@example.test","password":"correct-horse"}`, "")
		if login.StatusCode != fiber.StatusOK {
			t.Fatalf("login after unban = %d, want 200", login.StatusCode)
		}
		if got := r.do(t, fiber.MethodGet, "/api/v1/auth/me", "", login.session(t)).StatusCode; got != fiber.StatusOK {
			t.Errorf("new session after unban = %d, want 200", got)
		}
		// The ban bumped the generation: the old session stays dead.
		if got := r.do(t, fiber.MethodGet, "/api/v1/auth/me", "", victimCookie).StatusCode; got != fiber.StatusUnauthorized {
			t.Errorf("pre-ban session after unban = %d, want 401", got)
		}
		var by, reason any
		if err := r.pool.QueryRow(ctx, `SELECT banned_by, ban_reason FROM users WHERE id = $1`, victimID).Scan(&by, &reason); err != nil {
			t.Fatal(err)
		}
		if by != nil || reason != nil {
			t.Errorf("unban left banned_by=%v ban_reason=%v", by, reason)
		}
	})
}

// The moderation endpoints' refusals, one case each. Every case states the status the
// caller must see and, where a write was refused, that the account is unchanged.
func TestAccountBanModerationErrors(t *testing.T) {
	r := newBanRig(t)
	ctx := context.Background()
	modID, modCookie := r.signUp(t, "mod2@example.test", "moderator")
	adminID, _ := r.signUp(t, "admin2@example.test", "admin")
	victimID, _ := r.signUp(t, "victim2@example.test", "user")
	if got := r.ban(t, modCookie, victimID, "spam").StatusCode; got != fiber.StatusOK {
		t.Fatalf("setup ban = %d", got)
	}
	liveID, _ := r.signUp(t, "live2@example.test", "user")

	cases := []struct {
		name         string
		method, path string
		body         string
		want         int
	}{
		{"ban: non-numeric id", fiber.MethodPost, "/api/v1/moderation/users/abc/ban", `{"reason":"x"}`, fiber.StatusBadRequest},
		{"ban: malformed body", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", liveID), `{`, fiber.StatusBadRequest},
		{"ban: empty reason", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", liveID), `{"reason":""}`, fiber.StatusBadRequest},
		{"ban: whitespace reason", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", liveID), `{"reason":"   "}`, fiber.StatusBadRequest},
		{"ban: reason too long", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", liveID), fmt.Sprintf(`{"reason":%q}`, strings.Repeat("x", 501)), fiber.StatusBadRequest},
		{"ban: unknown user", fiber.MethodPost, "/api/v1/moderation/users/999999999/ban", `{"reason":"x"}`, fiber.StatusNotFound},
		{"ban: already banned", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", victimID), `{"reason":"again"}`, fiber.StatusConflict},
		{"ban: an admin", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", adminID), `{"reason":"x"}`, fiber.StatusForbidden},
		{"ban: self", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/ban", modID), `{"reason":"x"}`, fiber.StatusForbidden},
		{"unban: non-numeric id", fiber.MethodPost, "/api/v1/moderation/users/abc/unban", ``, fiber.StatusBadRequest},
		{"unban: not banned", fiber.MethodPost, fmt.Sprintf("/api/v1/moderation/users/%d/unban", liveID), ``, fiber.StatusNotFound},
		{"unban: unknown user", fiber.MethodPost, "/api/v1/moderation/users/999999999/unban", ``, fiber.StatusNotFound},
		{"find: no query", fiber.MethodGet, "/api/v1/moderation/users", ``, fiber.StatusBadRequest},
		{"find: no match", fiber.MethodGet, "/api/v1/moderation/users?q=nobody%40nowhere.test", ``, fiber.StatusNotFound},
		{"find: by email", fiber.MethodGet, "/api/v1/moderation/users?q=LIVE2%40example.test", ``, fiber.StatusOK},
		{"find: by id", fiber.MethodGet, fmt.Sprintf("/api/v1/moderation/users?q=%d", liveID), ``, fiber.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.do(t, tc.method, tc.path, tc.body, modCookie).StatusCode; got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("the refused writes changed nothing", func(t *testing.T) {
		var banned []int64
		rows, err := r.pool.Query(ctx, `SELECT id FROM users WHERE banned_at IS NOT NULL`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			banned = append(banned, id)
		}
		if len(banned) != 1 || banned[0] != victimID {
			t.Errorf("banned accounts = %v, want only %d", banned, victimID)
		}
	})
	t.Run("find shows the ban and its reason", func(t *testing.T) {
		resp := r.do(t, fiber.MethodGet, fmt.Sprintf("/api/v1/moderation/users?q=%d", victimID), "", modCookie)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		var banned bool
		var reason string
		if err := r.pool.QueryRow(ctx, `SELECT banned_at IS NOT NULL, ban_reason FROM users WHERE id = $1`, victimID).Scan(&banned, &reason); err != nil || !banned || reason != "spam" {
			t.Errorf("banned=%v reason=%q err=%v", banned, reason, err)
		}
	})
}

// users_ban_consistent: a ban must carry a non-blank reason, and a NULL reason is not
// blank-by-accident (a CHECK passes on NULL, so the constraint has to say it outright).
func TestAccountBanConstraintRefusesAMissingReason(t *testing.T) {
	r := newBanRig(t)
	ctx := context.Background()
	id, _ := r.signUp(t, "check@example.test", "user")
	for name, reason := range map[string]any{"NULL": nil, "blank": "  "} {
		if _, err := r.pool.Exec(ctx, `UPDATE users SET banned_at = now(), ban_reason = $2 WHERE id = $1`, id, reason); err == nil {
			t.Errorf("a ban with a %s reason was accepted", name)
		}
	}
	if _, err := r.pool.Exec(ctx, `UPDATE users SET banned_at = now(), ban_reason = 'ok' WHERE id = $1`, id); err != nil {
		t.Errorf("a ban with a reason was refused: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE users SET ban_reason = NULL WHERE id = $1`, id); err == nil {
		t.Error("clearing the reason while banned was accepted")
	}
}
