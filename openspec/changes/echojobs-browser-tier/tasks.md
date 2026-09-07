## 1. `platform/browser` — the launch flags

- [x] 1.1 Write the failing test for `LaunchOptions`: the stealth pair is present; an empty
      proxy adds no proxy flag; a proxy URL carrying credentials yields a `ProxyServer` with
      the scheme and host and with **neither the username nor the password anywhere in the
      option set** (a credential in argv is readable by any process on the host).
- [x] 1.2 Add `internal/platform/browser/browser.go` with `LaunchOptions`, documenting that
      this is the single home for the stealth flags and naming the 2026-09-02 measurement
      the pair came from.
- [x] 1.3 Register `browser` in the `platform` block in
      `internal/platform/arch/layering/blocks.go`, with the comment saying why it is transport
      (the `aigateway` precedent) rather than an ingest or api package.

## 2. `platform/browser` — the session

- [x] 2.1 Write the failing integration test (`//go:build integration`, needs Chrome) against
      a local `httptest` server with three routes: one plain `200`, one that serves its real
      body only after a JS-set cookie (the smallest thing shaped like a challenge), and one
      `404`. Assert: the challenged route is read, the `404` is reported **as 404 rather than
      as an error**, and a second fetch on the same tab pays no second clearance.
- [x] 2.2 Implement `Session`: launch via `LaunchOptions`, answer `Fetch.authRequired` over
      CDP with the proxy credentials, `Clear(ctx, origin)` to obtain clearance, and
      `Fetch(ctx, url) (status int, body []byte, err error)` evaluating an awaited in-page
      `fetch` — with the doc saying why the cookie cannot simply be handed to `net/http`
      (measured: it still gets the checkpoint).
- [x] 2.3 Add the tab pool: N tabs, each clearing on first use, `chromedp` serialising one
      request per tab. Document that whether clearance is shared across tabs is unmeasured
      and that the design does not depend on the answer.

## 3. `api/atsapply` takes its flags from the shared home

- [x] 3.1 Make `stealthAllocatorOptions` delegate to `browser.LaunchOptions("")`, leaving
      `newBrowserSession`, the DOM scan, the fill and the screenshots untouched. Its existing
      tests must stay green without edits.

## 4. `sources` — the browser-backed getter

- [x] 01 Write the failing integration test: a `browserClient` over a real session satisfies
      `XMLGetter` + `HTMLGetter`, decoding XML and parsing HTML served by a local server.
- [x] 02 Implement it, mapping a non-2xx status to an error that NAMES the status, so a
      caller can still tell `404` from `403`.

## 5. `echojobs` opts in

- [x] 01 Write the failing test for the registry: with a proxy configured, `echojobs`
      resolves to the browser-backed adapter; with none, it resolves to the plain one and no
      browser is constructed.
- [x] 02 Add `browserProviders` beside `proxiedProviders`, one entry, in the same shape, with
      the doc stating the measured reason both halves are required.

## 6. A crawl that reads nothing of what it listed fails

- [x] 01 Write the failing test in `echojobs_test.go`: a sitemap yielding candidates whose
      every detail fetch fails returns an ERROR; a sitemap yielding no candidates at all
      still returns success with no jobs.
- [x] 02 Implement the guard in `FetchNew`, with the comment explaining that the pipeline
      cannot make this call because a dropped posting never reaches it.

## 7. Ship

- [x] 7.1 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`;
      `go test -tags=integration` for the packages touched.
- [x] 7.2 Document the tier in `internal/ingest/sources/AGENTS.md` and the new package in
      `internal/platform/AGENTS.md`; add the module-file row for `platform/browser`.
- [x] 7.3 Open the PR referencing freehire#2588; state that enabling it on prod is a separate
      step and that the 84 605 closed rows stay closed.
