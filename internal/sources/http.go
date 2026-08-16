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

// CookieReader reads back a cookie a prior request established in the client's jar, for
// the double-submit CSRF pattern (see Client.CookieValue).
type CookieReader interface {
	CookieValue(url, name string) string
}

// HeaderFormPoster POSTs a form-urlencoded body with extra request headers and returns
// the response parsed as HTML — for a CSRF-protected listing endpoint that is form-bodied
// rather than JSON (e.g. aijobs.net's Django cookie-token flow).
type HeaderFormPoster interface {
	PostFormWithHeaders(ctx context.Context, url string, headers map[string]string, values url.Values) (*html.Node, error)
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
	HeaderFormPoster
	CookieReader
}

// maxResponseBody caps how many bytes a decoder reads from any response. It bounds a
// hostile or buggy endpoint returning a multi-GB body (or a gzip bomb the transport
// transparently inflates) so it cannot exhaust the worker's memory. The ceiling sits
// well past the largest real feed: measured 2026-07-30, the biggest list bodies were
// Lever jobgether 36.8 MiB, Greenhouse pulse 35.5 MiB and Anduril 34.6 MiB — a list
// endpoint that inlines every description (`?content=true`) reaches tens of MiB at a
// few thousand postings. The previous 32 MiB ceiling sat just under that, silently
// truncating five of the largest boards; those now fetch, and a board that ever does
// outgrow this one says so through BodyTooLargeError instead of looking transient.
const maxResponseBody = 64 << 20 // 64 MiB

// BodyTooLargeError is a response whose body ran past the size cap and was therefore
// truncated mid-stream. It is NOT transient — the same board returns the same oversized
// body on every run — so callers must not treat it as a fetch that might recover. A bare
// io.LimitReader used to cap the read, which handed the decoder a truncated stream that
// was indistinguishable from a dropped connection; Anduril's Greenhouse board then spent
// six weeks reporting "unexpected EOF" and cooling down as if the network were at fault.
type BodyTooLargeError struct {
	URL   string
	Limit int64
}

func (e *BodyTooLargeError) Error() string {
	return fmt.Sprintf("sources: GET %s: body exceeds %d bytes", e.URL, e.Limit)
}

// cappedReader bounds how many bytes are read from a response body and names the cap when
// it trips. The read window is limit+1 bytes: a body of exactly limit bytes is complete,
// and only the sentinel byte past it proves truncation.
type cappedReader struct {
	r     io.Reader
	url   string
	limit int64
	read  int64
}

func newCappedReader(r io.Reader, url string, limit int64) *cappedReader {
	return &cappedReader{r: r, url: url, limit: limit}
}

func (b *cappedReader) Read(p []byte) (int, error) {
	room := b.limit + 1 - b.read
	if room <= 0 {
		return 0, &BodyTooLargeError{URL: b.url, Limit: b.limit}
	}
	if int64(len(p)) > room {
		p = p[:room]
	}
	n, err := b.r.Read(p)
	b.read += int64(n)
	if b.read > b.limit {
		return n, &BodyTooLargeError{URL: b.url, Limit: b.limit}
	}
	return n, err
}

// cappedBody is a cappedReader that still closes the response body it wraps, so the
// connection is released after the bounded read.
type cappedBody struct {
	*cappedReader
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
	// refusalClient is an optional second transport used ONLY after the direct one has been
	// refused (429/403). It exists because a refusal is the one failure a different egress IP
	// can actually fix, and because the proxy behind it is a single shared IP: routing a whole
	// crawl through it would concentrate the same burst on a weaker address and starve the
	// providers that are genuinely IP-blocked. Nil for every client but the opted-in few.
	refusalClient *http.Client
	userAgent     string
	maxRetries    int
	retryDelay    time.Duration
	// maxBody overrides maxResponseBody; zero means the default. Only tests set it, so a
	// cap-trip case need not stream tens of MiB through the loopback.
	maxBody int64
}

// bodyLimit is the response-size cap in effect for this client.
func (c *Client) bodyLimit() int64 {
	if c.maxBody > 0 {
		return c.maxBody
	}
	return maxResponseBody
}

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
		return &StatusError{Method: http.MethodGet, Code: resp.StatusCode, URL: url}
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
// maxTextBody). It serves adapters whose endpoint is a raw page they scan or slice
// themselves rather than parse as a DOM (paylocity, taleo, infojobs, …) and the
// harvest/board-resolve probes that regex-scan a careers page for an embedded ATS link.
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
// "sources: <METHOD> <url>: status <n>" text the client has always produced, so any
// remaining string-based check keeps working.
type StatusError struct {
	Method string
	Code   int
	URL    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sources: %s %s: status %d", e.Method, e.URL, e.Code)
}

// StatusCode reports the response status. It exists so a package this one IMPORTS can
// still branch on the code: internal/applyform is imported by the Recruitee adapter, so
// it cannot import sources back, and a one-method interface it declares locally is
// satisfied by this without either package naming the other.
func (e *StatusError) StatusCode() int { return e.Code }

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

