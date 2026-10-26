//go:build integration

// Integration tests for the interview-buddy HTTP contract. A real Postgres exercises the
// service end to end; the routes are mounted through the real register so a path or a gate
// that drifts from the contract fails here. Run with:
// go test -tags=integration ./internal/api/handler/ -run Buddy
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// accountIDBase is the first account id newBuddyEnv seeds.
const accountIDBase = 7_340_001

type buddyEnv struct {
	t     *testing.T
	app   *fiber.App
	iss   *auth.Issuer
	pool  *pgxpool.Pool
	users map[string]int64
}

func newBuddyEnv(t *testing.T, names ...string) *buddyEnv {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	env := &buddyEnv{t: t, iss: iss, pool: pool, users: map[string]int64{}}

	cv, _ := json.Marshal(map[string]any{
		"total_years": 7,
		"skills":      []string{"Go"},
		"experience":  []map[string]any{{"title": "Senior Backend Engineer", "company": "Initech", "current": true}},
	})
	for i, name := range names {
		role := "user"
		if name == "mod" {
			role = "moderator"
		}
		// Account ids far from any conversation, message or report id, so a payload that
		// leaks one is caught by a plain substring search (TestBuddyPayloadsNeverNameTheAccount).
		var id int64
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO users (id, email, role, resume_structured) OVERRIDING SYSTEM VALUE
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			accountIDBase+int64(i), name+"@example.test", role, cv).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		env.users[name] = id
	}

	repo := buddy.NewQueriesRepository(queries)
	msg := messaging.New(messaging.NewQueriesRepository(queries), buddy.NewPersona(repo))
	env.app = fiber.New(fiber.Config{ErrorHandler: RenderError})
	mw := middleware{
		cookie:    auth.RequireAuth(iss, testVersions),
		moderator: auth.RequireRole(queries, "moderator"),
	}
	newBuddyHandlers(buddy.New(repo, msg)).register(env.app.Group("/api/v1"), mw)
	newConversationHandlers(msg).register(env.app.Group("/api/v1"), mw)
	return env
}

// send sends one request as user (empty for anonymous) and returns the raw answer.
func (e *buddyEnv) send(method, path, user string, body any) (int, []byte, http.Header) {
	e.t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(context.Background(), method, path, buf)
	req.Host = "localhost"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		tok, err := e.iss.Issue(e.users[user], testTokenVersion)
		if err != nil {
			e.t.Fatalf("issue: %v", err)
		}
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
	}
	resp, err := e.app.Test(req, -1)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, resp.Header
}

