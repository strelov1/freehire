package sources

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/firecrawl"
)

// firecrawlProviders is the opt-in allowlist of providers that refuse EVERY address this
// repository can egress from, so their crawl goes through a hosted scraping API.
//
// This is the third and last resort, and the order matters. proxiedProviders answers "our IP
// is blocked but another one is not". browserProviders answers "the page needs JavaScript
// run". This one answers "no address we can obtain is served at all" — and it is the only one
// that costs money per page, so nothing lands here that either of the others can reach.
//
// Measured 2026-09-07, both providers: 403 from the production datacenter IP and 403 through
// SOURCES_PROXY_URL, whose exit their edge classifies as datacenter too; served normally by
// the hosted API. The browser tier does not help, because the refusal comes before any
// challenge is offered.
//
// What is behind those walls is thin, and whoever changes this should know the number rather
// than rediscover it: run through this repository's own classify.IsTech, bayt's IT category
// passed 0 of 33 titles and gulftalent's postings 5 of 400 (1.25%). They are project managers,
// professors, chefs and hotel electricians. This tier was built with that measured and
// accepted, which is why every guard below exists.
var firecrawlProviders = map[string]func(*firecrawlClient) Source{
	"bayt":       func(c *firecrawlClient) Source { return NewBayt(c) },
	"gulftalent": func(c *firecrawlClient) Source { return NewGulfTalent(c) },
}

// defaultFirecrawlBudget is how many pages one run may fetch when nothing says otherwise.
// Deliberately small against a site listing 60 000 postings: the difference between a bounded
// run and an unbounded one is the difference between a nightly trickle and a month's
// allowance gone by morning. Raising it is a decision with a number attached.
const defaultFirecrawlBudget = 500

// firecrawlClient adapts the hosted fetch to the two transports the target adapters declare —
// baytHTTP is HTMLGetter, gulftalentHTTP is XMLGetter + HTMLGetter — so NEITHER ADAPTER
// CHANGES. What is wrong with these providers is the address a request leaves from, not how
// the response is read.
type firecrawlClient struct {
	api *firecrawl.Client
}

var (
	_ XMLGetter  = (*firecrawlClient)(nil)
	_ HTMLGetter = (*firecrawlClient)(nil)
)

// firecrawlStatusError renders a TARGET status as the same typed error the plain client
// produces, so detailUnreadable and isRateLimited keep reading it. The status is the target's,
// never the vendor's: the vendor answers 200 while reporting a refusal inside.
func firecrawlStatusError(url string, status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return &StatusError{Method: http.MethodGet, Code: status, URL: url}
}

func (c *firecrawlClient) get(ctx context.Context, url string) ([]byte, error) {
	status, body, err := c.api.Fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("sources: hosted GET %s: %w", url, err)
	}
	if err := firecrawlStatusError(url, status); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *firecrawlClient) GetXML(ctx context.Context, url string, v any) error {
	body, err := c.get(ctx, url)
	if err != nil {
		return err
	}
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil {
		return fmt.Errorf("sources: hosted GET %s: decode xml: %w", url, err)
	}
	return nil
}

func (c *firecrawlClient) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	node, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sources: hosted GET %s: parse html: %w", url, err)
	}
	return node, nil
}

// ApplyFirecrawlEgress rewires the firecrawlProviders in registry onto the hosted API.
//
// It MUST be applied LAST, after ApplyProxyEgress and ApplyBrowserEgress. gulftalent is in
// proxiedFingerprintProviders as well, and both write the same registry entry — the override
// is wanted (that transport is measured as refused, this one as served) so it is ordered and
// tested rather than left to whichever ran last. Without a key gulftalent keeps the
// fingerprint transport exactly as today, which is what makes an unconfigured deployment
// bit-for-bit unchanged.
//
// No key means no tier and no client, so merging this can spend nothing. An unusable page
// budget is an error rather than a fallback: it is the one knob that decides what a night
// costs, and a typo in it must not be quietly absorbed.
func ApplyFirecrawlEgress(registry map[string]Source) error {
	key := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if key == "" {
		return nil
	}
	if !anyFirecrawlProviderRegistered(registry) {
		return nil
	}
	budget, err := firecrawlBudget(os.Getenv("FIRECRAWL_MAX_PAGES_PER_RUN"))
	if err != nil {
		return err
	}
	api, err := firecrawl.New(firecrawl.Config{APIKey: key, MaxPagesPerRun: budget})
	if err != nil {
		return fmt.Errorf("sources: hosted tier: %w", err)
	}
	// One client for the whole run, so the budget counts across every provider in it: what is
	// being protected is the account, and no single adapter can know what the others spent.
	c := &firecrawlClient{api: api}
	for name, build := range firecrawlProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(c)
		}
	}
	return nil
}

func anyFirecrawlProviderRegistered(registry map[string]Source) bool {
	for name := range firecrawlProviders {
		if _, ok := registry[name]; ok {
			return true
		}
	}
	return false
}

// firecrawlBudget resolves FIRECRAWL_MAX_PAGES_PER_RUN, defaulting when unset and refusing
// anything that is not a positive number of pages.
func firecrawlBudget(env string) (int64, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return defaultFirecrawlBudget, nil
	}
	n, err := strconv.ParseInt(env, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("sources: FIRECRAWL_MAX_PAGES_PER_RUN must be a positive number of pages, got %q", env)
	}
	return n, nil
}
