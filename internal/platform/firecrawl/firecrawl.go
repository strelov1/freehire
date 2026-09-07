// Package firecrawl fetches a URL through a hosted scraping API, for a site that refuses
// every address this repository can egress from.
//
// It is transport, not a feature: it knows nothing about jobs, and it is the HTTP half of
// talking to a vendor — the same category as platform/browseruse, which is likewise a client
// for somebody else's hosted browser.
//
// It is NOT platform/browser's sibling, despite the similar shape. That package launches a
// process on our own host and costs nothing per page; this one calls a metered third party.
// Two things with the same shape and opposite economics should not be confused, which is why
// the budget below is part of the client rather than an afterthought a caller may forget.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// DefaultBaseURL is the vendor's API root.
const DefaultBaseURL = "https://api.firecrawl.dev"

// requestTimeout bounds one hosted fetch. Generous: the vendor is rendering a page behind a
// bot wall on our behalf, which took 1.5-3.5s in every measurement, and a slow page is worth
// waiting for once we have already paid for it.
const requestTimeout = 90 * time.Second

// ErrBudgetSpent is returned once a run has fetched as many pages as it was allowed. It is a
// sentinel so a caller can tell "we chose to stop spending" from "the fetch failed" — the
// first is a healthy run hitting its bound, the second is a problem.
var ErrBudgetSpent = errors.New("firecrawl: page budget for this run is spent")

// Config is what a client needs. Every field is required; there are no silent defaults for
// the two that decide whether money is spent and how much.
type Config struct {
	APIKey  string
	BaseURL string // defaults to DefaultBaseURL when empty
	// MaxPagesPerRun bounds how many pages ONE process may fetch. Must be positive.
	MaxPagesPerRun int64
	HTTP           *http.Client // defaults to a client with requestTimeout
}

// Client fetches pages through the vendor, counting every one against the run's budget.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client

	budget int64
	spent  atomic.Int64
}

// New builds a client, refusing the two configurations that could only end badly: no key (a
// client that cannot authenticate can do nothing but fail once per page) and no positive
// budget (an unbounded metered client is precisely the accident this package exists to make
// impossible).
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("firecrawl: no API key")
	}
	if cfg.MaxPagesPerRun <= 0 {
		return nil, fmt.Errorf("firecrawl: page budget must be positive, got %d", cfg.MaxPagesPerRun)
	}
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	return &Client{apiKey: cfg.APIKey, baseURL: base, http: hc, budget: cfg.MaxPagesPerRun}, nil
}

// scrapeResponse is the vendor's envelope. The distinction that matters is that IT answers
// 200 while reporting the TARGET's own status inside: reading the outer status as the page's
// would turn every refusal into a success carrying a bot wall as though it were content.
type scrapeResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    struct {
		RawHTML  string `json:"rawHtml"`
		Metadata struct {
			StatusCode int `json:"statusCode"`
		} `json:"metadata"`
	} `json:"data"`
}

// Fetch returns the TARGET's status and body for rawURL.
//
// A non-2xx target status is returned as data, not as an error, for the same reason the
// browser tier does it: only some statuses may be read as "this resource is gone", and
// collapsing them takes that decision away from the caller. A VENDOR failure is an error —
// it says nothing about the target, and a caller that mistook it for a refusal could conclude
// a posting is gone when only the API was down.
func (c *Client) Fetch(ctx context.Context, rawURL string) (int, []byte, error) {
	// Claimed BEFORE the request, so a refused fetch never spends the page it refuses.
	if spent := c.spent.Add(1); spent > c.budget {
		return 0, nil, fmt.Errorf("%w (%d pages)", ErrBudgetSpent, c.budget)
	}

	payload, err := json.Marshal(map[string]any{
		"url":     rawURL,
		"formats": []string{"rawHtml"},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("firecrawl: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/scrape", bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("firecrawl: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("firecrawl: fetch %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("firecrawl: read response for %s: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("firecrawl: fetch %s: api status %d", rawURL, resp.StatusCode)
	}

	var out scrapeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, nil, fmt.Errorf("firecrawl: decode response for %s: %w", rawURL, err)
	}
	if !out.Success {
		return 0, nil, fmt.Errorf("firecrawl: fetch %s: api reported failure: %s", rawURL, out.Error)
	}

	status := out.Data.Metadata.StatusCode
	if status == 0 {
		// The vendor succeeded but told us nothing about the target. Treating that as 200
		// would let an empty or error page through as content, so it is a failure of the
		// fetch rather than a status to reason about.
		return 0, nil, fmt.Errorf("firecrawl: fetch %s: api reported no target status", rawURL)
	}
	return status, []byte(out.Data.RawHTML), nil
}

// Spent reports how many pages this run has claimed, for a caller that wants to log the cost
// of a crawl. It counts claims rather than successes: a page paid for and then failed is
// still a page paid for.
func (c *Client) Spent() int64 { return c.spent.Load() }
