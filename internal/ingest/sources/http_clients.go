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

// NewProxyClient builds an ingest client whose requests egress through proxy, handed only to the
// curated providers whose prod IP is anti-bot-blocklisted (see All). The proxy URL, host and
// credentials come from configuration (SOURCES_PROXY_URL).
func NewProxyClient(proxy *url.URL) *Client { return newClientWithProxy(proxy) }

// NewRefusalRetryClient builds a client that crawls on the DIRECT IP and falls back to proxy only
// for a request the platform refused (429/403). It suits a platform that serves the prod IP fine
// until our own hourly fleet floods it; routing it entirely through the proxy would move that
// flood onto one shared address.
func NewRefusalRetryClient(proxy *url.URL) *Client {
	c := NewClient()
	c.refusalClient = safehttp.NewClientWithProxy(15*time.Second, proxy)
	return c
}

// NewRotatingProxyClient is NewProxyClient for a ROTATING gateway that hands out a fresh exit IP
// per connection. Go's transport reuses a proxy tunnel and a reused tunnel keeps its exit, so a
// pooled client would use one IP for the whole run. Closing the connection after every request
// makes the gateway rotate.
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

// longTimeout bounds a NewLongTimeoutClient request: long enough for a legitimately large single-
// page listing (HumanBit's ~5.6 MB page takes ~18s), past the standard 15s timeout, unlike
// streamTimeout's allowance for a throttled bulk feed.
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

// NewSingleUseConnClient builds a client that dials a fresh connection per request, for a platform
// whose server mishandles a connection it has already answered on (see
// safehttp.NewTransportWithoutKeepAlive). Exposed for host tools such as harvest-boards'
// Profession prober. Otherwise identical to NewClient.
func NewSingleUseConnClient() *Client { return newSingleUseConnClient() }

func newSingleUseConnClient() *Client {
	c := newClientWithProxy(nil)
	c.httpClient = safehttp.NewClientWithoutKeepAlive(15 * time.Second)
	c.streamClient = safehttp.NewClientWithoutKeepAlive(streamTimeout)
	return c
}

// BrowserUserAgent is a plain desktop Chrome string, for the one provider (4dayweek.io) whose
// Cloudflare edge refuses every client that identifies itself, even on a path its robots.txt
// ALLOWS.
//
// Use it only for providers that allow the path in robots.txt and refuse the client anyway; it is
// not a way around a Disallow. Exported for cmd/liveness, whose 4dayweek probe sits behind the
// same edge.
const BrowserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"

// newBrowserUAClient is NewClient with BrowserUserAgent. Unexported: nothing outside this package
// needs it.
func newBrowserUAClient() *Client {
	c := NewClient()
	c.userAgent = BrowserUserAgent
	return c
}

// macBrowserUserAgent is a desktop Chrome string on macOS, for Feishu Hire (feishujobs), whose
// edge filters on the operating system in the string: the project string, Linux and Windows Chrome
// strings and Go's default are all answered 405 with an empty body. The same robots.txt rule as
// BrowserUserAgent holds.
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

// compatibleUserAgent is the project's own identity in browser-compatible form, for an edge that
// refuses any string not starting with a browser token (OCC Mundial) yet does not ask for a
// browser. It still names us, so prefer it to BrowserUserAgent wherever it is enough.
const compatibleUserAgent = "Mozilla/5.0 (compatible; freehire/0.1; +https://freehire.me)"

// NewCompatibleUAClient is NewClient with compatibleUserAgent — same guarded transport, same
// retry budget, same everything else. Exported for cmd/harvest-boards, whose probes meet the
// same edges as the adapters.
func NewCompatibleUAClient() *Client {
	c := NewClient()
	c.userAgent = compatibleUserAgent
	return c
}

// crawlerUserAgent identifies as Googlebot, for energyjobline.com, whose job-detail pages server-
// render their schema.org JobPosting ld+json only for a recognised search-crawler User-Agent; any
// other string gets a client-rendered shell. Its robots.txt does not disallow /job/, so this reads
// content it already publishes for search engines.
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
