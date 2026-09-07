# Design

## Why the browser sits in `platform`

`internal/ingest` is block 7, `internal/api` is block 8, and a block may import only blocks
strictly below it. So `sources` cannot reach `api/atsapply`'s browser code — `depguard`
fails the import line and `arch/layering` fails the graph.

That leaves duplicating it in `ingest`, or moving what is shared down. The repository has
already answered this exact question once, and the answer is in `blocks.go`'s own comment:

> aigateway is here for the same reason as llm: it is the HTTP half of talking to the
> OpenAI-compatible gateway and knows nothing about the domain. Its two callers sit in
> different blocks (ai/speech and api/realtime), so anywhere else it would be an upward edge
> for one of them.

A headless browser is the same shape — transport that knows nothing about jobs, with callers
in two different blocks. It goes in `platform`.

## What moves, and what deliberately does not

`platform/browser` owns two things:

- **`LaunchOptions(proxyURL string)`** — the stealth flags. Today that is `headless` plus
  `disable-blink-features=AutomationControlled`, the pair the 2026-09-02 auto-apply spike
  measured against `bot.sannysoft.com`, plus the proxy server when one is given.
- **`Session`** — a running browser, its proxy authentication, and `Fetch`.

`api/atsapply` keeps its `newBrowserSession`, its DOM scanning, its filling and its
screenshots. Only `stealthAllocatorOptions` becomes a call into `LaunchOptions("")`.

The split is on purpose. What must not be duplicated is the *knowledge of how to look like a
real browser*, because that is the part a future anti-bot fix will touch, and two copies mean
the fix lands in one — the [hardcoded-list trap](../../../AGENTS.md) in another form. What
must not be moved is atsapply's session, because it does something entirely different with
the page (it fills a form and submits it) and a shared abstraction over both would be an
abstraction over nothing.

## Clearance, and why the fetch happens inside the page

`Session.Fetch(ctx, url)` returns `(status int, body []byte, err error)`. Under it:

1. On a tab's first use, navigate to the site's origin once and wait for the challenge to
   clear. ~7.7s, paid once per tab.
2. Every request after that is `fetch(url, {credentials:"include"})` evaluated in that page,
   awaited, returning status and body.

The third option — take the browser's cookie and use Go's `http.Client` — was measured and
**does not work**: the `_vcrcs` cookie transfers, and a plain GET carrying it still gets the
checkpoint. Vercel binds clearance to more than the cookie (a TLS fingerprint is the obvious
candidate), so the request has to leave from the browser itself. In-page `fetch` is how it
does that without paying for a render.

`Fetch` returns the status rather than an error for a non-2xx, because the caller needs to
tell `404` (the posting is gone — the only reading the job lifecycle accepts as gone) from
`403`/`429` (we are blocked — never a reason to close anything).

## Concurrency

`chromedp` serialises actions on one context, so one tab is one request at a time. The
session therefore holds a small pool of tabs, each clearing itself on first use.

Whether clearance is shared across tabs of one browser profile is **unmeasured**, and the
design does not depend on the answer: each tab clears once regardless. If it turns out to be
shared, tabs after the first will simply find themselves already cleared and the wait
collapses on its own — a saving, never a correctness question.

Pool size follows the adapter's existing `defaultDetailWorkers` rather than inventing a
second number.

## The seam in `sources`, and why the adapter is untouched

`echojobs` declares its transport as `echojobsHTTP`: `XMLGetter` + `HTMLGetter`. A type
backed by a `browser.Session` satisfying those two is a drop-in — `GetXML` fetches and
unmarshals, `GetHTML` fetches and parses. The sitemap walk, the shard freshness cutoff, the
JobPosting decode, the skill canonicalisation: all unchanged, all still covered by their
existing tests against fakes.

Opt-in follows `proxiedProviders`' shape — a map keyed by provider, each value rebuilding the
adapter over the browser-backed client — and is **gated on a proxy being configured**. The
spike proved a browser without one gets `403` from the prod IP, so launching Chrome in that
case would spend seconds to fail. Without `SOURCES_PROXY_URL` the provider stays on the plain
client and fails exactly as it does today, which is the honest outcome: nothing is quietly
working.

## Making a total failure look like one

The outage lasted 19 days because `ingested=0 failed=0` and a genuinely empty crawl are the
same line. `echojobs.FetchNew` already counts what it dropped and logs a summary; nothing
reads it.

The narrow fix: **a crawl that discovered candidate postings and hydrated none of them is a
board failure**, not an empty success.

It belongs in the ADAPTER, not the pipeline, and the reason is structural: a dropped posting
never reaches the pipeline at all. `Stats` counts `saveOne` failures, so from the pipeline's
side an adapter that discarded everything and an adapter that found nothing return the same
empty slice. Only `FetchNew` knows it walked 10 000 sitemap entries and yielded zero. Making
this a pipeline-wide rule would mean every adapter reporting its drops through a new signal —
a much larger change, for a guard that is needed where the drops happen.

So `echojobs.FetchNew` returns an error when it had candidates and hydrated none of them. A
board-level error is what `board_health` already watches, so nothing downstream needs teaching.

It deliberately does not try to be a general freshness monitor — the broader "a provider whose
`max(last_seen_at)` has not moved in N days" gauge belongs with `queue-metrics` and stays in
freehire#2588.

## Operational notes

The ingest units and the auto-apply unit are identical where it matters — same `User=freehire`,
same `EnvironmentFile`, and neither sets `PrivateTmp` or `ProtectHome`. Chrome already runs
under that user for auto-apply, so the ingest path needs no unit change. Verified on the host
rather than assumed.

Google Chrome 152 is installed at `/usr/bin/google-chrome`.

## Testing

- `platform/browser` — the launch options are a pure function and unit-testable: the flags are
  present, and a proxy URL with credentials yields a `ProxyServer` carrying the host but never
  the credentials (those go over CDP, and a credential in an argv is a leak). The session
  itself needs a real Chrome, so its test is `//go:build integration` and drives a local
  `httptest` server: one that answers plainly, one that answers only after a JS-set cookie —
  the smallest thing shaped like a challenge — plus a 404 to prove the status is reported
  rather than swallowed.
- `sources` browser getter — integration-tagged against the same local server, asserting that
  `GetXML` decodes and `GetHTML` parses what the session returned.
- `echojobs` adapter — existing tests unchanged. If any of them needed editing, the seam would
  be in the wrong place.
- `pipeline` — a unit test that a run which listed postings and hydrated none reports a board
  failure, and its converse: a board that genuinely listed nothing still reports success.