// CookieValue returns the value of the named cookie as currently held in the client's
// cookie jar for url's host, "" if unset or the client has no jar (the shared client,
// built without one). It exists for the double-submit CSRF pattern (e.g. aijobs.net's
// Django flow), where a value a prior GET's Set-Cookie response established must also be
// echoed back as a request header/body field — something the jar alone, which only
// re-attaches cookies to outgoing requests automatically, cannot do.
func (c *Client) CookieValue(rawURL, name string) string {
	if c.httpClient.Jar == nil {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, ck := range c.httpClient.Jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

// PostFormWithHeaders POSTs values as an application/x-www-form-urlencoded body with
// extra request headers, and returns the response parsed as HTML — for a listing
// endpoint that is POST-only and CSRF-protected (aijobs.net's Django cookie-token
// flow) rather than a JSON API.
func (c *Client) PostFormWithHeaders(ctx context.Context, url string, headers map[string]string, values url.Values) (*html.Node, error) {
	var node *html.Node
	err := c.do(ctx, request{
		method:      http.MethodPost,
		url:         url,
		body:        []byte(values.Encode()),
		contentType: "application/x-www-form-urlencoded",
		accept:      "text/html",
		headers:     headers,
		decode: func(resp *http.Response) error {
			n, err := html.Parse(resp.Body)
			if err != nil {
				return err
			}
			node = n
			return nil
		},
	})
	return node, err
}

// request is the parameters of a single HTTP exchange issued by do. A non-nil body is
// re-sent on each retry; the standard User-Agent/Accept headers always win over headers.
// contentType is the Content-Type sent alongside a non-nil body; empty defaults to
// "application/json" (every existing POST helper is JSON-bodied).
type request struct {
	method      string
	url         string
	body        []byte
	contentType string
	accept      string
	headers     map[string]string
	decode      func(*http.Response) error
}

// do issues an HTTP request (optionally with a JSON body) and applies r.decode to a
// successful response body, retrying transient failures (5xx / network / 429 rate
// limit) up to maxRetries times. The backoff is a fixed delay, except a 429 honors
// the server's Retry-After hint — busy ATS APIs (SmartRecruiters) throttle by IP
// under the concurrent crawl and recover on a brief wait. Other 4xx are not retried.
func (c *Client) do(ctx context.Context, r request) error {
	var lastErr error
	delay := c.retryDelay
	// refusalTried records that the fallback egress has had its one turn. A refusal says the
	// platform is there and is declining THIS IP, so a second IP is worth exactly one try;
	// spending more of a shared proxy on a request the platform is refusing anyway would take
	// the budget from the providers that have no direct path at all.
	refusalTried := false
	client := c.httpClient
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
			contentType := r.contentType
			if contentType == "" {
				contentType = "application/json"
			}
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := client.Do(req)
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
			resp.Body = cappedBody{
				cappedReader: newCappedReader(resp.Body, r.url, c.bodyLimit()),
				Closer:       resp.Body,
			}
			err := r.decode(resp)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("sources: decode %s: %w", r.url, err)
			}
			return nil
		case resp.StatusCode == http.StatusTooManyRequests:
			delay = retryAfter(resp, c.retryDelay) // honor the rate-limit hint
			resp.Body.Close()
			lastErr = &StatusError{Method: r.method, Code: resp.StatusCode, URL: r.url}
			if c.refusalClient != nil {
				// Both egresses refusing is not a transient failure of either — it is the
				// platform declining the request, and further attempts only spend a shared
				// proxy budget that the IP-blocked providers have no alternative to.
				if refusalTried {
					return lastErr
				}
				c.switchToRefusalEgress(&client, &refusalTried)
				delay = 0 // a fresh IP has no penalty to wait out
			}
			continue // rate limited — transient
		case resp.StatusCode >= 500:
			resp.Body.Close()
			lastErr = &StatusError{Method: r.method, Code: resp.StatusCode, URL: r.url}
			continue // server error — transient
		default:
			resp.Body.Close()
			lastErr = &StatusError{Method: r.method, Code: resp.StatusCode, URL: r.url}
			// A 403 is final on this IP but not necessarily on another: it is the shape an edge
			// uses to turn away an address it has judged, which is the one 4xx a different
			// egress can fix. So it retries once through the fallback when there is one, and
			// otherwise returns immediately exactly as before.
			if resp.StatusCode == http.StatusForbidden && c.switchToRefusalEgress(&client, &refusalTried) {
				delay = 0
				continue
			}
			return lastErr
		}
	}
	return fmt.Errorf("sources: %s %s failed after %d attempts: %w", r.method, r.url, c.maxRetries+1, lastErr)
}

// switchToRefusalEgress points client at the fallback transport for the remaining attempts,
// once. It reports whether the switch happened, so the caller can drop the backoff it was
// about to serve: the wait exists to let the refusing IP cool down, and the fallback is a
// different IP that owes nothing.
func (c *Client) switchToRefusalEgress(client **http.Client, tried *bool) bool {
	if c.refusalClient == nil || *tried {
		return false
	}
	*tried = true
	*client = c.refusalClient
	return true
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
