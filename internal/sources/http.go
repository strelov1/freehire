package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/safehttp"
)

// The capability interfaces below are the narrow transport roles an adapter depends
// on (interface segregation): a JSON-list adapter takes a JSONGetter, an HTML-detail
// adapter an HTMLGetter, and so on. The real Client implements them all, so one Client
// serves every adapter; a test fake implements only the role(s) its adapter uses
// rather than stubbing a six-method surface. Composed roles (e.g. JSON list + HTML
// detail) are declared as a small embedded interface at the adapter that needs them.

// JSONGetter fetches a URL and decodes its JSON body into v.
type JSONGetter interface {
	GetJSON(ctx context.Context, url string, v any) error
}

// StreamGetter issues a GET and hands the raw response body to fn for streaming decode.
// Unlike GetJSON it uses a long timeout and neither buffers nor size-caps the body, so it
// suits large, slow feeds (e.g. the throttled JobStream) that a 15s, size-capped GetJSON
// cannot read.
type StreamGetter interface {
	GetStream(ctx context.Context, url, accept string, fn func(io.Reader) error) error
}

// XMLGetter fetches a URL and decodes its XML body into v (platforms publishing an XML
// feed, e.g. Personio).
type XMLGetter interface {
	GetXML(ctx context.Context, url string, v any) error
}

// HTMLGetter fetches a URL and returns its parsed HTML tree, for adapters whose detail
// is server-rendered HTML rather than a structured body (e.g. SuccessFactors).
type HTMLGetter interface {
	GetHTML(ctx context.Context, url string) (*html.Node, error)
}

// HTMLResolvedGetter fetches a URL following redirects and returns its parsed HTML tree
// together with the FINAL URL after redirects — for adapters that must observe a redirect
// (e.g. djinni's listing, whose past-the-end page 302s to the bare listing, so the final
// URL is the only reliable end-of-feed signal).
type HTMLResolvedGetter interface {
	GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error)
}

// TextGetter fetches a URL and returns its raw response body, for adapters that regex a
// token or config blob out of a page rather than parsing its DOM (e.g. Cornerstone).
type TextGetter interface {
	GetText(ctx context.Context, url string) (string, error)
}

// HeaderTextGetter behaves like TextGetter but attaches extra request headers, for a raw-body
// endpoint gated behind a non-secret header (e.g. a Next.js App Router RSC data endpoint, which
// returns its flight payload only when the request carries the "RSC: 1" header). The custom
// headers never override the standard User-Agent/Accept.
type HeaderTextGetter interface {
	GetTextWithHeaders(ctx context.Context, url string, headers map[string]string) (string, error)
}

// JSONPoster sends a JSON request body and decodes the JSON response (platforms whose
// listing API is POST-only, e.g. Workday).
type JSONPoster interface {
	PostJSON(ctx context.Context, url string, body, v any) error
}

// HeaderJSONGetter and HeaderJSONPoster behave like JSONGetter/JSONPoster but attach
// extra request headers, for an API gated behind a non-secret header (e.g. MTS's public
// x-api-key). The custom headers never override the standard User-Agent/Accept.
type HeaderJSONGetter interface {
	GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error
}

type HeaderJSONPoster interface {
	PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error
}

// HTTPClient is the full transport surface, composing every capability role. The real
// Client implements it; sources.All holds one and passes it to each adapter, which then
// narrows it to the role(s) it actually uses.
type HTTPClient interface {
	JSONGetter
	StreamGetter
	XMLGetter
	HTMLGetter
	HTMLResolvedGetter
	TextGetter
	HeaderTextGetter
	JSONPoster
	HeaderJSONGetter
	HeaderJSONPoster
}

// maxResponseBody caps how many bytes a decoder reads from any response. ATS list
// feeds run to a few MB at most; this generous ceiling bounds a hostile or buggy
// endpoint returning a multi-GB body (or a gzip bomb the transport transparently
// inflates) so it cannot exhaust the worker's memory.
const maxResponseBody = 32 << 20 // 32 MiB

// limitedBody caps how many bytes a decoder reads from a response while preserving
// the original Closer, so the connection is still released after the bounded read.
type limitedBody struct {
	io.Reader
	io.Closer
}

// Client is the real HTTPClient: a timeout-bounded GET with a project User-Agent and
// a bounded retry-with-backoff on transient (5xx / network) failures. A 4xx is not
// retried — it will not recover on its own.
type Client struct {
	httpClient *http.Client
	// streamClient is a long-timeout transport for GetStream. The bulk feeds it serves
	// (JobStream) throttle to a trickle, so a full window read takes minutes — far past the
	// 15s httpClient timeout. Same SSRF-guarded transport, just patient.
	streamClient *http.Client
	userAgent    string
	maxRetries   int
	retryDelay   time.Duration
}

// NewClient builds the default ingest HTTP client.
func NewClient() *Client { return newClientWithProxy(nil) }

// NewProxyClient builds an ingest client whose requests egress through proxy. It is
// identical to NewClient but for the proxied transport, and is handed only to the
// curated set of providers whose prod-IP is anti-bot-blocklisted (see All). The proxy
// URL — including its host and credentials — comes entirely from configuration
// (SOURCES_PROXY_URL); no proxy endpoint is hardcoded here.
func NewProxyClient(proxy *url.URL) *Client { return newClientWithProxy(proxy) }