// do sends one request as user (empty for anonymous) and decodes the JSON answer.
func (e *buddyEnv) do(method, path, user string, body any) (int, map[string]any, http.Header) {
	e.t.Helper()
	code, raw, hdr := e.send(method, path, user, body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return code, out, hdr
}

// raw sends one request as user and returns the undecoded answer, failing on any status
// but code.
func (e *buddyEnv) raw(code int, method, path, user string, body any) string {
	e.t.Helper()
	got, out, _ := e.send(method, path, user, body)
	if got != code {
		e.t.Fatalf("%s %s as %q: status %d body %s, want %d", method, path, user, got, out, code)
	}
	return string(out)
}

func (e *buddyEnv) want(code int, method, path, user string, body any) map[string]any {
	e.t.Helper()
	got, out, _ := e.do(method, path, user, body)
	if got != code {
		e.t.Fatalf("%s %s as %q: status %d body %v, want %d", method, path, user, got, out, code)
	}
	return out
}

func (e *buddyEnv) join(user string) string {
	e.t.Helper()
	out := e.want(http.StatusOK, http.MethodPost, "/api/v1/me/buddies", user, nil)
	return out["data"].(map[string]any)["handle"].(string)
}

// counterpartHandle is the other member's handle in a conversation answer.
func counterpartHandle(conv map[string]any) any {
	return conv["counterpart"].(map[string]any)["detail"].(map[string]any)["handle"]
}

func convPath(id float64, suffix string) string {
	return "/api/v1/me/conversations/" + strconv.FormatInt(int64(id), 10) + suffix
}

func TestBuddyRequiresSession(t *testing.T) {
	env := newBuddyEnv(t, "a")
	env.want(http.StatusUnauthorized, http.MethodGet, "/api/v1/me/buddies", "", nil)
	env.want(http.StatusUnauthorized, http.MethodGet, "/api/v1/me/buddies/similar", "", nil)
	env.want(http.StatusUnauthorized, http.MethodPost, "/api/v1/me/buddies/invite", "", map[string]any{})
	env.want(http.StatusUnauthorized, http.MethodGet, "/api/v1/me/conversations", "", nil)
	env.want(http.StatusUnauthorized, http.MethodGet, "/api/v1/moderation/conversation-reports", "", nil)
}

func TestBuddyNonMember(t *testing.T) {
	env := newBuddyEnv(t, "a")
	out := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies", "a", nil)
	if out["data"].(map[string]any)["member"] != false {
		t.Fatalf("status = %v", out)
	}
	env.want(http.StatusForbidden, http.MethodGet, "/api/v1/me/buddies/similar", "a", nil)
	env.want(http.StatusForbidden, http.MethodGet, "/api/v1/me/buddies/feed", "a", nil)
	env.want(http.StatusForbidden, http.MethodPost, "/api/v1/me/buddies/invite", "a", map[string]any{"handle": "x", "body": "hi"})
	// The conversation list is the messaging core's, open to every account: a non-member
	// simply has no buddy conversations in it.
	if l := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/conversations", "a", nil)["data"].([]any); len(l) != 0 {
		t.Fatalf("non-member list = %v", l)
	}
	env.want(http.StatusForbidden, http.MethodDelete, "/api/v1/me/buddies", "a", nil)
}

func TestBuddyMembershipSimilarFeedProfile(t *testing.T) {
	env := newBuddyEnv(t, "a", "b")
	ha := env.join("a")
	hb := env.join("b")
	if ha == "" || ha == hb {
		t.Fatalf("handles %q %q", ha, hb)
	}
	st := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies", "a", nil)["data"].(map[string]any)
	if st["member"] != true || st["handle"] != ha || st["card_empty"] != false {
		t.Fatalf("status = %v", st)
	}

	sim := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/similar?limit=10", "a", nil)
	rows := sim["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["handle"] != hb {
		t.Fatalf("similar = %v", sim)
	}
	if rows[0].(map[string]any)["shared_companies"] == nil {
		t.Fatalf("shared_companies must be [] not null: %v", rows[0])
	}
	if sim["meta"].(map[string]any)["limit"] != float64(10) {
		t.Fatalf("meta = %v", sim["meta"])
	}

	prof := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/profiles/"+hb, "a", nil)["data"].(map[string]any)
	if prof["handle"] != hb || prof["card"] == nil {
		t.Fatalf("profile = %v", prof)
	}
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/nobody-here", "a", nil)

	feed := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/feed?mode=all", "a", nil)
	if feed["meta"].(map[string]any)["next_cursor"] != nil {
		t.Fatalf("feed meta = %v", feed["meta"])
	}
	env.want(http.StatusBadRequest, http.MethodGet, "/api/v1/me/buddies/feed?mode=nope", "a", nil)
	env.want(http.StatusBadRequest, http.MethodGet, "/api/v1/me/buddies/feed?mode=all&cursor=garbage", "a", nil)

	// ?handle= reads one member's activity; mode is not read beside it.
	member := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/feed?handle="+hb+"&mode=nope", "a", nil)
	if data, ok := member["data"].([]any); !ok || len(data) != 0 || member["meta"].(map[string]any)["next_cursor"] != nil {
		t.Fatalf("member feed = %v", member)
	}
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/feed?handle=nobody-here", "a", nil)
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/feed?handle="+ha, "a", nil)
	env.want(http.StatusBadRequest, http.MethodGet, "/api/v1/me/buddies/feed?handle="+hb+"&cursor=garbage", "a", nil)

	env.want(http.StatusNoContent, http.MethodDelete, "/api/v1/me/buddies", "b", nil)
	if env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies", "b", nil)["data"].(map[string]any)["member"] != false {
		t.Fatal("left member still reports as member")
	}
}

