## Context

See proposal.md - Why for the live reproduction. The relevant code is `Client.do` in
`internal/ingest/sources/http.go`, specifically the `switch` on `resp.StatusCode` around lines
709-754: a `2xx` response runs `r.decode(resp)` and, on error, returns immediately via
`fmt.Errorf("sources: decode %s: %w", r.url, err)`. The neighboring `429`/`5xx` cases instead
set `lastErr` and `continue` the attempt loop, which already has a retry budget
(`c.maxRetries`, default 2) and backoff (`c.retryDelay`, `retryAfter` for `429`).

`decode` is a caller-supplied closure (`request.decode func(*http.Response) error`). The two
call sites this change targets are `GetXML` (`xml.NewDecoder(resp.Body).Decode(v)`) and
`GetJSON`/`PostJSON*` (`json.NewDecoder(resp.Body).Decode(v)`). Both of Go's stdlib decoders
return exactly `io.EOF` — not a wrapped or annotated variant — when `Decode` is called on a
reader that yields zero bytes before any token is read. Verified locally: `html.Parse` (used
by `GetHTML`/`PostFormWithHeaders`) returns `nil` error on an empty reader, so it never
produces this signal — confirming the fix has no reach into HTML-decoding call sites without
needing to special-case them.

## Goals / Non-Goals

**Goals:**
- Retry an empty-bodied `2xx` exactly like a `5xx`, reusing the existing budget/backoff state
  machine rather than adding a parallel one.
- Leave every other branch of `Client.do` (WAF challenge, `429`, `5xx`, `403` refusal-egress,
  other `4xx`, non-empty decode failure) behaviorally untouched.

**Non-Goals:**
- Distinguishing WHY the body was empty (rate-limiting vs. a genuine CDN glitch vs. something
  else) — out of scope, and not observable from this side regardless.
- Retrying decode failures that are not `io.EOF` (a present-but-malformed body). Widening the
  net there risks masking a real, persistent adapter/format bug (e.g. jackhenry.avature.net's
  known XML syntax error) behind a few extra seconds of retries before it surfaces the same way
  it does today.
- Touching `GetText`/`GetTextWithHeaders`'s `cappedReader`-based truncation handling — that
  path already has its own typed signal (`BodyTooLargeError`) for a different failure shape
  (oversized, not empty) and is unaffected by this change.

## Decisions

**Detect via `errors.Is(err, io.EOF)` at the point `decode` fails, not a new response
wrapper.** Go's decoders already surface this precisely — `io.EOF` and only `io.EOF` for a
truly empty input — so no extra body-peeking (e.g. reading and checking `len() == 0` before
handing the reader to `decode`) is needed. Peeking would also require buffering the whole body
up front, undoing the streaming decode `xml.NewDecoder`/`json.NewDecoder` do today.

Alternative considered: check `resp.ContentLength == 0` instead of the decode error. Rejected
— `ContentLength` is `-1` for chunked/unknown-length responses (common behind a CDN), so it is
not a reliable signal, whereas the decoder's own `io.EOF` is authoritative regardless of
whether the length was advertised.

**Apply uniformly to every method (`GET` and `POST`), not gated by verb.** The two `POST`
call sites (`PostJSON*`, `PostFormWithHeaders`) are read-only queries against search/listing
APIs (Workday-style POST-as-query platforms), never mutations, so retrying one that came back
empty carries the same safety as retrying a `GET`. Gating by method would add a branch with no
adapter that currently needs the distinction.

**No new error type.** Unlike `BodyTooLargeError` (which callers match with `errors.As` to
distinguish "this board is genuinely oversized" from a transient failure), an empty body that
survives every retry is, from the caller's perspective, indistinguishable from any other
exhausted-retry failure — nothing downstream needs to branch on it specifically. The final
error keeps today's shape: `fmt.Errorf("sources: decode %s: %w", r.url, err)`, still wrapping
`io.EOF` so a future caller could `errors.Is` it if a need arises.

**Where in the switch:** inside the existing `case resp.StatusCode >= 200 && resp.StatusCode <
300:` branch, after `err := r.decode(resp)`, branch on `errors.Is(err, io.EOF)` before the
current unconditional `return`. On the EOF path: close the body (already done before the
branch), set `lastErr = fmt.Errorf(...)`, `continue`. On any other decode error: return exactly
as today.

## Risks / Trade-offs

- **A source whose feed is legitimately, permanently empty (zero postings) now costs up to
  `maxRetries` extra requests before returning.** Mitigation: `maxRetries` is 2, so this is 2
  extra requests with backoff (≤ ~1s at the default `retryDelay`) per affected board per crawl
  — negligible against the run-once-and-exit cron model, and no adapter treats "zero postings,
  no error" as a failure today (an empty `urlset`/`sitemapindex` decodes successfully with zero
  entries; this branch only fires when the body couldn't be decoded AT ALL because it was
  empty, which is a different, always-erroneous shape for a well-formed feed).
- **Masking a platform that has started permanently serving empty responses (a real outage on
  their side) behind a slightly longer failure loop.** Mitigation: unchanged end state — after
  `maxRetries` is exhausted the board still fails and `board_health.consecutive_failures` still
  increments toward cooldown exactly as before; this only changes whether a single unlucky
  empty response fails the run outright versus getting the existing retry chance every other
  transient class already gets.

## Migration Plan

Pure code change to a shared, already-deployed client; no data migration, no config, no
feature flag. Ships on the next regular deploy. Rollback is a plain revert if needed — no
state to unwind.
