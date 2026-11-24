# scraplingfetch

The Go side of the Scrapling fetch tier: an `http.RoundTripper` that forwards each request to
[services/scrapling-fetch](../../../services/scrapling-fetch/README.md) on the crawl host and
rebuilds the upstream answer as the response.

**Why a RoundTripper and not a getter.** `sources.Client` wraps an `*http.Client`. Putting this
underneath it keeps every getter (`GetJSON`, `GetHTML`, `GetText`, `GetXML`, the POST paths),
the retries, the 64 MiB body cap, `StatusError` and `BodyTooLargeError` as the code they already
are. The hosted-Firecrawl client this replaced re-implemented `GetXML`/`GetHTML` and could do
nothing else.

**One Transport, one mode and one proxy flag.** `ModeHTTP` is one curl_cffi request with
Chrome's TLS fingerprint; `ModeBrowser` is a navigation in a stealth browser that solves
Cloudflare challenges, GET only. `proxy` asks the service to egress through its own
`SOURCES_PROXY_URL` — the credential never crosses this wire.

**The upstream status is a response, a service failure is an error.** A 403 page comes back as
a 403 response, so the caller's ordinary status handling sees it. A service that is down,
answers 5xx, or answers malformed is a transport error. A non-200 service response carries
`ServiceError` so a caller can retry specific service statuses. Naukrigulf retries a service 502
within its existing backoff and request limit.

**Time.** A browser page that meets a Cloudflare challenge measured ~110 s on its first fetch
(bayt, 2026-10-20) and ~5 s after, so the timeout a caller passes to `New` has to cover the
first page, not the typical one.

**No SSRF guard here.** The service, not this process, dials the target, so `safehttp`'s
private-address check never sees it. That is acceptable for the reason the proxied tier gives:
only curated providers with fixed public hosts are routed here, never the link-following path.