func TestBuddyHiddenApplications(t *testing.T) {
	env := newBuddyEnv(t, "a")
	env.join("a")
	out := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/hidden-applications", "a", nil)
	if l, ok := out["data"].([]any); !ok || len(l) != 0 {
		t.Fatalf("hidden = %v", out)
	}
	// An application that is not the caller's is not found.
	env.want(http.StatusNotFound, http.MethodPut, "/api/v1/me/buddies/hidden-applications/999999", "a", nil)
	env.want(http.StatusNoContent, http.MethodDelete, "/api/v1/me/buddies/hidden-applications/999999", "a", nil)
	env.want(http.StatusBadRequest, http.MethodPut, "/api/v1/me/buddies/hidden-applications/abc", "a", nil)
}

func TestBuddyConversationHappyPath(t *testing.T) {
	env := newBuddyEnv(t, "a", "b", "c")
	env.join("a")
	hb := env.join("b")
	env.join("c")

	inv := env.want(http.StatusCreated, http.MethodPost, "/api/v1/me/buddies/invite", "a",
		map[string]any{"handle": hb, "body": "hello there"})["data"].(map[string]any)
	id := inv["id"].(float64)
	if inv["status_for_me"] != "outgoing" || inv["kind"] != "buddy" || counterpartHandle(inv) != hb {
		t.Fatalf("invite = %v", inv)
	}

	// The invitee sees it as incoming, with the invitation as unread.
	list := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/conversations", "b", nil)["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["status_for_me"] != "incoming" {
		t.Fatalf("b list = %v", list)
	}
	if list[0].(map[string]any)["counterpart"].(map[string]any)["reachable"] != true {
		t.Fatalf("active member listed as unreachable: %v", list)
	}

	// A pending invitation takes no messages.
	env.want(http.StatusConflict, http.MethodPost, convPath(id, "/messages"), "a", map[string]any{"body": "too early"})
	// The inviter cannot answer their own invitation.
	env.want(http.StatusConflict, http.MethodPost, convPath(id, "/accept"), "a", nil)

	acc := env.want(http.StatusOK, http.MethodPost, convPath(id, "/accept"), "b", nil)["data"].(map[string]any)
	if acc["status_for_me"] != "open" {
		t.Fatalf("accept = %v", acc)
	}

	msg := env.want(http.StatusCreated, http.MethodPost, convPath(id, "/messages"), "a", map[string]any{"body": "first"})["data"].(map[string]any)
	if msg["mine"] != true || msg["body"] != "first" {
		t.Fatalf("send = %v", msg)
	}

	msgs := env.want(http.StatusOK, http.MethodGet, convPath(id, "/messages"), "b", nil)["data"].([]any)
	if len(msgs) < 2 {
		t.Fatalf("b messages = %v", msgs)
	}
	last := msgs[len(msgs)-1].(map[string]any)
	if last["mine"] != false {
		t.Fatalf("message mine flag = %v", last)
	}

	unread := func(user string) float64 {
		l := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/conversations", user, nil)["data"].([]any)
		return l[0].(map[string]any)["unread"].(float64)
	}
	if unread("b") == 0 {
		t.Fatal("b should have unread messages")
	}
	env.want(http.StatusNoContent, http.MethodPost, convPath(id, "/read"), "b", map[string]any{"up_to": last["id"]})
	if got := unread("b"); got != 0 {
		t.Fatalf("unread after read = %v", got)
	}

	// An outsider sees nothing of it.
	env.want(http.StatusNotFound, http.MethodGet, convPath(id, "/messages"), "c", nil)
	env.want(http.StatusNotFound, http.MethodPost, convPath(id, "/accept"), "c", nil)
	env.want(http.StatusNotFound, http.MethodPost, convPath(id, "/block"), "c", nil)
	env.want(http.StatusNotFound, http.MethodPost, convPath(id, "/report"), "c", map[string]any{"reason": "spam"})

	env.want(http.StatusNoContent, http.MethodPost, convPath(id, "/block"), "b", nil)
	closed := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/conversations", "a", nil)["data"].([]any)
	if len(closed) != 1 || closed[0].(map[string]any)["status_for_me"] != "closed" {
		t.Fatalf("a list after block = %v", closed)
	}
	// A blocked pair stop being identified to each other.
	if cp := closed[0].(map[string]any)["counterpart"].(map[string]any); cp["reachable"] != false || cp["detail"] != nil {
		t.Fatalf("blocked member still identified: %v", cp)
	}
}

