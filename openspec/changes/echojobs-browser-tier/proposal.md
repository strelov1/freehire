## Why

`echojobs` ingested nothing for 19 days while every run reported success (freehire#2588).
echojobs.io now sits behind Vercel's bot protection, and the adapter's stated premise —
"neither needs JavaScript execution — a plain GET reaches both" — stopped being true.

Measured, not assumed:

| origin | plain GET | headless Chrome |
|---|---|---|
| prod datacenter IP | `403`, `x-vercel-mitigated: deny` | **`403`** — the challenge is never even offered |
| `SOURCES_PROXY_URL` egress | `429` + `Vercel Security Checkpoint` | **200, real page** |

Neither half works alone. The browser is useless from the prod IP, because a browser can
only solve a challenge it is served; the proxy is useless without a browser, because the
challenge needs JavaScript. Only the pair passes — which is why adding `echojobs` to
`proxiedProviders`, the one-line remedy that recovered `djinni`, `enlizt` and `wantedkr`,
was tried in a spike and rejected.

Once a browser holds clearance, the cheap tier is **inside the page**, not outside it. A
`_vcrcs` cookie carried into Go's own HTTP client still gets the checkpoint — the clearance
is bound to more than the cookie — but `fetch()` evaluated in the cleared page is served
normally, at roughly a tenth of a full navigation:

| request | via navigation | via in-page `fetch()` |
|---|---|---|
| one-time clearance | 7.7s | 7.7s (once per tab) |
| `/sitemap.xml` (27 shards) | 0.33s | **0.19s** |
| `/sitemap-jobs/1.xml` (10 000 locs, 2.1 MB) | 6.4s | **0.37s** |
| a posting page (JobPosting present) | 4.2s | **0.36s** |

In-page `fetch()` also returns the real status code — a missing posting answers `404`, which
the plain navigation could not distinguish from a block. That matters to the lifecycle,
where only `404`/`410` may mean "gone".

At ~0.36s a posting and 400–1500 new postings a day, a steady-state crawl costs minutes.

## What Changes

- **A headless-browser fetcher becomes shared transport.** A new `internal/platform/browser`
  owns launching a stealth Chrome (optionally through an authenticating proxy) and one
  operation over it: fetch a URL from inside a cleared page and return its status and body.
  It knows nothing about jobs, exactly like `platform/llm` and `platform/aigateway`, and it
  is in `platform` for the reason `aigateway`'s own comment gives — its two callers sit in
  different blocks, so anywhere else is an upward edge for one of them.
- **The stealth launch flags get a single home.** `api/atsapply` keeps its own session, DOM
  scanning and form filling, and takes only its launch options from the new package. Two
  copies of the flags would mean a future stealth fix lands in one and silently not the
  other.
- **`sources` gains a browser-backed getter** satisfying `XMLGetter` + `HTMLGetter`, which is
  exactly `echojobsHTTP`. **Not one line of the adapter's reading changes** — its sitemap
  walk, its shard freshness cutoff and its JobPosting parse already work, and what broke was
  only how the bytes arrive. Its sole edit is the guard below, which is about reporting, not
  about reading.
- **`echojobs` opts in per-provider**, in the shape `proxiedProviders` already uses, and only
  when a proxy is configured — a browser without one is proven useless, so it is never
  launched for nothing.
- **A crawl that lists postings and hydrates none fails its board** instead of reporting an
  empty success. This is the narrow half of freehire#2588's second point: what let the
  outage run for 19 days was not the block but that a total failure and an empty crawl were
  indistinguishable. The check lives in the adapter, because a dropped posting never reaches
  the pipeline — only `FetchNew` knows it walked the sitemap and yielded nothing.

## Capabilities

### New Capabilities

- `job-source-browser-tier`: how a source whose pages are gated behind a JavaScript
  challenge is crawled — what clearance is, how requests ride it, when the tier is used at
  all, and what a crawl that reaches nothing must report.

### Modified Capabilities

<!-- None. The echojobs adapter's own parsing behaviour is unchanged; only its transport is. -->

## Impact

- `internal/platform/browser/` — new: launch options, session, in-page fetch.
- `internal/api/atsapply/browser.go` — `stealthAllocatorOptions` delegates; nothing else moves.
- `internal/ingest/sources/` — new browser-backed getter and the per-provider opt-in; the
  registry wiring beside `proxiedProviders`.
- `internal/ingest/sources/echojobs.go` — the listed-but-hydrated-none board error (its
  only change; the sitemap walk and JobPosting parse are untouched).
- `internal/platform/arch/layering/blocks.go` — `browser` joins the `platform` block.

Not in scope: reviving the 84 605 postings closed on 2026-09-07. A working crawl re-opens
whatever the 14-day sitemap window still lists; the rest are a month old, roughly half were
already duplicate-suppressed, and re-opening postings nobody has verified is the thing this
catalogue's ghost-job work exists to prevent. Also out: recovering the employer apply URL,
which the page has not carried since the 2026-08 rewrite.