// newClientWithProxy builds a Client with an optional egress proxy (nil = direct).
func newClientWithProxy(proxy *url.URL) *Client {
	return &Client{
		// Guarded transport: adapters and link-following fetch attacker-influenced
		// URLs, so the client must refuse internal/metadata targets (SSRF). A proxied
		// client relaxes the target check (the proxy resolves the target) and is only
		// given to trusted, curated providers — never the link-following path.
		httpClient:   safehttp.NewClientWithProxy(15*time.Second, proxy),
		streamClient: safehttp.NewClientWithProxy(streamTimeout, proxy),
		userAgent:    "freehire/0.1 (+https://freehire.dev)",
		maxRetries:   2,
		retryDelay:   500 * time.Millisecond,
	}
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

// streamTimeout bounds a GetStream read. Generous because the throttled bulk feeds it
// serves trickle in over many minutes; the per-run page/window budget keeps the actual
// transfer well under this ceiling, which is only a stuck-connection backstop.
const streamTimeout = 15 * time.Minute

// GetStream issues a guarded GET and hands the raw response body to fn for streaming
// decode. It uses the long-timeout transport and does NOT buffer or size-cap the body, so
// it suits large, slow feeds that GetJSON cannot read. There is no retry: a streamed body
// cannot be safely replayed once fn has begun consuming it.
func (c *Client) GetStream(ctx context.Context, url, accept string, fn func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("sources: build request %s: %w", url, err)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	req.Header.Set("Accept", accept)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("sources: get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StatusError{Code: resp.StatusCode, URL: url}
	}
	return fn(resp.Body)
}

// GetJSON fetches url and decodes its JSON body into v.
func (c *Client) GetJSON(ctx context.Context, url string, v any) error {
	return c.GetJSONWithHeaders(ctx, url, nil, v)
}

// GetJSONWithHeaders fetches url with extra request headers and decodes its JSON body
// into v.
func (c *Client) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	return c.do(ctx, request{
		method:  http.MethodGet,
		url:     url,
		accept:  "application/json",
		headers: headers,
		decode: func(resp *http.Response) error {
			return json.NewDecoder(resp.Body).Decode(v)
		},
	})
}

// GetXML fetches url and decodes its XML body into v (used by adapters whose platform
// publishes an XML feed, e.g. Personio).
func (c *Client) GetXML(ctx context.Context, url string, v any) error {
	return c.do(ctx, request{
		method: http.MethodGet,
		url:    url,
		accept: "application/xml",
		decode: func(resp *http.Response) error {
			return xml.NewDecoder(resp.Body).Decode(v)
		},
	})
}

// maxTextBody caps the raw body GetText reads. Careers pages can be large, but the
// ATS link we scan for sits in the markup, not megabytes of trailing content; the
// cap keeps a runaway page from ballooning memory.
const maxTextBody = 2 << 20 // 2 MiB

// GetText fetches url and returns its raw response body as a string (capped at
// maxTextBody). Used by the domain-following harvest, which regex-scans a careers
// page's HTML for an embedded ATS link rather than parsing a DOM.
func (c *Client) GetText(ctx context.Context, url string) (string, error) {
	return c.GetTextWithHeaders(ctx, url, nil)
}

// GetTextWithHeaders behaves like GetText but attaches extra request headers, for a raw-body
// endpoint gated behind a non-secret header (e.g. a Next.js RSC data endpoint reached with
// "RSC: 1").
func (c *Client) GetTextWithHeaders(ctx context.Context, url string, headers map[string]string) (string, error) {
	var body string
	err := c.do(ctx, request{
		method:  http.MethodGet,
		url:     url,
		accept:  "text/html",
		headers: headers,
		decode: func(resp *http.Response) error {
			b, err := io.ReadAll(io.LimitReader(resp.Body, maxTextBody))
			if err != nil {
				return err
			}
			body = string(b)
			return nil
		},
	})
	return body, err
}

// GetHTML fetches url and returns its parsed HTML tree.
func (c *Client) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	node, _, err := c.getHTML(ctx, url)
	return node, err
}

// GetHTMLResolved fetches url, following redirects, and returns its parsed HTML tree
// together with the final URL after redirects — for shortener-fronted detail pages
// (e.g. a u.habr.com link that 301s to career.habr.com/vacancies/<id>).
func (c *Client) GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error) {
	return c.getHTML(ctx, url)
}

// getHTML fetches url and returns its parsed tree plus the final URL after redirects,
// backing both GetHTML and GetHTMLResolved.
func (c *Client) getHTML(ctx context.Context, url string) (*html.Node, string, error) {
	var node *html.Node
	var final string
	err := c.do(ctx, request{
		method: http.MethodGet,
		url:    url,
		accept: "text/html",
		decode: func(resp *http.Response) error {
			final = resp.Request.URL.String()
			n, err := html.Parse(resp.Body)
			if err != nil {
				return err
			}
			node = n
			return nil
		},
	})
	return node, final, err
}