func TestBuddyDeclineIsNotRevealed(t *testing.T) {
	env := newBuddyEnv(t, "a", "b")
	env.join("a")
	hb := env.join("b")
	id := env.want(http.StatusCreated, http.MethodPost, "/api/v1/me/buddies/invite", "a",
		map[string]any{"handle": hb, "body": "hi"})["data"].(map[string]any)["id"].(float64)
	env.want(http.StatusOK, http.MethodPost, convPath(id, "/decline"), "b", nil)
	l := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/conversations", "a", nil)["data"].([]any)
	if len(l) != 1 || l[0].(map[string]any)["status_for_me"] != "outgoing" {
		t.Fatalf("declined invitation must still read outgoing: %v", l)
	}
}

func TestBuddyInviteReopensConversationClosedByLeaving(t *testing.T) {
	env := newBuddyEnv(t, "a", "b")
	env.join("a")
	hb := env.join("b")
	url := "/api/v1/me/buddies/invite"
	id := env.want(http.StatusCreated, http.MethodPost, url, "a",
		map[string]any{"handle": hb, "body": "hi"})["data"].(map[string]any)["id"].(float64)
	env.want(http.StatusOK, http.MethodPost, convPath(id, "/accept"), "b", nil)
	env.want(http.StatusNoContent, http.MethodDelete, "/api/v1/me/buddies", "b", nil)
	env.join("b")

	again := env.want(http.StatusCreated, http.MethodPost, url, "a",
		map[string]any{"handle": hb, "body": "back?"})["data"].(map[string]any)
	if again["id"].(float64) != id || again["status_for_me"] != "outgoing" {
		t.Fatalf("reopened = %v, want conversation %v outgoing", again, id)
	}
}

func TestBuddyInviteErrors(t *testing.T) {
	env := newBuddyEnv(t, "a", "b")
	env.join("a")
	hb := env.join("b")
	url := "/api/v1/me/buddies/invite"

	env.want(http.StatusBadRequest, http.MethodPost, url, "a", map[string]any{"handle": hb, "body": ""})
	env.want(http.StatusBadRequest, http.MethodPost, url, "a", map[string]any{"body": "no handle"})
	env.want(http.StatusForbidden, http.MethodPost, url, "a", map[string]any{"handle": "ghost-handle-1", "body": "hi"})

	env.want(http.StatusCreated, http.MethodPost, url, "a", map[string]any{"handle": hb, "body": "hi"})
	env.want(http.StatusConflict, http.MethodPost, url, "a", map[string]any{"handle": hb, "body": "again"})
}

