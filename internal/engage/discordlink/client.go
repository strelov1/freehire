package discordlink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// discordAPIBase is Discord's REST root, pinned to v10: Discord retires versions
// on notice and a changed body shape would look like our bug.
const discordAPIBase = "https://discord.com/api/v10"

// Discord's error codes. Only the two this feature can provoke are named.
const (
	codeUnknownMember      = 10007
	codeMissingPermissions = 50013
)

// maxRetryAfter caps how long a rate limit may hold one call. A larger
// retry_after means a global limit, and waiting it out would spend the run on
// one account.
const maxRetryAfter = 5 * time.Second

var (
	// ErrUnknownMember means the Discord account is not in the guild. It is an
	// absence, not a failure: a run going red for it would bury real failures.
	ErrUnknownMember = errors.New("discord: member is not in the guild")

	// ErrMissingPermissions means Discord refused the role change. Almost always the role
	// hierarchy — see the wrapping message.
	ErrMissingPermissions = errors.New("discord: missing permissions")
)

// ClientConfig is everything the client needs to authenticate and to know which role on
// which server it manages.
type ClientConfig struct {
	ClientID     string
	ClientSecret string
	BotToken     string
	GuildID      string
	PaidRoleID   string
}

// Client talks to Discord's REST API. There is no gateway connection or bot
// process; every call is request/response, so it runs in a handler or cron worker.
type Client struct {
	cfg  ClientConfig
	http *http.Client
	// baseURL is overridden by tests. Not a config knob: pointing production at
	// another host would hand a bot token to it.
	baseURL string
}

// NewClient builds a client for one Discord application and one guild.
// A plain http.Client rather than safehttp, as in internal/engage/socialdigest:
// the host is a fixed vendor endpoint from operator config, so there is no SSRF
// surface.
func NewClient(cfg ClientConfig) *Client {
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: discordAPIBase,
	}
}

// ExchangeCode turns an authorization code into the user's access token.
// ExchangeCode turns an authorization code into the user's access token. The
// token is returned, not stored: it serves two more calls (read identity, add to
// guild) and keeping it would mean holding a credential per subscriber.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)

	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("discord: token exchange returned no access token")
	}
	return out.AccessToken, nil
}

// CurrentUserID reads the Discord user id the token belongs to. The binding is
// made against this id, which comes from Discord, not from the browser.
func (c *Client) CurrentUserID(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/users/@me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("discord: /users/@me returned no id")
	}
	return out.ID, nil
}

// AddGuildMember puts the user on the community server.
// Two credentials, deliberately: the bot authorises the call and the user's own
// token in the body proves they consented. Discord answers 201 when added and
// 204 when already a member; both are success, and treating 204 as an error
// would fail every re-link.
func (c *Client) AddGuildMember(ctx context.Context, discordUserID, accessToken string) error {
	body, err := json.Marshal(map[string]string{"access_token": accessToken})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/guilds/%s/members/%s", c.baseURL, c.cfg.GuildID, discordUserID),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authoriseAsBot(req)
	return c.do(req, nil)
}

// GrantPaidRole gives the Discord account the role the closed channels are gated on.
// Idempotent: Discord answers 204 whether or not the role was already held.
func (c *Client) GrantPaidRole(ctx context.Context, discordUserID string) error {
	return c.roleRequest(ctx, http.MethodPut, discordUserID)
}

// RevokePaidRole takes the role away. Idempotent, and ErrUnknownMember when the
// person has left the server (the same outcome).
func (c *Client) RevokePaidRole(ctx context.Context, discordUserID string) error {
	return c.roleRequest(ctx, http.MethodDelete, discordUserID)
}

func (c *Client) roleRequest(ctx context.Context, method, discordUserID string) error {
	req, err := http.NewRequestWithContext(ctx, method,
		fmt.Sprintf("%s/guilds/%s/members/%s/roles/%s",
			c.baseURL, c.cfg.GuildID, discordUserID, c.cfg.PaidRoleID), nil)
	if err != nil {
		return err
	}
	c.authoriseAsBot(req)
	return c.do(req, nil)
}

func (c *Client) authoriseAsBot(req *http.Request) {
	req.Header.Set("Authorization", "Bot "+c.cfg.BotToken)
}

// do sends a request, decoding a JSON body into out when non-nil, and retries
// ONCE through a rate limit. Not until success: the caller is a bounded run, a
// limit that outlives one wait means come back later (the next hourly run), and
// a loop would spend a run on one account and hide the condition from logs.
func (c *Client) do(req *http.Request, out any) error {
	// The body must be re-readable for the retry; bodies are tiny, so buffer.
	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	wait, rateLimited, err := c.attempt(req, out)
	if !rateLimited {
		return err
	}

	select {
	case <-time.After(wait):
	case <-req.Context().Done():
		return req.Context().Err()
	}

	retry := req.Clone(req.Context())
	if body != nil {
		retry.Body = io.NopCloser(bytes.NewReader(body))
	}
	_, _, err = c.attempt(retry, out)
	return err
}

// attempt is one round trip, so the response body is closed in the function that
// opened it rather than deferred inside do's loop.
func (c *Client) attempt(req *http.Request, out any) (time.Duration, bool, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	return classify(resp, out)
}

// discordError is the shape of Discord's JSON error body.
type discordError struct {
	Message    string  `json:"message"`
	Code       int     `json:"code"`
	RetryAfter float64 `json:"retry_after"`
}

// classify turns a response into a decoded success or the error to report, and
// says whether a retry could clear it.
func classify(resp *http.Response, out any) (wait time.Duration, rateLimited bool, err error) {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return 0, false, nil
		}
		return 0, false, json.NewDecoder(resp.Body).Decode(out)
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var de discordError
	_ = json.Unmarshal(raw, &de)

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return retryAfter(de), true, fmt.Errorf("discord: rate limited: %s", de.Message)
	case de.Code == codeUnknownMember || resp.StatusCode == http.StatusNotFound:
		return 0, false, ErrUnknownMember
	case de.Code == codeMissingPermissions:
		// Discord reports two causes, fixed in different places, with this one code:
		// role hierarchy, or a bot invited with no permissions at all. Name both, in
		// the order they are checked.
		return 0, false, fmt.Errorf(
			"%w: the bot needs the Manage Roles permission on the server (re-invite it with "+
				"that permission if its own role grants nothing), AND its role must sit above "+
				"the paid role in Server Settings → Roles — a bot cannot manage a role above "+
				"its own",
			ErrMissingPermissions)
	default:
		return 0, false, fmt.Errorf("discord: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
}

// retryAfter is how long to wait before the single retry, capped. Discord reports it in the
// body in seconds, as a float.
func retryAfter(de discordError) time.Duration {
	d := time.Duration(de.RetryAfter * float64(time.Second))
	if d <= 0 {
		return 50 * time.Millisecond
	}
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}
