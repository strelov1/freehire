// Package llmkey mints, reads and revokes the per-account credentials the LLM gateway
// knows a user by.
//
// It talks to the gateway's administrative API, which is a different surface from the
// one inference uses: `/key/generate` lives at the gateway root while chat completions
// are served under `/v1`, and the two are configured separately so the admin API need
// not be reachable wherever inference is. Nothing here makes a model call.
//
// Every method is safe on a nil client, which is what an unconfigured deployment gets.
// Absence is the off switch: no admin URL or no admin key means no minting, and every
// call in the system goes out on the one service credential exactly as it did before.
package llmkey

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

// errUnauthorized is a refused credential, and what it means depends entirely on WHICH
// credential was refused. On a self-read it is the user's own key and means exactly
// "unknown, re-mint". On an administrative call it is our configured admin key and means
// the deployment is misconfigured — reading that as a stale user key would turn one
// wrong environment variable into a re-minting storm across every account.
var errUnauthorized = fmt.Errorf("%w: credential refused", ErrUpstream)

// requestTimeout bounds one admin call. It is short because these calls sit in front of
// a user's request: minting is what stands between somebody pressing send and their
// answer starting, so a slow gateway must give up quickly and let the call proceed
// unattributed rather than hold the turn open.
const requestTimeout = 5 * time.Second

const (
	// aliasPrefix labels a key for whoever reads the gateway's own listings.
	aliasPrefix = "freehire-user-"
	// ownerPrefix namespaces the account this gateway aggregates spend under. The
	// gateway serves more than one product, so a bare id would collide with another's.
	ownerPrefix = "freehire-"
)

// Spend is what the gateway says an account has spent in the current window. Limit is 0
// when no ceiling is configured and ResetsAt is zero when the gateway reports no window —
// both are the ordinary state of a deployment that has not chosen a budget.
type Spend struct {
	Amount   float64
	Limit    float64
	ResetsAt time.Time
}

// Config is the gateway's administrative endpoint and the policy applied to new keys.
// MaxBudget, RPMLimit and BudgetWindow are omitted from the mint request when unset,
// because a zero sent explicitly reads as "nothing is allowed" rather than "no limit".
type Config struct {
	BaseURL  string
	AdminKey string

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
// internal/speech: callers ask whether they have a client and carry on without one,
// rather than every call site testing two strings.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" || cfg.AdminKey == "" {
		return nil
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: requestTimeout}}
}

// Mint issues a credential naming this account.
//
// The account is named twice on purpose. key_alias is what a human reads in the
// gateway's listings; user_id is what the gateway aggregates daily spend under, and
// without it the per-account rollup has nothing to group by. Both are namespaced,
// because this gateway serves more than one product and a bare numeric id would collide.
func (c *Client) Mint(ctx context.Context, userID int64) (string, error) {
	if c == nil {
		return "", fmt.Errorf("%w: no admin API configured", ErrUpstream)
	}
	id := strconv.FormatInt(userID, 10)
	body := map[string]any{
		"key_alias": aliasPrefix + id,
		"user_id":   ownerPrefix + id,
	}
	if c.cfg.MaxBudget > 0 {
		body["max_budget"] = c.cfg.MaxBudget
	}
	if c.cfg.RPMLimit > 0 {
		body["rpm_limit"] = c.cfg.RPMLimit
	}
	if c.cfg.BudgetWindow != "" {
		body["budget_duration"] = c.cfg.BudgetWindow
	}

	var out struct {
		Key string `json:"key"`
	}
	if err := c.post(ctx, "/key/generate", c.cfg.AdminKey, body, &out); err != nil {
		return "", err
	}
	// A 200 carrying no key happens when something between us and the gateway rewrites
	// the response. Storing "" would mark the account as credentialled for good and send
	// every later call out unattributed, which is worse than not minting at all.
	if out.Key == "" {
		return "", fmt.Errorf("%w: minted an empty key", ErrUpstream)
	}
	return out.Key, nil
}

