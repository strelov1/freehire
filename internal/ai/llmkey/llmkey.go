// Package llmkey mints, reads and revokes the per-account credentials the LLM gateway
// knows a user by.
//
// It talks to the gateway's administrative API, which is a different surface from the one
// inference uses: governance lives under `/api` while chat completions are served under
// `/v1`, and the two are configured separately so the admin API need not be reachable
// wherever inference is. Nothing here makes a model call.
//
// Every method is safe on a nil client, which is what an unconfigured deployment gets.
// Absence is the off switch: no admin URL or no administrator means no minting, and every
// call in the system goes out on the one service credential exactly as it did before.
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

// ErrUpstream is what every failure on the gateway's side wraps: a refusal, a fault, an
// answer we cannot read. Callers treat it as "attribution is unavailable right now" and
// fall back to the service credential — never as a reason to fail the user's request.
var ErrUpstream = errors.New("llm gateway")

// ErrUnknownKey reports that the gateway does not recognise a credential we hold. It is
// deliberately distinct from ErrUpstream: this one means the stored value is worthless
// and should be replaced, while a general fault means the value is probably fine and
// throwing it away would orphan a live credential.
var ErrUnknownKey = fmt.Errorf("%w: key not recognised", ErrUpstream)

// requestTimeout bounds one admin call. It is short because these calls sit in front of
// a user's request: minting is what stands between somebody pressing send and their
// answer starting, so a slow gateway must give up quickly and let the call proceed
// unattributed rather than hold the turn open.
const requestTimeout = 5 * time.Second

// keysPath is the governance collection every credential is created under and addressed
// beneath. Written once because a typo in one of the three verbs that use it would fail as
// a 404, which Block and Delete both read as "already done".
const keysPath = "/api/governance/virtual-keys"

// namePrefix labels a key for whoever reads the gateway's own listings. It is the only
// place the account id appears at the gateway: spend is grouped by the credential's own
// identifier, which we store, so nothing here has to double as a foreign key.
const namePrefix = "freehire-user-"

// Credential is one account's key at the gateway: what its calls present, and what an
// administrative call addresses.
//
// Two fields because this gateway separates them. The secret is a bearer token and the
// only thing a model call needs; the ID is opaque and the only thing block and delete
// accept — aiming either at the secret answers 404. Storing one without the other yields
// a credential that can spend but not be revoked, which is what account deletion needs.
type Credential struct {
	ID     string
	Secret string
}

// Config is the gateway's administrative endpoint, the administrator it authenticates as,
// and the policy applied to new keys.
//
// MaxBudget, RPMLimit and BudgetWindow are omitted from the mint request when unset,
// because a zero sent explicitly reads as "nothing is allowed" rather than "no limit".
type Config struct {
	BaseURL       string
	AdminUsername string
	AdminPassword string

	// TemplateKey is the id of the virtual key whose provider policy every minted
	// credential copies. See Mint: without a policy a new key is refused everything,
	// and reading one rather than carrying one is what keeps the provider vocabulary
	// out of this repository.
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

// New builds a client, or returns nil when the admin API is not configured.
//
// Nil is the "this deployment does not attribute spend" answer, following
// internal/ai/speech: callers ask whether they have a client and carry on without one,
// rather than every call site testing four strings.
//
// TemplateKey counts as configuration. A client without one could mint, and every
// credential it minted would be refused by every provider — an outage that looks like a
// model problem and is really a missing environment variable.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" || cfg.AdminUsername == "" || cfg.AdminPassword == "" || cfg.TemplateKey == "" {
		return nil
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: requestTimeout}}
}

// providerConfig is one entry of a virtual key's provider allowlist. The fields are
// carried through verbatim from the template to the new key; this package does not
// interpret them and deliberately knows no provider's name.
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

// envelope covers both answer shapes the governance API uses: some routes return the key
// at the top level, others wrap it. Reading both is cheaper than depending on which.
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

