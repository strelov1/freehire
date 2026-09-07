package discordlink

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestClient points a client at a stub standing in for Discord.
func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := NewClient(ClientConfig{
		ClientID:     "app-1",
		ClientSecret: "shh",
		BotToken:     "bot-token",
		GuildID:      "guild-1",
		PaidRoleID:   "role-1",
	})
	c.baseURL = srv.URL
	return c
}

func TestExchangeCode(t *testing.T) {
	var gotForm string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("path = %q, want /oauth2/token", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		gotForm = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"user-token","token_type":"Bearer"}`)
	}))

	tok, err := c.ExchangeCode(context.Background(), "the-code", "https://freehire.me/cb")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok != "user-token" {
		t.Errorf("token = %q, want user-token", tok)
	}
	for _, want := range []string{"grant_type=authorization_code", "code=the-code", "redirect_uri="} {
		if !strings.Contains(gotForm, want) {
			t.Errorf("form %q is missing %q", gotForm, want)
		}
	}
}

func TestCurrentUserID(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("authorization = %q, want the USER's bearer token", got)
		}
		_, _ = io.WriteString(w, `{"id":"1000000000000000001","username":"someone"}`)
	}))

	id, err := c.CurrentUserID(context.Background(), "user-token")
	if err != nil {
		t.Fatalf("CurrentUserID: %v", err)
	}
	if id != "1000000000000000001" {
		t.Errorf("id = %q, want the snowflake", id)
	}
}

func TestAddGuildMemberSendsTheUsersToken(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/guilds/guild-1/members/1000000000000000001" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// The BOT authorises the call; the user's token travels in the body as the proof
		// that they consented to being added.
		if got := r.Header.Get("Authorization"); got != "Bot bot-token" {
			t.Errorf("authorization = %q, want the bot token", got)
		}
		var body struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.AccessToken != "user-token" {
			t.Errorf("body access_token = %q, want the user's token", body.AccessToken)
		}
		w.WriteHeader(http.StatusCreated)
	}))

	if err := c.AddGuildMember(context.Background(), "1000000000000000001", "user-token"); err != nil {
		t.Fatalf("AddGuildMember: %v", err)
	}
}

// 204 means "already a member" and is as good an outcome as 201. Treating it as an error
// would make every re-link of an existing member fail.
func TestAddGuildMemberAcceptsAnExistingMember(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := c.AddGuildMember(context.Background(), "1", "user-token"); err != nil {
		t.Fatalf("AddGuildMember on an existing member: %v", err)
	}
}

func TestGrantAndRevokeRole(t *testing.T) {
	var method, path atomic.Value
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method.Store(r.Method)
		path.Store(r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bot bot-token" {
			t.Errorf("authorization = %q, want the bot token", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.Background()

	if err := c.GrantPaidRole(ctx, "1000000000000000001"); err != nil {
		t.Fatalf("GrantPaidRole: %v", err)
	}
	if method.Load() != http.MethodPut {
		t.Errorf("grant used %v, want PUT", method.Load())
	}
	if path.Load() != "/guilds/guild-1/members/1000000000000000001/roles/role-1" {
		t.Errorf("grant path = %v", path.Load())
	}

	if err := c.RevokePaidRole(ctx, "1000000000000000001"); err != nil {
		t.Fatalf("RevokePaidRole: %v", err)
	}
	if method.Load() != http.MethodDelete {
		t.Errorf("revoke used %v, want DELETE", method.Load())
	}
}

// Somebody who left the server is an absence, not a failure. If this came back as an error
// the hourly run would go red for an ordinary thing users do, and the real failures would
// be lost in it.
func TestUnknownMemberIsATypedAbsence(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Unknown Member","code":10007}`)
	}))

	err := c.RevokePaidRole(context.Background(), "1")
	if !errors.Is(err, ErrUnknownMember) {
		t.Fatalf("error = %v, want ErrUnknownMember", err)
	}
}

// The single most common way a role-granting bot is broken is the target role sitting above
// the bot's own in the server's list. Discord answers 50013, which on its own reads as a
// generic permission problem — the error must name the cause or every report of it starts
// with an hour of guessing.
func TestMissingPermissionsNamesTheRoleHierarchy(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Missing Permissions","code":50013}`)
	}))

	err := c.GrantPaidRole(context.Background(), "1")
	if !errors.Is(err, ErrMissingPermissions) {
		t.Fatalf("error = %v, want ErrMissingPermissions", err)
	}
	if !strings.Contains(err.Error(), "above") {
		t.Errorf("error %q does not mention the role hierarchy", err)
	}
}

func TestRateLimitIsRetriedOnce(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"You are being rate limited.","retry_after":0.01}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := c.GrantPaidRole(context.Background(), "1"); err != nil {
		t.Fatalf("GrantPaidRole through a rate limit: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2 (the refusal and the retry)", got)
	}
}

// A rate limit that survives the retry must surface rather than be swallowed: the run's
// bound already keeps it from hammering, and a silent success would record a grant that
// never happened.
func TestPersistentRateLimitFails(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"message":"You are being rate limited.","retry_after":0.01}`)
	}))

	if err := c.GrantPaidRole(context.Background(), "1"); err == nil {
		t.Fatal("a persistent rate limit must not read as success")
	}
}
