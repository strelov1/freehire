// Package llmkey mints, reads and revokes the per-account credentials the LLM gateway
// knows a user by. It uses the gateway's admin API (`/api`), configured separately from
// inference (`/v1`); nothing here makes a model call.
//
// Every method is safe on a nil client (an unconfigured deployment): no minting, and every
// call goes out on the one service credential.
package llmkey

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrUpstream wraps every failure on the gateway's side. Callers treat it as "attribution
// unavailable", fall back to the service credential, and never fail the user's request.
var ErrUpstream = errors.New("llm gateway")

// ErrUnknownKey reports that the gateway does not recognise a credential we hold: the
// stored value is worthless and should be replaced. Distinct from ErrUpstream, where the
// value is probably fine and discarding it would orphan a live credential.
var ErrUnknownKey = fmt.Errorf("%w: key not recognised", ErrUpstream)

// errNameTaken is the gateway refusing a create because a key already carries the name.
var errNameTaken = fmt.Errorf("%w: key name already taken", ErrUpstream)

// requestTimeout bounds one admin call. Short because minting sits in front of a user's
// request: a slow gateway must give up and let the call go unattributed.
const requestTimeout = 5 * time.Second

// keysPath is the governance collection for credentials. Written once: a typo in one verb
// would 404, which Block and Delete read as "already done".
const keysPath = "/api/governance/virtual-keys"

// namePrefix labels a key in the gateway's listings; spend is grouped by the credential's
// own id, not by this name.
const namePrefix = "freehire-user-"

// Credential is one account's key at the gateway. The Secret is the bearer token a model
// call presents; the ID is opaque and the only thing block and delete accept (the secret
// answers 404). Storing one without the other gives a key that can spend but not be revoked.
type Credential struct {
	ID     string
	Secret string
}

// Config is the gateway's admin endpoint, its administrator, and the policy for new keys.
// A zero or negative MaxBudget or RPMLimit, or an empty BudgetWindow, is omitted from the
// mint request, so a new key gets no limit for it.
type Config struct {
	BaseURL       string
	AdminUsername string
	AdminPassword string

	// TemplateKey is the id of the virtual key whose provider policy every minted credential
	// copies (see Mint), keeping the provider vocabulary out of this repository.
	TemplateKey string

	MaxBudget    float64
	RPMLimit     int
	BudgetWindow string
}

// Client is the administrative API of one gateway.
type Client struct {
	cfg  Config
	http *http.Client
}

// New builds a client, or returns nil when the admin API is not configured, so callers
// carry on without one (as in internal/ai/speech).
//
// TemplateKey counts as configuration: without it every minted credential would be refused
// by every provider, an outage that looks like a model problem.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" || cfg.AdminUsername == "" || cfg.AdminPassword == "" || cfg.TemplateKey == "" {
		return nil
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: requestTimeout}}
}

