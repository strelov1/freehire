package sources

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// NewClient builds the default ingest HTTP client.
func NewClient() *Client { return newClientWithProxy(nil) }

// NewProxyClient builds an ingest client whose requests egress through proxy. It is
// identical to NewClient but for the proxied transport, and is handed only to the
// curated set of providers whose prod-IP is anti-bot-blocklisted (see All). The proxy
// URL — including its host and credentials — comes entirely from configuration
// (SOURCES_PROXY_URL); no proxy endpoint is hardcoded here.
func NewProxyClient(proxy *url.URL) *Client { return newClientWithProxy(proxy) }

// NewRefusalRetryClient builds a client that crawls on the DIRECT IP and falls back to proxy
// only for a request the platform refused (429/403). It suits a platform that serves the prod
// IP perfectly well until our own hourly fleet floods it — measured on teamtailor, where a
// direct request returns 200 on demand and yet 1207 of 1208 board failures fell inside the
// crawl hour. Routing such a platform entirely through the proxy (NewProxyClient) would move
// that same flood onto one shared address; sending it only what was already refused spends the
// proxy on requests that are otherwise pure loss.
func NewRefusalRetryClient(proxy *url.URL) *Client {
	c := NewClient()
	c.refusalClient = safehttp.NewClientWithProxy(15*time.Second, proxy)
	return c
}

// NewRotatingProxyClient is NewProxyClient for a ROTATING gateway, one that hands out a fresh
// exit IP per connection. Go's transport keeps a proxy tunnel open and reuses it, and a reused
// tunnel keeps its exit: measured 2026-10-25 through the Webshare gateway, four requests over one
// connection all left from 145.223.45.39 while three separate connections left from three
// addresses. So a pooled client turns "a fresh IP per request" into one IP for the whole run, and
// a platform that rate-limits by IP refuses it exactly as it refused the fixed exit. Closing the
// connection after every request is what makes the gateway rotate.
func NewRotatingProxyClient(proxy *url.URL) *Client {
	c := newClientWithProxy(proxy)
	freshConnectionPerRequest(c.httpClient)
	freshConnectionPerRequest(c.streamClient)
	return c
}

// NewRotatingRefusalRetryClient is NewRefusalRetryClient whose retry lane is a rotating gateway;
// see NewRotatingProxyClient for why its connections must not be reused.
func NewRotatingRefusalRetryClient(proxy *url.URL) *Client {
	c := NewRefusalRetryClient(proxy)
	freshConnectionPerRequest(c.refusalClient)
	return c
}

func freshConnectionPerRequest(c *http.Client) {
	if t, ok := c.Transport.(*http.Transport); ok {
		t.DisableKeepAlives = true
	}
}

// newClientWithProxy builds a Client with an optional egress proxy (nil = direct).
func newClientWithProxy(proxy *url.URL) *Client {
	return &Client{
		// Guarded transport: adapters and link-following fetch attacker-influenced
		// URLs, so the client must refuse internal/metadata targets (SSRF). A proxied
		// client relaxes the target check (the proxy resolves the target) and is only
		// given to trusted, curated providers — never the link-following path.
		httpClient:   safehttp.NewClientWithProxy(15*time.Second, proxy),
		streamClient: safehttp.NewClientWithProxy(streamTimeout, proxy),
		userAgent:    "freehire/0.1 (+https://freehire.me)",
		maxRetries:   2,
		retryDelay:   500 * time.Millisecond,
		backoff:      newHostBackoff(hostBackoffBase, hostBackoffMax),
	}
}

// longTimeout bounds a NewLongTimeoutClient request — generous enough for a legitimately
// large single-page fetch (a listing that inlines every posting rather than paginating),
// unlike streamTimeout's many-minute allowance for a throttled bulk feed trickling in.
// Sized against a real measurement: HumanBit's ~5.6 MB listing page took ~18s to fetch
// (curl -w, 2026-09-13), past the standard 15s httpClient timeout, which made every crawl
// of that board fail intermittently with "context deadline exceeded" in production.
const longTimeout = 45 * time.Second

// NewLongTimeoutClient builds an ingest client identical to NewClient but with a longer
// request timeout, for a platform whose listing page is legitimately large rather than
// paginated (e.g. HumanBit). It keeps the ordinary streamTimeout for GetStream — this only
// widens the bounded, buffered fetch path (GetJSON/GetHTML/GetText/...).
func NewLongTimeoutClient() *Client {
	c := NewClient()
	c.httpClient = safehttp.NewClientWithProxy(longTimeout, nil)
	return c
}

// NewSingleUseConnClient builds a client that dials a fresh connection per request, for a
// platform whose server mishandles one it has already answered on. It is exposed for host
// tools (harvest-boards' Profession prober, which reads the same sitemaps the crawl does
// and hits the same wall).
//
// Everything else about it is NewClient: same guard, same User-Agent, same retry budget.
// See safehttp.NewTransportWithoutKeepAlive for what the failure looks like and why a
// truncated body makes it worth a dedicated client rather than a retry.
func NewSingleUseConnClient() *Client { return newSingleUseConnClient() }

func newSingleUseConnClient() *Client {
	c := newClientWithProxy(nil)
	c.httpClient = safehttp.NewClientWithoutKeepAlive(15 * time.Second)
	c.streamClient = safehttp.NewClientWithoutKeepAlive(streamTimeout)
	return c
}