// PostJSON marshals body to JSON, POSTs it to url, and decodes the JSON response into
// v (used by adapters whose listing API is POST-only, e.g. Workday).
func (c *Client) PostJSON(ctx context.Context, url string, body, v any) error {
	return c.PostJSONWithHeaders(ctx, url, nil, body, v)
}

// PostJSONWithHeaders behaves like PostJSON but attaches extra request headers.
func (c *Client) PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("sources: marshal request %s: %w", url, err)
	}
	return c.do(ctx, request{
		method:  http.MethodPost,
		url:     url,
		body:    payload,
		accept:  "application/json",
		headers: headers,
		decode: func(resp *http.Response) error {
			return json.NewDecoder(resp.Body).Decode(v)
		},
	})
}

// StatusError is a non-2xx HTTP response from the sources client. Callers that need
// to branch on the status code (e.g. eightfold's rate-limit backoff) match it with
// errors.As instead of scraping the message. Its Error() renders the same
// "sources: GET <url>: status <n>" text the client has always produced, so any
// remaining string-based check keeps working.
type StatusError struct {
	Code int
	URL  string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sources: GET %s: status %d", e.URL, e.Code)
}

// ChallengeError is an AWS-WAF Challenge response — a request the WAF answered with an
// interactive JS proof-of-work shell instead of the real content (marked by the
// "x-amzn-waf-action: challenge" header, served with HTTP 202). It is NOT a transient
// failure: a fixed backoff will not clear it, and retrying only deepens the per-IP
// penalty. Callers behind a rate-based WAF (e.g. clinch) match it with errors.As to
// stop hydrating for the rest of the run rather than parse the shell as a posting.
type ChallengeError struct {
	URL string
}

func (e *ChallengeError) Error() string {
	return fmt.Sprintf("sources: GET %s: WAF challenge", e.URL)
}

// request is the parameters of a single HTTP exchange issued by do. A non-nil body is
// re-sent on each retry; the standard User-Agent/Accept headers always win over headers.
type request struct {
	method  string
	url     string
	body    []byte
	accept  string
	headers map[string]string
	decode  func(*http.Response) error
}

// do issues an HTTP request (optionally with a JSON body) and applies r.decode to a
// successful response body, retrying transient failures (5xx / network / 429 rate
// limit) up to maxRetries times. The backoff is a fixed delay, except a 429 honors
// the server's Retry-After hint — busy ATS APIs (SmartRecruiters) throttle by IP
// under the concurrent crawl and recover on a brief wait. Other 4xx are not retried.
func (c *Client) do(ctx context.Context, r request) error {
	var lastErr error
	delay := c.retryDelay
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		delay = c.retryDelay

		var reqBody io.Reader
		if r.body != nil {
			reqBody = bytes.NewReader(r.body)
		}
		req, err := http.NewRequestWithContext(ctx, r.method, r.url, reqBody)
		if err != nil {
			return fmt.Errorf("sources: build request %s: %w", r.url, err)
		}
		// Custom headers go first; the standard headers below always win, so a caller
		// can never accidentally override User-Agent/Accept.
		for k, val := range r.headers {
			req.Header.Set(k, val)
		}
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}
		req.Header.Set("Accept", r.accept)
		if r.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue // network error — transient
		}

		// An AWS-WAF Challenge is served with HTTP 202 (a 2xx that would otherwise be
		// decoded as content) and its own action header. Catch it before the success
		// branch and return immediately — it is not transient, so a retry is wasted and
		// only prolongs the WAF's per-IP penalty.
		if resp.Header.Get("X-Amzn-Waf-Action") == "challenge" {
			resp.Body.Close()
			return &ChallengeError{URL: r.url}
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			// Bound the read so a pathological body cannot OOM the worker; Close still
			// runs on the underlying body to release the connection.
			resp.Body = limitedBody{Reader: io.LimitReader(resp.Body, maxResponseBody), Closer: resp.Body}
			err := r.decode(resp)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("sources: decode %s: %w", r.url, err)
			}
			return nil
		case resp.StatusCode == http.StatusTooManyRequests:
			delay = retryAfter(resp, c.retryDelay) // honor the rate-limit hint
			resp.Body.Close()
			lastErr = &StatusError{Code: resp.StatusCode, URL: r.url}
			continue // rate limited — transient
		case resp.StatusCode >= 500:
			resp.Body.Close()
			lastErr = &StatusError{Code: resp.StatusCode, URL: r.url}
			continue // server error — transient
		default:
			resp.Body.Close()
			return &StatusError{Code: resp.StatusCode, URL: r.url}
		}
	}
	return fmt.Errorf("sources: GET %s failed after %d attempts: %w", r.url, c.maxRetries+1, lastErr)
}

// retryAfter is how long to wait before retrying a 429, honoring the response's
// Retry-After header (delta-seconds) when present and sane, else the fallback. It
// is capped so one rate-limited board cannot stall the whole crawl.
func retryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	const max = 30 * time.Second
	if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			d := time.Duration(secs) * time.Second
			if d > max {
				return max
			}
			return d
		}
	}
	return fallback
}
