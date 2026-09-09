# Hosted fetch

An HTTP client for a third-party scraping API, for a site that refuses every address this
repository can egress from. Transport, not a feature — the same category as
`platform/browseruse`, which is likewise a client for somebody else's hosted browser.

**Not `platform/browser`'s sibling, despite the shape.** That package launches a process on our
own host and costs nothing per page. This one calls a metered vendor. Two things with the same
shape and opposite economics should not be confused, which is why the budget below is part of
the client rather than something a caller may forget.

## When this is the right answer, and when it is not

There are three transports, and this is the last resort:

| tier | answers | cost |
|---|---|---|
| `proxiedProviders` | our IP is blocked, another is not | the proxy we already pay for |
| `browserProviders` | the page needs JavaScript run | our own Chrome, free |
| **this one** | **no address we can obtain is served at all** | **a credit per page** |

Nothing belongs here that either of the others can reach. Its two providers (`bayt`,
`gulftalent`) are `403` from the production IP *and* `403` through `SOURCES_PROXY_URL`, whose
exit their edge classifies as datacenter too — and the browser tier does not help, because the
refusal comes before any challenge is offered, so there is nothing to solve.

## Know the yield before you widen this

Measured 2026-09-07 by running the sources' own titles through `classify.IsTech`:

| source | sample | passed the tech gate |
|---|---|---|
| `bayt`, its **IT** category | 33 | **0** |
| `gulftalent`, real postings | 400 | **5 (1.25%)** |

bayt's IT category is project managers, VPs, department directors and professors. gulftalent's
postings are nannies, chefs, camp bosses and hotel electricians, with about one engineer in
eighty. At ~60 000 gulftalent postings that is perhaps 750 technical ones for the entire site,
and every page fetched costs a credit whether or not it turns out to be one of them.

This was built with that measured and accepted. The number is recorded here so the next person
inherits it rather than rediscovering it at their own expense.

## A different shape: `hh` is not IP-blocked at all

`bayt`/`gulftalent` refuse **every** address outright; `hh` (headhunter.ru) doesn't. Its listing
pages work fine on the plain direct IP, and even its detail pages aren't blocked in the ordinary
sense — through `SOURCES_PROXY_URL` they redirect to an interactive DDoS-Guard image CAPTCHA
(`/account/captcha`, measured 2026-09-08), which the browser tier can't solve either (it's a
real CAPTCHA, not a JS challenge) and which this package doesn't attempt to solve. Firecrawl's
own egress simply isn't the flagged proxy IP, so it reaches the same pages cleanly.

So `hh` is a MIXED-tier provider, like `wantapply`: only the part that's actually broken (detail
hydration) is hosted, and listing stays on a free, unproxied client — see
[internal/ingest/sources/AGENTS.md](../../ingest/sources/AGENTS.md)'s "hosted tier" section for
the wiring. Unlike `wantapply`, hh's free part must NOT reuse `ApplyFirecrawlEgress`'s shared
`direct` transport — that value becomes the proxied client whenever `SOURCES_PROXY_URL` is set
for any OTHER provider, which is exactly the (captcha-walled) transport hh's listing needs to
avoid.

## Three bounds, none redundant

- **No key, no client.** `ApplyFirecrawlEgress` is a no-op without `FIRECRAWL_API_KEY` and the
  client is never constructed, so merging or deploying this spends nothing.
- **A per-run page budget** (`FIRECRAWL_MAX_PAGES_PER_RUN`, default 500), counted **inside the
  client** and shared across every provider in the run. An adapter cannot know what the others
  already spent, and what is being protected is the account rather than any one crawl. The
  count is claimed BEFORE the request, so a refused fetch never spends the page it refuses.
  `New` rejects a non-positive budget outright — an unbounded metered client is the accident
  this package exists to make impossible.
- **The timer stays off.** A key alone crawls nothing until an ingest timer is enabled. Three
  deliberate acts, not one.

## The status is the target's, never the vendor's

The API answers `200` while reporting the target's own status inside its envelope. Reading the
outer one would turn every refusal into a success carrying a bot wall as though it were
content, so `Fetch` returns `metadata.statusCode` and treats a missing one as a failed fetch
rather than as an implied success.

A non-2xx **target** status comes back as data, not an error — only some statuses may be read
as "this resource is gone". A **vendor** failure is an error, and the difference matters: a
caller that mistook a vendor outage for a refusal could conclude a posting is gone when only
the API was down.

## Testing

Everything is exercised against an `httptest` stand-in for the vendor: no network, no key, no
credits. Live verification is deliberately **not** automated — a test that quietly spends money
on every CI run is a worse failure than no test. The live evidence for this tier is the
measurement above, taken by hand and written down.