// BrowserUserAgent is a plain desktop Chrome string, for the one provider whose edge refuses
// every client that identifies itself.
//
// This is not a preference. Measured against 4dayweek.io on 2026-09-07, on the path its own
// robots.txt explicitly ALLOWS (/api/v2), Cloudflare answered 403 to every honest form —
// "freehire/0.1 (+https://freehire.me)", the same wrapped as
// "Mozilla/5.0 (compatible; freehire/0.1; +...)", and "freehire-aggregator/0.1" — and 200 only
// to a browser string carrying no identity at all. The managed rule refuses self-identification
// as such, not our name, so staying identifiable and staying readable turned out to be
// mutually exclusive there.
//
// Keep this for providers that ALLOW the path in robots.txt and refuse the client anyway. It is
// not a way around a Disallow: the fix for a disallowed path is to use an allowed one, which is
// exactly what the 4dayweek adapter now does.
//
// Exported for cmd/liveness, whose 4dayweek probe reads the same allowed /api/v2 path from
// behind the same edge.
const BrowserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"

// newBrowserUAClient is NewClient with BrowserUserAgent — same guarded transport, same retry
// budget, same everything else. Unexported, unlike newCookieClient's and
// newSingleUseConnClient's wrappers: those are exported because host tools reuse them, and
// nothing outside this package needs this one.
func newBrowserUAClient() *Client {
	c := NewClient()
	c.userAgent = BrowserUserAgent
	return c
}

// macBrowserUserAgent is a desktop Chrome string on macOS, for Feishu Hire (feishujobs), whose
// edge filters on the operating system in the string. Measured 2026-10-15 against the job
// search on both jobs.bytedance.com and a customer's <tenant>.jobs.feishu.cn: the project's own
// string, BrowserUserAgent (Linux), a Windows Chrome string and Go's default were all answered
// 405 with an empty body, while "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" alone was
// served. Neither robots.txt disallows the API path; the same line as BrowserUserAgent holds.
const macBrowserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// newMacBrowserUAClient is NewClient with macBrowserUserAgent — same guarded transport, same
// retry budget, same everything else.
func newMacBrowserUAClient() *Client {
	c := NewClient()
	c.userAgent = macBrowserUserAgent
	return c
}

// withUserAgent returns a copy of c that sends ua as its User-Agent, for a provider whose edge
// admits one client string and which must ALSO egress through whatever transport the registry
// wiring hands it (naukri, rebuilt over the proxied client by ApplyProxyEgress). The copy
// shares c's transports; the shared client itself is untouched. A nil client — the taxonomy
// path — stays nil.
func withUserAgent(c HTTPClient, ua string) HTTPClient {
	cl, ok := c.(*Client)
	if !ok || cl == nil {
		return c
	}
	cp := *cl
	cp.userAgent = ua
	return &cp
}

// compatibleUserAgent is the project's own identity in the browser-compatible form, for an edge
// that refuses any string not starting with a browser token yet does not ask for a browser.
// Measured 2026-10-15 against OCC Mundial's search pages (www.occ.com.mx/empleos/, which
// robots.txt allows): "freehire/0.1 (+https://freehire.me)", a bare "Mozilla/5.0" and the
// project string with "Mozilla/5.0" appended were all answered 403, while this string was
// served. It still names us, so prefer it to BrowserUserAgent wherever it is enough.
const compatibleUserAgent = "Mozilla/5.0 (compatible; freehire/0.1; +https://freehire.me)"

// NewCompatibleUAClient is NewClient with compatibleUserAgent — same guarded transport, same
// retry budget, same everything else. Exported for cmd/harvest-boards, whose probes meet the
// same edges as the adapters.
func NewCompatibleUAClient() *Client {
	c := NewClient()
	c.userAgent = compatibleUserAgent
	return c
}

// crawlerUserAgent identifies as Googlebot, for the one provider whose edge does the OPPOSITE
// of blocking bots: energyjobline.com's job-detail pages server-render their schema.org
// JobPosting ld+json only for a recognised search-crawler User-Agent. Both the project's own
// "freehire/0.1 (+https://freehire.me)" and a plain desktop browser string (BrowserUserAgent)
// get a client-rendered shell with no ld+json at all — verified live 2026-10-07. The site's own
// robots.txt does not disallow the /job/ path, and SSR'ing only for crawlers is how it gets
// Google's job rich snippets in the first place, so this is the intended way to read exactly
// the content it already publishes for search engines, not a bypass of a restriction.
const crawlerUserAgent = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

// newCrawlerUAClient is NewClient with crawlerUserAgent — same guarded transport, same retry
// budget, same everything else.
func newCrawlerUAClient() *Client {
	c := NewClient()
	c.userAgent = crawlerUserAgent
	return c
}

// NewCookieClient exposes newCookieClient to host tools (e.g. harvest-boards' Taleo
// prober, which reuses the session-bound Taleo adapter to validate a board).
func NewCookieClient() *Client { return newCookieClient() }

// newCookieClient builds an HTTP client that persists cookies across calls, for the one
// adapter (Taleo) whose session-bound API needs the JSESSIONID a careersection GET sets to
// carry into the searchjobs POST. The jar is host-scoped, so a single client segregates
// cookies across tenants without per-tenant sessions. cookiejar.New(nil) never errors.
func newCookieClient() *Client {
	c := NewClient()
	jar, _ := cookiejar.New(nil)
	c.httpClient.Jar = jar
	return c
}