// Spend reports what a credential has spent in the current window.
//
// The read authenticates as the credential itself rather than as the administrator, for
// two reasons. It keeps the secret out of the request URL — the gateway logs
// `$request_uri`, so a query parameter would file a live credential into an access log on
// every usage read, in plaintext, forever. And it needs no administrative rights to
// perform, so the one path a user's request can reach cannot do anything but read that
// user's own line. A key cannot mint keys (verified: the gateway answers 401).
func (c *Client) Spend(ctx context.Context, secret string) (Spend, error) {
	if c == nil {
		return Spend{}, fmt.Errorf("%w: no admin API configured", ErrUpstream)
	}
	var out struct {
		Info struct {
			Spend     float64  `json:"spend"`
			MaxBudget *float64 `json:"max_budget"`
			ResetAt   *string  `json:"budget_reset_at"`
		} `json:"info"`
	}
	err := c.get(ctx, "/key/info", secret, &out)
	// Refused here means the gateway does not know this key, because the key IS the
	// credential being checked. That is the signal to replace it.
	if errors.Is(err, errUnauthorized) {
		return Spend{}, ErrUnknownKey
	}
	if err != nil {
		return Spend{}, err
	}
	spend := Spend{Amount: out.Info.Spend}
	if out.Info.MaxBudget != nil {
		spend.Limit = *out.Info.MaxBudget
	}
	if out.Info.ResetAt != nil {
		// An unparseable instant is not worth failing a usage readout over: the amount is
		// the answer, and the reset is decoration beside it.
		if at, err := time.Parse(time.RFC3339, *out.Info.ResetAt); err == nil {
			spend.ResetsAt = at
		}
	}
	return spend, nil
}

// Block retires a credential without erasing it: it stops spending and stays in the
// gateway's records, so what was already spent with it remains attributable.
//
// This is what a departing account gets. Delete would take the gateway's own account of
// the spend along with the key, and a cost record that disappears when somebody leaves is
// not a cost record.
//
// A key the gateway has already forgotten reports success — that is the state the caller
// asked for, and account deletion must not log a fault for reaching it.
func (c *Client) Block(ctx context.Context, secret string) error {
	if c == nil {
		return nil
	}
	err := c.post(ctx, "/key/block", c.cfg.AdminKey, map[string]any{"key": secret}, nil)
	if errors.Is(err, ErrUnknownKey) {
		return nil
	}
	return err
}

// Delete erases a credential outright, records and all.
//
// It is for a key that has no records worth keeping: one minted moments ago that lost the
// race to store itself, or one nothing can reach any more. For a credential that has
// actually been spent with, use Block.
func (c *Client) Delete(ctx context.Context, secret string) error {
	if c == nil {
		return nil
	}
	err := c.post(ctx, "/key/delete", c.cfg.AdminKey, map[string]any{"keys": []string{secret}}, nil)
	if errors.Is(err, ErrUnknownKey) {
		return nil
	}
	return err
}

// post and get take the bearer credential explicitly rather than defaulting to the
// administrator's. Which credential a call travels on is a security decision, and one
// with a silent failure mode — an over-privileged read works perfectly — so it is named
// at every call site instead of being inherited.
func (c *Client) post(ctx context.Context, path, bearer string, body any, out any) error {
	blob, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", ErrUpstream, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+path, bytes.NewReader(blob))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, bearer, out)
}

func (c *Client) get(ctx context.Context, path, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	return c.do(req, bearer, out)
}

// do carries out one call and decodes its answer.
//
// 404 and 401 are singled out, but only 404 is classified here: the gateway answers it
// for a key it does not know regardless of who asked. A 401 depends on whose credential
// was refused, which only the caller knows, so it comes back as errUnauthorized for them
// to interpret. Every other non-2xx is the gateway's own problem.
//
// Error messages carry the path and never the query or the credential.
func (c *Client) do(req *http.Request, bearer string, out any) error {
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUpstream, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrUnknownKey
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
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
