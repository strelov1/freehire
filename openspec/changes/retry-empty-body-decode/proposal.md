## Why

Investigating GitHub issue #2079 (Avature adapter, `mantech.avature.net`) found that the
board's actual live failure is not what the issue describes. Reproduced from the production
host's own egress IP: three consecutive requests to the board's real listing sitemap
(`careers.mantech.com/en_US/careers/sitemap.xml`) returned `200, 382789 bytes` then `200, 0
bytes` twice in a row — the edge intermittently answers success with a genuinely empty body.
The shared ingest HTTP client (`internal/ingest/sources/http.go`, `Client.do`) already retries
network errors, `429`, and `5xx`, but a `2xx` response whose decode callback fails is returned
immediately as a hard, non-retried error. One unlucky empty-body response therefore fails the
whole board crawl and increments `board_health.consecutive_failures` toward cooldown, even
though the very next request against the same URL succeeds. This is a transport-level gap, not
specific to Avature — every adapter that calls `GetJSON*`/`GetXML`/`PostJSON*` shares this
client.

## What Changes

- `Client.do` treats a `2xx` response whose `decode` callback fails with exactly `io.EOF` as
  transient and retries it against the existing retry budget (`maxRetries`/`retryDelay`), the
  same way the neighboring `429`/`5xx` branches already do.
- Any other decode failure (a non-empty but malformed body — e.g. a genuine XML syntax error)
  keeps failing immediately, exactly as today: the new behavior is scoped to Go's stdlib
  `json`/`xml` decoders' specific signal for "the input was completely empty," which they
  never return for a malformed-but-present body.
- No change to `GetHTML`/`PostFormWithHeaders` (`html.Parse` does not signal an empty body as
  `io.EOF`, so this fix has no effect on HTML-decoding call sites) and no change to the body
  size cap, the WAF-challenge short-circuit, or the 403 refusal-egress switch.

## Capabilities

### New Capabilities

- `ingest-http-retry`: the shared ingest HTTP client's retry policy — which response shapes
  (network error, `429`, `5xx`, and now an empty-body `2xx`) are treated as transient and
  retried against the client's bounded retry budget, versus which are returned immediately as
  final. No spec previously existed for this shared transport; this proposal documents the
  existing policy alongside the new rule rather than leaving the addition undocumented next to
  nothing.

### Modified Capabilities

(none — `ingest-http-retry` is new)

## Impact

- `internal/ingest/sources/http.go` — `Client.do`'s response-handling switch.
- `internal/ingest/sources/http_test.go` — new coverage: an empty-body `2xx` retried to
  success, an empty-body `2xx` still failing after exhausting the retry budget, and a
  non-empty malformed body (XML syntax error) still failing immediately with zero retries.
- Every adapter registered in `internal/ingest/sources/registry.go` that goes through
  `GetJSON*`/`GetXML`/`PostJSON*` benefits transparently; no adapter code changes.
- `mantech.avature.net`'s current `board_health` failure is expected to self-heal once this
  ships and the board's next scheduled crawl runs — no manual intervention needed beyond that.
- Follow-up outside this change: comment on GitHub issue #2079 noting its own diagnosis no
  longer matches the live cause, and that this change addresses the actual current failure.