// providerConfig is one entry of a key's provider allowlist, copied verbatim from the
// template; this package knows no provider's name.
type providerConfig struct {
	Provider      string   `json:"provider"`
	Weight        *float64 `json:"weight,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	KeyIDs        []string `json:"key_ids,omitempty"`
}

// virtualKey is the gateway's own shape for a credential, in both directions.
type virtualKey struct {
	ID              string           `json:"id"`
	Value           string           `json:"value"`
	ProviderConfigs []providerConfig `json:"provider_configs"`
}

// envelope covers both governance answer shapes: the key at the top level or wrapped.
type envelope struct {
	VirtualKey *virtualKey `json:"virtual_key"`
	virtualKey
}

func (e envelope) key() virtualKey {
	if e.VirtualKey != nil {
		return *e.VirtualKey
	}
	return e.virtualKey
}

// Mint issues a credential naming this account. It takes two calls: a key with no provider
// policy is refused every provider, so the policy is read from the template key. That keeps
// the provider list and weights in the gateway's configuration.
func (c *Client) Mint(ctx context.Context, userID int64) (Credential, error) {
	if c == nil {
		return Credential{}, fmt.Errorf("%w: no admin API configured", ErrUpstream)
	}

	policy, err := c.policy(ctx)
	if err != nil {
		return Credential{}, err
	}

	name := namePrefix + strconv.FormatInt(userID, 10)
	minted, err := c.create(ctx, name, policy)
	// The name is taken by the account's own earlier credential: Forget blocks a key rather
	// than deleting it, and the name stays. Retry with a suffix, keeping the account's name
	// as the prefix spend reports group by.
	if errors.Is(err, errNameTaken) {
		minted, err = c.create(ctx, name+"-r"+strconv.FormatInt(time.Now().Unix(), 10), policy)
	}
	return minted, err
}

// create posts one virtual key under name with the policy Mint read.
func (c *Client) create(ctx context.Context, name string, policy []providerConfig) (Credential, error) {
	body := map[string]any{
		"name":             name,
		"description":      "per-user spend attribution",
		"is_active":        true,
		"provider_configs": policy,
	}
	if c.cfg.MaxBudget > 0 {
		budget := map[string]any{"max_limit": c.cfg.MaxBudget}
		if c.cfg.BudgetWindow != "" {
			budget["reset_duration"] = c.cfg.BudgetWindow
		}
		body["budgets"] = []any{budget}
	}
	if c.cfg.RPMLimit > 0 {
		body["rate_limit"] = map[string]any{
			"request_max_limit":      c.cfg.RPMLimit,
			"request_reset_duration": "1m",
		}
	}

	var out envelope
	if err := c.do(ctx, http.MethodPost, keysPath, body, &out); err != nil {
		return Credential{}, err
	}
	minted := out.key()
	// A 2xx with neither field means something rewrote the response; a half credential
	// would leave the account unusable or unrevokable.
	if minted.Value == "" || minted.ID == "" {
		return Credential{}, fmt.Errorf("%w: minted an incomplete credential", ErrUpstream)
	}
	return Credential{ID: minted.ID, Secret: minted.Value}, nil
}

// policy is the provider allowlist for a new credential, read from the template key.
// key_ids is normalised: a read answers null where a write requires ["*"], and echoing the
// null would pin the key to no provider key.
func (c *Client) policy(ctx context.Context) ([]providerConfig, error) {
	var tpl envelope
	if err := c.do(ctx, http.MethodGet, keysPath+"/"+url.PathEscape(c.cfg.TemplateKey), nil, &tpl); err != nil {
		return nil, fmt.Errorf("read policy template: %w", err)
	}
	configs := tpl.key().ProviderConfigs
	if len(configs) == 0 {
		return nil, fmt.Errorf("%w: policy template %q allows no provider", ErrUpstream, c.cfg.TemplateKey)
	}
	for i := range configs {
		if len(configs[i].KeyIDs) == 0 {
			configs[i].KeyIDs = []string{"*"}
		}
	}
	return configs, nil
}

// Block retires a credential without erasing it: it stops spending and stays in the
// gateway's listings. A key the gateway already forgot reports success, so account
// deletion does not log a fault for it.
func (c *Client) Block(ctx context.Context, keyID string) error {
	if c == nil || keyID == "" {
		return nil
	}
	err := c.do(ctx, http.MethodPut, keysPath+"/"+url.PathEscape(keyID),
		map[string]any{"is_active": false}, nil)
	if errors.Is(err, ErrUnknownKey) {
		return nil
	}
	return err
}

// Delete erases a credential outright, for a key with nothing worth keeping (minted moments
// ago and lost the race to be stored, or unreachable).
func (c *Client) Delete(ctx context.Context, keyID string) error {
	if c == nil || keyID == "" {
		return nil
	}
	err := c.do(ctx, http.MethodDelete, keysPath+"/"+url.PathEscape(keyID), nil, nil)
	if errors.Is(err, ErrUnknownKey) {
		return nil
	}
	return err
}

// Activity is what an account did over a window: model calls, failures and tokens. No
// money: the gateway's cost is a list price against a mixed pool, not our cost or their
// price.
type Activity struct {
	Requests int
	Failed   int
	Tokens   int
}

// Activity reports what one credential did between two dates, inclusive.
//
// It makes two calls because the gateway reports a success rate, and deriving a failure
// count from it rounds; the second call filters to failures. The window is widened to whole
// days, since the gateway works in instants and a literal from/to would drop the start of
// the first day. The key id is opaque, not a secret, so it travels in the query string.
func (c *Client) Activity(ctx context.Context, keyID string, from, to time.Time) (Activity, error) {
	if c == nil {
		return Activity{}, fmt.Errorf("%w: no admin API configured", ErrUpstream)
	}
	if keyID == "" {
		// An account that never had a credential did nothing: zeroes, not an error.
		return Activity{}, nil
	}

	window := url.Values{
		"virtual_key_ids": {keyID},
		"start_time":      {from.UTC().Truncate(24 * time.Hour).Format(time.RFC3339)},
		"end_time":        {to.UTC().Truncate(24 * time.Hour).Add(24*time.Hour - time.Nanosecond).Format(time.RFC3339Nano)},
	}

	var total struct {
		Requests int `json:"total_requests"`
		Tokens   int `json:"total_tokens"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/logs/stats?"+window.Encode(), nil, &total); err != nil {
		return Activity{}, err
	}

	// Narrowed in place: url.Values is a map, so copying it would alias it anyway.
	window.Set("status", "error")
	var errs struct {
		Requests int `json:"total_requests"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/logs/stats?"+window.Encode(), nil, &errs); err != nil {
		return Activity{}, err
	}

	return Activity{Requests: total.Requests, Failed: errs.Requests, Tokens: total.Tokens}, nil
}

// do carries out one administrative call and decodes its answer. It always authenticates
// as the administrator, so it takes no credential.
//
// 404 means an unknown key, which Block and Delete treat as already done. Every other
// non-2xx, including 401, is the gateway's problem: a 401 means OUR admin credentials are
// wrong, and treating it as a stale user key would cause a re-minting storm.
//
// Error messages carry the path, never the query or credentials.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	// A nil blob is an empty reader, which GET and DELETE want.
	var blob []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%w: encode request: %v", ErrUpstream, err)
		}
		blob = encoded
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, bytes.NewReader(blob))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	if blob != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth(c.cfg.AdminUsername, c.cfg.AdminPassword)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUpstream, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrUnknownKey
	}
	if resp.StatusCode == http.StatusConflict {
		return errNameTaken
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: %s: status %d", ErrUpstream, req.URL.Path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: %s: decode response: %v", ErrUpstream, req.URL.Path, err)
	}
	return nil
}