func TestBuddyInviteLimit(t *testing.T) {
	names := []string{"a"}
	for i := range buddy.DailyInvitationCap + 1 {
		names = append(names, "p"+strconv.Itoa(i))
	}
	env := newBuddyEnv(t, names...)
	env.join("a")
	handles := make([]string, 0, len(names)-1)
	for _, n := range names[1:] {
		handles = append(handles, env.join(n))
	}
	url := "/api/v1/me/buddies/invite"
	for _, h := range handles[:buddy.DailyInvitationCap] {
		env.want(http.StatusCreated, http.MethodPost, url, "a", map[string]any{"handle": h, "body": "hi"})
	}
	code, out, hdr := env.do(http.MethodPost, url, "a", map[string]any{"handle": handles[buddy.DailyInvitationCap], "body": "hi"})
	if code != http.StatusTooManyRequests {
		t.Fatalf("status %d body %v, want 429", code, out)
	}
	at, err := time.Parse(time.RFC3339, out["retry_at"].(string))
	if err != nil || !at.After(time.Now()) {
		t.Fatalf("retry_at = %v (%v)", out["retry_at"], err)
	}
	if secs, err := strconv.Atoi(hdr.Get("Retry-After")); err != nil || secs < 1 {
		t.Fatalf("Retry-After = %q", hdr.Get("Retry-After"))
	}
	if out["error"] == nil {
		t.Fatalf("429 body carries no error: %v", out)
	}
}

func TestBuddyReportAndModeration(t *testing.T) {
	env := newBuddyEnv(t, "a", "b", "mod")
	env.join("a")
	hb := env.join("b")
	id := env.want(http.StatusCreated, http.MethodPost, "/api/v1/me/buddies/invite", "a",
		map[string]any{"handle": hb, "body": "buy my thing"})["data"].(map[string]any)["id"].(float64)

	env.want(http.StatusBadRequest, http.MethodPost, convPath(id, "/report"), "b", map[string]any{"reason": "because"})
	env.want(http.StatusNoContent, http.MethodPost, convPath(id, "/report"), "b", map[string]any{"reason": "spam", "note": "ad"})
	env.want(http.StatusConflict, http.MethodPost, convPath(id, "/report"), "b", map[string]any{"reason": "spam"})

	env.want(http.StatusForbidden, http.MethodGet, "/api/v1/moderation/conversation-reports", "b", nil)
	env.want(http.StatusForbidden, http.MethodPost, "/api/v1/moderation/conversation-reports/1/resolve", "b", nil)

	q := env.want(http.StatusOK, http.MethodGet, "/api/v1/moderation/conversation-reports", "mod", nil)
	rows := q["data"].([]any)
	if len(rows) != 1 {
		t.Fatalf("queue = %v", q)
	}
	r := rows[0].(map[string]any)
	if r["kind"] != "buddy" || r["reason"] != "spam" || r["reported"] != ha(env, "a") || r["reporter"] != hb {
		t.Fatalf("report = %v", r)
	}
	if msgs := r["messages"].([]any); len(msgs) != 1 || msgs[0].(map[string]any)["body"] != "buy my thing" {
		t.Fatalf("report messages = %v", r["messages"])
	}

	rid := strconv.FormatInt(int64(r["id"].(float64)), 10)
	env.want(http.StatusNoContent, http.MethodPost, "/api/v1/moderation/conversation-reports/"+rid+"/resolve", "mod", nil)
	env.want(http.StatusNotFound, http.MethodPost, "/api/v1/moderation/conversation-reports/999999/resolve", "mod", nil)
	if open := env.want(http.StatusOK, http.MethodGet, "/api/v1/moderation/conversation-reports", "mod", nil)["data"].([]any); len(open) != 0 {
		t.Fatalf("resolved report still queued: %v", open)
	}
	if all := env.want(http.StatusOK, http.MethodGet, "/api/v1/moderation/conversation-reports?include_resolved=true", "mod", nil)["data"].([]any); len(all) != 1 {
		t.Fatalf("include_resolved = %v", all)
	}
}

// ha is the handle a seeded user joined under.
func ha(env *buddyEnv, user string) string {
	out := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies", user, nil)
	return out["data"].(map[string]any)["handle"].(string)
}

