//go:build integration

// Integration tests for the email preference centre against a real Postgres: the
// unauthenticated read, the partial write, the RFC 8058 one-click target, and what
// every refusal is allowed to say.
//
// The last of those is the one worth having. These routes take no session, so a
// caller can present any token they like; if a refusal told them apart — forged
// signature, unknown group, deleted account — they could walk the id space and learn
// which accounts exist, one status code at a time.
//
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/platform/db"
)

// prefsSecret is the signing secret for these tests, long enough to clear the floor
// emailprefs puts on it.
const prefsSecret = "email-prefs-integration-secret-32b"

func newEmailPrefsApp(queries *db.Queries) *fiber.App {
	h := newEmailPrefsHandlers(emailprefs.NewService(queries, prefsSecret))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// A no-op limiter: throttling is registered in handler.go and is not what these
	// cases are about.
	h.register(app.Group("/api/v1"), func(c *fiber.Ctx) error { return c.Next() })
	return app
}

type prefsResp struct {
	Data struct {
		Email    string `json:"email"`
		Alerts   bool   `json:"alerts_enabled"`
		Activity bool   `json:"activity_enabled"`
		News     bool   `json:"news_enabled"`
		Searches []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
		} `json:"searches"`
	} `json:"data"`
}

// prefsCall is what a request produced, minus the response itself. The body is
// drained and closed inside doPrefs, and returning the *http.Response afterwards
// would hand every caller a closed body and make bodyclose flag each one — so the
// helper returns the two things the cases actually assert on.
type prefsCall struct {
	Status  int
	Cookies []*http.Cookie
}

func doPrefs(t *testing.T, app *fiber.App, method, path, body string) (prefsCall, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return prefsCall{Status: res.StatusCode, Cookies: res.Cookies()}, string(raw)
}

func mintPrefsToken(t *testing.T, userID int64, group emailprefs.Group) string {
	t.Helper()
	token, err := emailprefs.NewSigner(prefsSecret).Mint(userID, group)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return token
}

// seedPrefsUser makes a verified account with two email digest subscriptions and no
// notification_settings row — the state most accounts are actually in, and the one
// the complaint came from.
func seedPrefsUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	userID := insertUserForPrefs(t, pool, email)
	for _, name := range []string{"Rust in Berlin", "Senior Go, remote"} {
		var searchID int64
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, $2, '{}') RETURNING id`,
			userID, name).Scan(&searchID); err != nil {
			t.Fatalf("insert saved search: %v", err)
		}
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO subscriptions (user_id, saved_search_id, channel) VALUES ($1, $2, 'email')`,
			userID, searchID); err != nil {
			t.Fatalf("insert subscription: %v", err)
		}
	}
	return userID
}

func insertUserForPrefs(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestEmailPrefs_ReadsWithoutASession(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	userID := seedPrefsUser(t, pool, "reader@example.test")

	res, body := doPrefs(t, app, http.MethodGet,
		"/api/v1/email-prefs?t="+mintPrefsToken(t, userID, emailprefs.GroupNews), "")
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Status, body)
	}
	// No session may be established by a link somebody could have forwarded.
	if len(res.Cookies) != 0 {
		t.Errorf("the response set %d cookies; this page must never start a session", len(res.Cookies))
	}

	var got prefsResp
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if got.Data.Email != "reader@example.test" {
		t.Errorf("email = %q", got.Data.Email)
	}
	// An account with no settings row: alerts and news default on, activity off.
	if !got.Data.Alerts || !got.Data.News || got.Data.Activity {
		t.Errorf("defaults for an account with no settings row = %+v; want alerts+news on, activity off", got.Data)
	}
	if len(got.Data.Searches) != 2 {
		t.Fatalf("searches = %+v, want the two email subscriptions", got.Data.Searches)
	}
	if got.Data.Searches[0].Name != "Rust in Berlin" {
		t.Errorf("searches are not ordered by name: %+v", got.Data.Searches)
	}
}

// A partial body must not read as "turn everything off". This is the trap the
// contacts endpoint fell into once already.
func TestEmailPrefs_PartialWriteLeavesTheOtherSwitchesAlone(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	userID := seedPrefsUser(t, pool, "partial@example.test")
	token := mintPrefsToken(t, userID, emailprefs.GroupNews)

	res, body := doPrefs(t, app, http.MethodPatch, "/api/v1/email-prefs",
		`{"token":"`+token+`","news_enabled":false}`)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Status, body)
	}
	var got prefsResp
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if got.Data.News {
		t.Error("news should be off")
	}
	if !got.Data.Alerts {
		t.Error("alerts was not in the body and must keep its stored value")
	}
	if got.Data.Activity {
		t.Error("activity was not in the body and must keep its stored value (off)")
	}
}

