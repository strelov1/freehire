## Why

Two providers are unreachable from anything this repository can egress from. `bayt` and
`gulftalent` answer `403` to the production datacenter IP **and** to `SOURCES_PROXY_URL`,
whose exit their edge classifies as a datacenter too. `proxy.go` has recorded that for months
and named the remedy it was waiting for: "the day a genuinely residential pool exists".

Measured 2026-09-07 against a hosted scraping API (Firecrawl), both are served normally:

| source | prod IP | our proxy | hosted fetch |
|---|---|---|---|
| `bayt` (UAE listing) | `403` | `403` | **200, 284 KB** |
| `gulftalent` (sitemap) | `403` | `403` | **200, 30 000 job URLs** |

So the missing piece is not a better fingerprint or a better browser — the browser tier
(freehire#2588) does not help here either, because the refusal comes before any challenge.
The missing piece is an address we do not own.

**What is behind those walls is thin, and this change is being made with that measured rather
than assumed.** Running their own titles through this repository's own `classify.IsTech`:

| source | sample | pass the tech gate |
|---|---|---|
| `bayt`, its **IT** category | 33 titles | **0** |
| `gulftalent`, real postings | 400 titles | **5 (1.25%)** |

bayt's IT category is project managers, VPs, department directors and professors; gulftalent's
postings are nannies, chefs, camp bosses and hotel electricians, with about one engineer in
eighty. At roughly 60 000 gulftalent postings that is perhaps 750 technical ones for the whole
site, and every page fetched costs a credit.

That trade was put to the owner with these numbers and the decision was to build it anyway.
This proposal therefore optimises for the one thing that follows from those numbers: **it must
be impossible for this tier to spend money by accident.**

## What Changes

- **A hosted-fetch client becomes shared transport.** A new `internal/platform/firecrawl`
  speaks the vendor's scrape API and returns a page's raw bytes. It knows nothing about jobs —
  the same "transport, not domain" category as `platform/browseruse`, which is likewise an HTTP
  client for somebody else's hosted browser.
- **`sources` gains a hosted-fetch getter** satisfying `XMLGetter` + `HTMLGetter`. Both target
  adapters already declare exactly those (`baytHTTP` is `HTMLGetter`; `gulftalentHTTP` is both),
  so **neither adapter changes**.
- **`bayt` and `gulftalent` opt in per-provider**, in the shape `proxiedProviders` and
  `browserProviders` already use.
- **The tier is off unless `FIRECRAWL_API_KEY` is set**, and no request is ever made without it.
- **A per-run page budget bounds one crawl** (`FIRECRAWL_MAX_PAGES_PER_RUN`). Every fetch is
  counted; past the budget the run stops asking rather than continuing to spend. With a 1%
  useful yield and a metered API, an unbounded crawl of a 60 000-page site is a month's
  allowance spent in one night.
- **`gulftalent` moves off the fingerprint tier deliberately, not accidentally.** It is in
  `proxiedFingerprintProviders` today, and both tiers rewire the same registry entry. The
  override is made explicit, ordered last, and pinned by a test — the alternative is the exact
  hazard the browser tier's disjointness test exists to prevent.

## Capabilities

### New Capabilities

- `job-source-hosted-fetch`: how a source that refuses every address this repository can egress
  from is crawled through a third party — when the tier engages, what bounds its spending, and
  how it interacts with the transports that would otherwise claim the same provider.

### Modified Capabilities

<!-- None. bayt's and gulftalent's own parsing is unchanged; only their transport is. -->

## Impact

- `internal/platform/firecrawl/` — new: the API client.
- `internal/platform/arch/layering/blocks.go` — `firecrawl` joins the `platform` block.
- `internal/ingest/sources/` — the hosted-fetch getter, the per-provider opt-in and the page
  budget; `proxy.go`'s comment about waiting for a residential pool, which this answers.
- `cmd/ingest` — applying the tier, after the others.
- `internal/ingest/sources/bayt.go`, `gulftalent.go` — **unchanged**, and a test says so by
  continuing to pass untouched.

Not in scope: enabling either provider's ingest timer. Merging this changes nothing until a key
is configured AND a timer is turned on, which are two separate deliberate acts. Also out:
using this tier for `echojobs`, which the browser tier already serves at no per-page cost.
