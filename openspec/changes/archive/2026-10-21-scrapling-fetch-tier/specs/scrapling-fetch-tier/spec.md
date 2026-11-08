## ADDED Requirements

### Requirement: Fetch service contract

The Scrapling fetch service SHALL accept `POST /fetch` with a JSON body naming `url`, `method`,
`headers`, `body`, `mode` (`http` or `browser`) and `proxy` (boolean), and SHALL answer a
completed fetch with HTTP 200 and a JSON body carrying the upstream `status`, the final `url`
after redirects, the upstream response `headers`, and the response `body` base64-encoded. An
upstream non-2xx status SHALL be reported as data in that body, never as the service's own
status.

#### Scenario: Upstream success

- **WHEN** a caller fetches a page the upstream serves with 200
- **THEN** the service answers 200 with `status` 200 and the page bytes in `body`

#### Scenario: Upstream refusal is data

- **WHEN** the upstream answers 403
- **THEN** the service answers 200 with `status` 403 and the refusal page in `body`

#### Scenario: Service-side failure

- **WHEN** the fetch cannot complete (browser crash, timeout, unreachable host)
- **THEN** the service answers with a 5xx status and an error message, and no upstream status

### Requirement: Fetch modes

The service SHALL serve `http` mode through Scrapling's TLS-impersonating HTTP fetcher for any
method, and `browser` mode through a stealth browser session that solves Cloudflare
challenges. `browser` mode SHALL accept only `GET`.

#### Scenario: Non-GET in browser mode

- **WHEN** a caller sends `method: POST` with `mode: browser`
- **THEN** the service answers 400 without starting a browser

### Requirement: Proxy credential stays in the service

The proxy URL SHALL come from the service's own environment (`SOURCES_PROXY_URL`). A caller
SHALL only select it with `proxy: true`; the request SHALL NOT carry a proxy address.

#### Scenario: Proxy requested without one configured

- **WHEN** a caller sends `proxy: true` and the service has no proxy configured
- **THEN** the service answers 400 rather than fetching directly

### Requirement: Warm browser sessions under a global cap

The service SHALL reuse warm browser sessions keyed by `(host, proxy)` across requests, each
serving up to `SCRAPLING_TABS_PER_BROWSER` pages at once, opening another for the same key only
when its existing ones are full, closing a session after an idle
timeout, and SHALL keep no more than `SCRAPLING_MAX_BROWSERS` sessions alive — closing an idle
session of another key to make room, and otherwise queuing a request for at most a bounded time
before failing it.

#### Scenario: A session in constant use is still retired

- **WHEN** a session is older than `SCRAPLING_SESSION_MAX_SECONDS` or has served
  `SCRAPLING_SESSION_MAX_PAGES` pages
- **THEN** it takes no new pages and closes once its in-flight pages finish

#### Scenario: A hung page frees its slot

- **WHEN** a page is still running after the per-use deadline
- **THEN** the request fails with a 5xx and its session is retired

#### Scenario: Second page on a host reuses the session

- **WHEN** two `browser` fetches for the same host and proxy arrive in sequence
- **THEN** the second runs on the session the first opened

#### Scenario: Cap reached

- **WHEN** a `browser` fetch needs a new session, the cap is reached and every session is busy
- **THEN** it waits until one frees, never opening one past the cap, and fails with a 5xx if
  none frees within the queue bound

### Requirement: Proxy rotation

When `SCRAPLING_PROXY_ROTATING` is set, an `http`-mode request with `proxy: true` SHALL egress
through it, and a 403 or 429 answer SHALL be retried up to twice more, since each attempt leaves
from another exit IP. When `SCRAPLING_PROXY_STICKY` and `SCRAPLING_PROXY_STICKY_COUNT` are set,
each new proxied browser session SHALL take the next fixed exit IP in turn and keep it for its
life, and a session whose page is refused with 403 or 429 SHALL be retired, the refusal still
returned to the caller. With neither set, `SOURCES_PROXY_URL` SHALL serve both.

#### Scenario: A flagged exit IP

- **WHEN** an http request through the rotating proxy is answered 403 and its retry 200
- **THEN** the caller receives the 200

#### Scenario: A refused browser session

- **WHEN** a proxied browser page is answered 429
- **THEN** the caller receives the 429, and the next proxied page on that host opens a new
  session on another sticky exit IP

#### Scenario: Direct requests are never retried for a refusal

- **WHEN** an http request without the proxy is answered 403
- **THEN** it is returned after one attempt

### Requirement: Service access is local and authenticated

The service SHALL listen on `127.0.0.1` and SHALL refuse a request without the bearer token
`SCRAPLING_FETCH_TOKEN` with 401. It SHALL bound the request body and the upstream response
body it returns.

#### Scenario: Missing token

- **WHEN** a request arrives without the bearer token
- **THEN** the service answers 401 and fetches nothing

### Requirement: Internal targets refused

The service SHALL refuse with 400, before fetching, a URL whose host resolves to any loopback,
private, link-local, multicast, reserved or unspecified address, because adapters follow links
scraped from the sources' own pages and the fetch runs on the crawl host.

#### Scenario: Metadata endpoint

- **WHEN** a request names `http://169.254.169.254/...` or a host resolving to `10.0.0.5`
- **THEN** the service answers 400 and fetches nothing

### Requirement: Go transport over the service

`internal/platform/scraplingfetch` SHALL provide an `http.RoundTripper` that sends each request
to the service with the provider's configured mode and proxy flag and rebuilds an
`*http.Response` from the answer, so that an ordinary ingest `Client` built over it keeps its
body caps and status errors. A service 5xx or an unreachable service SHALL surface as a
transport error. The client built over it SHALL NOT retry on its own (the service retries once),
so one dead page cannot cost several browser timeouts.

#### Scenario: Upstream status passes through

- **WHEN** the service reports upstream `status` 404
- **THEN** the round trip returns a response with status 404 and the upstream body

#### Scenario: Service down

- **WHEN** the service cannot be reached
- **THEN** the round trip returns an error, and the crawl fails rather than falling back to a
  direct request

### Requirement: Per-provider allowlist

`cmd/ingest` SHALL rebuild the providers named in `scraplingProviders` over the Scrapling
transport at startup, each with its own mode and proxy flag, when `SCRAPLING_FETCH_URL` is set.
When it is unset, every provider SHALL keep its existing transport. The allowlist SHALL cover
infojobs, bayt, gulftalent, wantapply, wellfound, hh (detail only), startupjobs (its
Cloudflare-challenged pages only), cleverstaff and kontur.

#### Scenario: Unset means unchanged

- **WHEN** `SCRAPLING_FETCH_URL` is unset
- **THEN** no provider's transport changes

#### Scenario: Partial routing

- **WHEN** `SCRAPLING_FETCH_URL` is set
- **THEN** hh hydrates details through the service while its listing stays on a plain client,
  and startupjobs reads only its challenged site pages through the service

### Requirement: Hosted Firecrawl tier removed

The repository SHALL NOT contain the hosted Firecrawl client or tier, and SHALL NOT read
`FIRECRAWL_API_KEY` or `FIRECRAWL_MAX_PAGES_PER_RUN`.

#### Scenario: Old setting is inert

- **WHEN** `FIRECRAWL_API_KEY` is set in a worker's environment
- **THEN** nothing reads it and no request goes to the hosted API