func TestEmailPrefs_TurnsOffOneSearchAndLeavesTheOthers(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	userID := seedPrefsUser(t, pool, "one-search@example.test")
	token := mintPrefsToken(t, userID, emailprefs.GroupAlerts)

	_, body := doPrefs(t, app, http.MethodGet, "/api/v1/email-prefs?t="+token, "")
	var before prefsResp
	if err := json.Unmarshal([]byte(body), &before); err != nil {
		t.Fatalf("decode: %v", err)
	}
	target := before.Data.Searches[0].ID

	_, body = doPrefs(t, app, http.MethodPatch, "/api/v1/email-prefs",
		`{"token":"`+token+`","deactivate_searches":[`+strconv.FormatInt(target, 10)+`]}`)
	var after prefsResp
	if err := json.Unmarshal([]byte(body), &after); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	for _, s := range after.Data.Searches {
		if s.ID == target && s.Active {
			t.Errorf("search %d should be off", target)
		}
		if s.ID != target && !s.Active {
			t.Errorf("search %d was not named and must stay on", s.ID)
		}
	}
}

// A leaked link may subtract but never add. There is no wire field that turns a
// subscription back on, and an id belonging to somebody else affects nothing.
func TestEmailPrefs_CannotReactivateOrTouchAnotherAccount(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	mine := seedPrefsUser(t, pool, "mine@example.test")
	theirs := seedPrefsUser(t, pool, "theirs@example.test")

	_, body := doPrefs(t, app, http.MethodGet,
		"/api/v1/email-prefs?t="+mintPrefsToken(t, theirs, emailprefs.GroupAlerts), "")
	var other prefsResp
	if err := json.Unmarshal([]byte(body), &other); err != nil {
		t.Fatalf("decode: %v", err)
	}
	victim := other.Data.Searches[0].ID

	res, _ := doPrefs(t, app, http.MethodPatch, "/api/v1/email-prefs",
		`{"token":"`+mintPrefsToken(t, mine, emailprefs.GroupAlerts)+`","deactivate_searches":[`+strconv.FormatInt(victim, 10)+`]}`)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d; naming a stranger's id must be ignored, not reported", res.Status)
	}

	_, body = doPrefs(t, app, http.MethodGet,
		"/api/v1/email-prefs?t="+mintPrefsToken(t, theirs, emailprefs.GroupAlerts), "")
	if err := json.Unmarshal([]byte(body), &other); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, s := range other.Data.Searches {
		if s.ID == victim && !s.Active {
			t.Error("one account's token switched off another account's subscription")
		}
	}
}

// The mail client's own button: no confirmation screen, and it must stop only the
// kind of mail it was reached from.
func TestEmailPrefs_OneClickStopsOnlyItsOwnGroup(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	userID := seedPrefsUser(t, pool, "one-click@example.test")

	res, body := doPrefs(t, app, http.MethodPost,
		"/api/v1/email-prefs/one-click?t="+mintPrefsToken(t, userID, emailprefs.GroupNews),
		"List-Unsubscribe=One-Click")
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Status, body)
	}
	if !strings.Contains(body, `"unsubscribed_from":"news"`) {
		t.Errorf("the response must name what it turned off: %s", body)
	}

	_, body = doPrefs(t, app, http.MethodGet,
		"/api/v1/email-prefs?t="+mintPrefsToken(t, userID, emailprefs.GroupAlerts), "")
	var got prefsResp
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.News {
		t.Error("news should be off after one-click")
	}
	if !got.Data.Alerts {
		t.Error("one click on a campaign silenced the job alerts; it must stop only its own group")
	}
}

// RFC 8058 clients retry. A second POST must read as success, not as an error.
func TestEmailPrefs_OneClickIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	userID := seedPrefsUser(t, pool, "repeat@example.test")
	url := "/api/v1/email-prefs/one-click?t=" + mintPrefsToken(t, userID, emailprefs.GroupAlerts)

	for i := range 2 {
		res, body := doPrefs(t, app, http.MethodPost, url, "List-Unsubscribe=One-Click")
		if res.Status != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, body = %s", i+1, res.Status, body)
		}
	}
}

// The one thing these routes must never do: tell an unauthenticated caller which
// accounts exist, or leak anything about one they cannot open.
func TestEmailPrefs_EveryRefusalLooksTheSameAndNamesNobody(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	app := newEmailPrefsApp(queries)
	real := seedPrefsUser(t, pool, "real-account@example.test")
	valid := mintPrefsToken(t, real, emailprefs.GroupNews)

	cases := []struct {
		name  string
		token string
	}{
		{"missing", ""},
		{"garbage", "not-a-token"},
		{"tampered signature", valid[:len(valid)-3] + "aaa"},
		{"another secret", func() string {
			other, err := emailprefs.NewSigner("a-completely-different-secret-32b").Mint(real, emailprefs.GroupNews)
			if err != nil {
				t.Fatalf("mint: %v", err)
			}
			return other
		}()},
		{"no such account", mintPrefsToken(t, 999999, emailprefs.GroupNews)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, body := doPrefs(t, app, http.MethodGet, "/api/v1/email-prefs?t="+tc.token, "")
			if res.Status != http.StatusNotFound {
				t.Errorf("status = %d, want 404 for every refusal alike", res.Status)
			}
			for _, secret := range []string{"real-account@example.test", "Rust in Berlin", "999999"} {
				if strings.Contains(body, secret) {
					t.Errorf("a refusal leaked %q: %s", secret, body)
				}
			}
		})
	}
}