// Mint issues a credential naming this account.
//
// It takes two calls, and the first one is not optional. A virtual key with no provider
// policy is refused every provider — deny by default — so a key minted without one is
// born useless, and usefully so only in the sense that the failure is total rather than
// subtle. The policy could have been carried in configuration here; instead it is read
// from the template key, so the list of providers and the weights between them stay in
// the gateway's own configuration where they are already maintained, and adding a
// provider never becomes a deployment of this service.
func (c *Client) Mint(ctx context.Context, userID int64) (Credential, error) {
	if c == nil {
		return Credential{}, fmt.Errorf("%w: no admin API configured", ErrUpstream)
	}

	policy, err := c.policy(ctx)
	if err != nil {
		return Credential{}, err
	}

	body := map[string]any{
		"name":             namePrefix + strconv.FormatInt(userID, 10),
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
	// A 2xx carrying neither field happens when something between us and the gateway
	// rewrites the response. Storing a half credential would mark the account as
	// credentialled for good while leaving it unusable or unrevokable.
	if minted.Value == "" || minted.ID == "" {
		return Credential{}, fmt.Errorf("%w: minted an incomplete credential", ErrUpstream)
	}
	return Credential{ID: minted.ID, Secret: minted.Value}, nil
}

// policy is the provider allowlist a new credential is born with, read from the template
// key rather than carried here.
//
// key_ids is normalised rather than echoed: a read answers null where a write requires
// ["*"], and passing the null straight back through mints a key pinned to no provider key
// at all.
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
// gateway's listings, so what it was is still legible to whoever reads them.
//
// This is what a departing account gets. On the previous gateway it was also how the
// spend record survived, because that record hung off the key; here the usage log is
// keyed by the credential's id and outlives the credential itself, so the distinction
// between this and Delete is now about legibility rather than about losing history.
//
// A key the gateway has already forgotten reports success — that is the state the caller
// asked for, and account deletion must not log a fault for reaching it.
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

// Delete erases a credential outright.
//
// It is for a key with nothing worth keeping in the listings: one minted moments ago that
// lost the race to store itself, or one nothing can reach any more.
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

// Activity is what an account did over a window: how many model calls it made, how many
// of them failed, and the tokens they moved.
//
// Deliberately no money. This is what a person is shown about their own use, and the
// gateway's cost figure is a list price against a mixed upstream pool — not our cost, and
// certainly not their price, which is measured in credits.
type Activity struct {
	Requests int
	Failed   int
	Tokens   int
}

// Activity reports what one credential did between two dates, inclusive.
//
// Two calls rather than one, because the gateway reports a success RATE and not a failure
// count, and deriving one from the other rounds: 7 of 9 is 77.77…%, and turning that back
// into "2 failed" is arithmetic on a display value. The second call asks the same question
// filtered to failures and reads the count directly.
//
// The window is widened to whole days at both ends. The caller works in dates, the gateway
// in instants, and a from/to pair taken literally would silently drop everything that
// happened earlier on the first day.
//
// This is an administrative read keyed by the credential id — opaque, not a secret, so it
// travels in a query string where the credential could not.
func (c *Client) Activity(ctx context.Context, keyID string, from, to time.Time) (Activity, error) {
	if c == nil {
		return Activity{}, fmt.Errorf("%w: no admin API configured", ErrUpstream)
	}
	if keyID == "" {
		// An account that never had a credential did nothing, which is an answer and
		// not a fault: the usage page renders zeroes rather than an error.
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

	// Narrowed in place rather than through a copy. url.Values is a map, so `failed :=
	// window` would alias it and the "widen the window" reasoning above would silently
	// depend on the totals having already been read. One value, narrowed after use, says
	// what is happening.
	window.Set("status", "error")
	var errs struct {
		Requests int `json:"total_requests"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/logs/stats?"+window.Encode(), nil, &errs); err != nil {
		return Activity{}, err
	}

	return Activity{Requests: total.Requests, Failed: errs.Requests, Tokens: total.Tokens}, nil
}

// do carries out one administrative call and decodes its answer.
//
// Every call here authenticates as the administrator and there is no second credential to
// choose from, so unlike its predecessor this takes none: the gateway's management surface
// admits exactly one identity, and offering a parameter would imply a decision that does
// not exist.
//
// 404 is singled out: the gateway answers it for a key it does not know, which is the
// signal Block and Delete treat as already done. Every other non-2xx, including a 401, is
// the gateway's own problem — a 401 here means OUR administrator credentials are wrong,
// never that a user's key went stale, and conflating the two would turn one mistyped
// environment variable into a re-minting storm.
//
// Error messages carry the path and never the query or the credentials.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	// A nil blob is an empty reader, which is what GET and DELETE want. Marshalling
	// only when there is a body keeps the two shapes on one path.
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