func TestBuddyMemberActivity(t *testing.T) {
	env := newBuddyEnv(t, "a", "b")
	handleA := env.join("a")
	handleB := env.join("b")
	ctx := context.Background()
	var app int64
	if err := env.pool.QueryRow(ctx, `INSERT INTO applications (user_id, company_slug, applied_at) VALUES ($1, 'acme', now()) RETURNING id`,
		env.users["b"]).Scan(&app); err != nil {
		t.Fatal(err)
	}
	// Two applications two days ago and a rejection, which the grid never counts.
	for _, ev := range [][2]string{{"applied", ""}, {"applied", ""}, {"stage_set", "rejected"}} {
		if _, err := env.pool.Exec(ctx, `INSERT INTO application_events
			(user_id, application_id, company_slug, kind, signal, occurred_at, source)
			VALUES ($1, $2, 'acme', $3, $4, (date_trunc('day', now() AT TIME ZONE 'UTC') - interval '2 days' + interval '12 hours') AT TIME ZONE 'UTC', 'user')`,
			env.users["b"], app, ev[0], ev[1]); err != nil {
			t.Fatal(err)
		}
	}

	path := "/api/v1/me/buddies/profiles/" + handleB + "/activity"
	env.want(http.StatusUnauthorized, http.MethodGet, path, "", nil)
	data := env.want(http.StatusOK, http.MethodGet, path, "a", nil)["data"].(map[string]any)
	days := data["days"].([]any)
	if len(days) != 1 {
		t.Fatalf("days = %v", days)
	}
	day := days[0].(map[string]any)
	if day["count"] != float64(2) || day["day"] != time.Now().UTC().AddDate(0, 0, -2).Format(time.DateOnly) {
		t.Fatalf("day = %v", day)
	}
	// An empty year is an empty list, never null.
	empty := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/profiles/"+handleA+"/activity", "b", nil)["data"].(map[string]any)
	if l, ok := empty["days"].([]any); !ok || len(l) != 0 {
		t.Fatalf("empty activity = %v", empty)
	}
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/"+handleA+"/activity", "a", nil)
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/nobody-here/activity", "a", nil)
}

// Personas are never joined: nothing a buddy conversation answers — to either member or to
// a moderator — may carry the account behind a handle, by id or by email.
func TestBuddyPayloadsNeverNameTheAccount(t *testing.T) {
	env := newBuddyEnv(t, "a", "b", "mod")
	env.join("a")
	hb := env.join("b")

	var payloads []string
	invite := env.raw(http.StatusCreated, http.MethodPost, "/api/v1/me/buddies/invite", "a",
		map[string]any{"handle": hb, "body": "hello there"})
	payloads = append(payloads, invite)
	var inv struct {
		Data struct {
			ID          float64 `json:"id"`
			Counterpart struct {
				Persona string `json:"persona"`
			} `json:"counterpart"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(invite), &inv); err != nil || inv.Data.Counterpart.Persona != "buddy" {
		t.Fatalf("invite = %s (%v)", invite, err)
	}
	id := inv.Data.ID
	payloads = append(payloads,
		env.raw(http.StatusOK, http.MethodPost, convPath(id, "/accept"), "b", nil),
		env.raw(http.StatusCreated, http.MethodPost, convPath(id, "/messages"), "b", map[string]any{"body": "hi back"}),
	)
	for _, user := range []string{"a", "b"} {
		payloads = append(payloads,
			env.raw(http.StatusOK, http.MethodGet, "/api/v1/me/conversations", user, nil),
			env.raw(http.StatusOK, http.MethodGet, convPath(id, "/messages"), user, nil),
		)
	}
	env.raw(http.StatusNoContent, http.MethodPost, convPath(id, "/report"), "a", map[string]any{"reason": "spam"})
	payloads = append(payloads, env.raw(http.StatusOK, http.MethodGet, "/api/v1/moderation/conversation-reports", "mod", nil))

	for _, user := range []string{"a", "b"} {
		id := strconv.FormatInt(env.users[user], 10)
		for _, p := range payloads {
			if strings.Contains(p, id) || strings.Contains(p, user+"@example.test") {
				t.Fatalf("payload names account %s (%s): %s", user, id, p)
			}
		}
	}
}
