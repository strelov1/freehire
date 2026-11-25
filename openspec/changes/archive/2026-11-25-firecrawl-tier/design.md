# Design

## Where the client lives

`internal/platform/firecrawl`, beside `browseruse`, and for the reason `browseruse`'s own entry
in `blocks.go` gives: it is the HTTP half of talking to a vendor API and knows nothing about the
domain. One caller today (`ingest/sources`); the category, not the caller count, is what places
it — `browseruse` also has one.

It is deliberately NOT `platform/browser`'s neighbour in behaviour. That package launches a
process on our own host and costs nothing per page. This one calls a metered third party. Two
things with the same shape and opposite economics should not share a name or a home, which is
why the sources-side maps stay separate too.

## The seam, again

`baytHTTP` is `HTMLGetter`. `gulftalentHTTP` is `XMLGetter` + `HTMLGetter`. A hosted-fetch
client satisfying both is a drop-in, and **neither adapter changes** — exactly as with the
browser tier, and for the same reason: what is broken about these providers is the address the
request leaves from, not how the response is read.

Errors are `*StatusError`, the type the plain client produces, so `detailUnreadable` and
`isRateLimited` keep working. A hosted fetch adds one failure mode the others do not have —
the vendor answering fine while reporting that the target refused — so the target's status is
what gets rendered, never the vendor's.

## Spending is the design problem

Everything above is small. The part that needs care is that this tier costs real money per
page, against a measured 1% useful yield.

Three bounds, each doing something the others cannot:

**No key, no tier.** `ApplyFirecrawlEgress` is a no-op without `FIRECRAWL_API_KEY`, and the
client is never constructed. Merging this cannot spend anything.

**A per-run page budget.** `FIRECRAWL_MAX_PAGES_PER_RUN` (default 500) is counted inside the
client, across every provider in the run. Past it, further fetches fail fast with a named error
rather than being made. 500 is deliberately small: gulftalent alone lists 60 000 postings, and
the difference between a bounded run and an unbounded one is the difference between a nightly
trickle and a month's allowance gone by morning. An operator who wants a bigger pass raises it
for that run, which is a decision with a number attached rather than an accident.

The budget lives in the CLIENT, not in each adapter. An adapter cannot know what the other
adapters in the same run have already spent, and the thing being protected is the account, not
the crawl.

**The timer stays off.** Merging this and configuring a key still crawls nothing until an
ingest timer is enabled. Three deliberate acts, not one.

## The gulftalent override

`gulftalent` is in `proxiedFingerprintProviders` today. Both tiers rewire the same registry
entry, and `cmd/ingest` applies them in sequence, so without care it would silently get
whichever ran last — the hazard the browser tier's disjointness test exists to catch.

Here the override is wanted: the fingerprint transport is measured as refused, the hosted one
as served. So it is made explicit instead of forbidden.

- `ApplyFirecrawlEgress` runs **last** in `cmd/ingest`, after the proxy and browser tiers.
- A test pins the order's effect: with a key configured, `gulftalent` resolves to the hosted
  client, not the fingerprint one.
- A test pins the fallback: with no key, it resolves to the fingerprint client exactly as
  today, so an unconfigured deployment is bit-for-bit unchanged.

The browser tier's disjointness rule is narrowed rather than dropped: browser and proxy tiers
must still not overlap (nothing wants that), while the hosted tier is allowed to override, and
the test says which is which and why. A rule with a documented exception beats a rule quietly
broken.

## Testing

- `platform/firecrawl` — the request/response mapping and the budget against an `httptest`
  server standing in for the vendor: a page returned, a target refusal reported as the
  TARGET's status, a vendor error distinguished from a target one, and the budget refusing the
  (N+1)th fetch without issuing it. No network, no key, no credits.
- `sources` — `GetXML` decodes and `GetHTML` parses what the client returned; a non-2xx target
  status surfaces as `*StatusError` carrying that code.
- The tier wiring — no key is a no-op; a key rewires both providers; `gulftalent` resolves to
  hosted with a key and to fingerprint without one; an unrelated registry is untouched.
- `bayt` and `gulftalent`'s own tests are not edited. If any of them needed editing, the seam
  would be in the wrong place.

Live verification against the real vendor is deliberately NOT automated: it costs credits per
run, and a test that quietly spends money every time CI runs is a worse failure than no test.
The live evidence for this change is the measurement in the proposal, taken by hand.
