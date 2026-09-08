package sources

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
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
// Measured 2026-09-07 for bayt and gulftalent: 403 from the production datacenter IP and 403
// through SOURCES_PROXY_URL, whose exit their edge classifies as datacenter too; served
// normally by the hosted API. The browser tier does not help, because the refusal comes before
// any challenge is offered.
//
// What is behind those two walls is thin, and whoever changes this should know the number
// rather than rediscover it: run through this repository's own classify.IsTech, bayt's IT
// category passed 0 of 33 titles and gulftalent's postings 5 of 400 (1.25%). They are project
// managers, professors, chefs and hotel electricians. That was measured and accepted, which is
// why every guard below exists.
//
// A build function receives BOTH transports, because the useful shape is not always "everything
// through the hosted client". wantapply is the case that proved it, and it is a different shape
// from the other two: its pages are perfectly reachable on the free .cy mirror, and only the
// ENUMERATION is missing there — .cy's sitemap lists ~605 vacancies where .com's lists 2 755,
// and five vacancies absent from the former all answered 200 on the latter's host. So one
// metered request buys the list and the 2 755 pages stay free. Its yield is also the best of
// the three by a distance: 636 of 2 753 titles (23%) pass classify.IsTech.
var firecrawlProviders = map[string]func(hosted *firecrawlClient, direct HTTPClient) Source{
	"bayt":       func(hosted *firecrawlClient, _ HTTPClient) Source { return NewBayt(hosted) },
	"gulftalent": func(hosted *firecrawlClient, _ HTTPClient) Source { return NewGulfTalent(hosted) },
	// Only the enumeration is hosted: .com lists 2 755 vacancies where .cy lists ~605, and every
	// one of them is readable on .cy. See NewWantapplyViaHostedSitemap.
	"wantapply": func(hosted *firecrawlClient, direct HTTPClient) Source {
		return NewWantapplyViaHostedSitemap(direct, hosted, wantapplyComSitemapURL, wantapplyComHostname)
	},
	// hh.ru's detail pages sit behind DDoS-Guard through the proxy (redirected to an interactive
	// image CAPTCHA, measured 2026-09-08 — not a JS challenge, so the browser tier does not help
	// either) while listing works fine on the direct IP. Only detail hydration is hosted; listing
	// gets its own fresh, unproxied client rather than the `direct` parameter, which becomes the
	// PROXIED client whenever SOURCES_PROXY_URL is set for any other provider — reusing it here
	// would put hh's listing right back on the transport this exists to stop using.
	"hh": func(hosted *firecrawlClient, _ HTTPClient) Source {
		return NewHHWithDetailGetter(NewClient(), hosted)
	},
}

const (
	// wantapplyComSitemapURL is the fuller enumeration; wantapplyComHostname is the host its
	// entries carry, which is not the host the pages are fetched from.
	wantapplyComSitemapURL = "https://wantapply.com/sitemap.xml"
	wantapplyComHostname   = "wantapply.com"
)

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
	// The free transport a mixed-tier provider keeps for the bulk of its work. It is the proxied
	// client when a proxy is configured, since the providers here are refused on the direct IP.
	direct := HTTPClient(NewClient())
	if raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL")); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			direct = NewProxyClient(u)
		}
	}
	// One client for the whole run, so the budget counts across every provider in it: what is
	// being protected is the account, and no single adapter can know what the others spent.
	c := &firecrawlClient{api: api}
	for name, build := range firecrawlProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(c, direct)
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
