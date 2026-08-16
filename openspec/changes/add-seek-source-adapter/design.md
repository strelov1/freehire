## Context

Issue #1634 proposed SEEK as the highest-value, highest-risk target of its source batch: AU is a
market no per-employer ATS covers at scale, but a plain fetch of a SEEK search page answered 403
with a tiny body, which reads as active bot protection. The issue explicitly deferred the work
pending a feasibility spike.

The spike ran on 2026-08-16 and produced these live-verified facts, which the whole design rests on:

- The 403 is a **Cloudflare interstitial** ("Just a moment...") and it covers the human-facing HTML
  pages, including a job's own `/job/<id>` page. Browser-shaped headers do not clear it.
- SEEK's frontend **search API does not sit behind it**: `GET
  https://www.seek.com.au/api/jobsearch/v5/search` answers 200 JSON with no cookie, no credential
  and no browser-shaped User-Agent — verified with no UA at all, with `curl/8.7.1`, with
  `Go-http-client/2.0`, and with the project's own `freehire/0.1` agent. All four succeeded.
- `www.seek.com.au` now 308-redirects its HTML routes to `au.seek.com`, and the older
  `chalice-search` API paths are gone. `api/jobsearch/v5/search` is the live one on both hosts.
- The listing carries every field the adapter needs **except the description** (only a one-line
  teaser). The body comes from `POST /graphql`, operation `jobDetails`, which validates its own
  query — an unused variable is rejected with `GRAPHQL_VALIDATION_FAILED`, so a drifted field fails
  loudly rather than silently returning nothing.
- SEEK serves at most ~550 results per query (page size 100 serves pages 1–5 and answers page 6
  empty; page size 50 serves through page 11, offset 550). Page size above 100 returns nothing.
- **`totalCount` is a function of `pageSize`.** The same query answered 36 at `pageSize=1`, 688 at
  `pageSize=20`, 680 at 50 and 666 at 100. It cannot drive pagination or detect truncation.
- Omitting the `where` parameter does not mean "everywhere": it collapsed a 688-posting slice to 36.
- Roughly one posting in thirty has no profiled employer; `advertiser.description` carries the typed
  name instead, and sometimes SEEK's `"Private Advertiser"` placeholder.

Measured live at spike time: AU ICT ≈ 6,471 postings across 22 subclassifications, NZ ≈ 1,323 across
21.

## Goals / Non-Goals

**Goals:**

- Cover SEEK's ICT catalogue in both AU and NZ, with descriptions, at a request cost that does not
  grow with catalogue size once steady-state.
- Fit the existing `Source` / `HydratingSource` contracts and the repository's board-is-a-slice
  aggregator shape — no new pipeline concepts.
- Encode every verified platform trap where it will be read: at the code that would otherwise trip
  on it.

**Non-Goals:**

- Non-ICT classifications. SEEK's taxonomy spans every industry; the catalogue is an IT job board.
- Defeating the Cloudflare interstitial. Nothing the adapter needs is behind it.
- Structured salary. SEEK states a free-text label, not an amount.
- Liveness probing SEEK postings. Their job pages are interstitial-gated.

## Decisions

**Board = ICT subclassification, region = market.** SEEK's whole ICT classification is ~6.5k
postings in AU against a ~550 result window, so it is unreachable as one query; its subclassifications
(319–746 postings) are mostly reachable as slices. Region carries the market, mirroring
`sources/adzuna.yml`, where region is the country and board the category. Region is already part of
the board dedupe key and of the `board_health` primary key, so the same subclassification id in `au`
and `nz` is two independent, independently-healthy crawl targets for free.

*Alternative considered:* slicing further by state, giving full coverage of the five slices that
exceed the window. Rejected: it quadruples the request count and the board-file size to chase a tail
that ages out anyway (below), and the user chose the simpler shape explicitly.

**Five slices exceed the window; wait rather than slice.** AU's Engineering - Software (~746),
Help Desk (~689), Business/Systems Analysts (~688), Developers/Programmers (~638) and Programme &
Project Management (~584) hold more postings than the ~550 the crawl can reach. Ordered newest-first,
the reachable window covers roughly the first 24 days of a SEEK advertisement's 30-day run, so a
steadily-running crawl sees every posting while it is new and only loses sight of it near expiry.
The cost is a tail that reads as unseen, which is exactly what `sweepGrace` exists for.

**14-day sweep grace.** Same reasoning `whatjobs` uses: a posting drifting past crawl depth would be
closed and reopened on the 48-hour default, writing a phantom removal into `job_daily_stats` each
cycle. 14 days is wider than the drift and still narrower than the ~30-day ad life. The marker is
sound here specifically because liveness CANNOT be probed — SEEK's job pages are interstitial-gated —
which is the condition the marker's own documentation sets.

**`HydratingSource`, not a plain `Source`.** A detail request per posting per run would be ~7.8k
GraphQL calls every crawl. Hydrating only what the catalogue lacks reduces steady state to the day's
new postings, and `SeenRefresh` keeps the pipeline from re-deriving facets from an empty body.

**Never trust `totalCount`.** The walk's stop condition is "this page added no new id", the
repository's existing `added == 0` convention. A page ceiling of 6 backstops it. The field is
deliberately absent from the response struct so no future edit can reach for it.

**Fold the salary label into the description.** `Job`'s structured salary fields take a structured
amount; SEEK's label ranges from `"$75,000 – $85,000 per year"` to `"160000"` to `"Rates
Negotiable"`. `hh` sets the precedent for folding a list-carried salary into the body and letting
enrichment parse it.

**Store the interstitial-gated `/job/<id>` URL anyway.** It is the URL a human needs, and it works
in a browser. Our crawler cannot fetch it, but nothing in the ingest path needs to: SEEK is
board-swept, not liveness-probed.

## Risks / Trade-offs

- **Both endpoints are frontend internals, not a documented API** → a first-page failure is a
  board-level error, so board health cools the board and surfaces it rather than letting the
  catalogue drift. The GraphQL operation validates its own query, so a renamed field errors instead
  of silently emptying descriptions.
- **The five over-window slices lose their oldest postings** → accepted, mitigated by newest-first
  ordering plus the 14-day grace. Re-slicing by state remains available without a schema change if
  the loss proves material.
- **SEEK could extend the interstitial to `/api/`** → the crawl fails loudly per board. There is no
  silent-degradation path, because the walk never trusts a reported count.
- **Cloudflare may rate-limit at crawl volume** → the pipeline's per-board concurrency plus the
  adapter's bounded detail pool keep the burst modest; if it proves too much, the repository already
  has the pacer (`internal/sources/pacer.go`) and the proxy-egress allowlist as the two established
  remedies.
- **`"Private Advertiser"` postings are dropped** → a real, if small, coverage loss, taken because a
  placeholder company would pollute the company dimension for every such posting across both markets.
- **A posting whose detail failed is ingested body-less and never retried** → accepted, and NOT
  fixable in this adapter. `seen` is `func(externalID string) bool` over
  `ExistingExternalIDs`, which reports row existence (and `is_tech`), never whether the stored row
  carries a description — so an adapter cannot tell a hydrated row from a body-less one, and every
  `HydratingSource` in the repository shares the behaviour. The alternative available at adapter
  level is to drop the posting on a detail failure so the next crawl sees it as new, which trades a
  permanent body-less row for a temporarily absent one and contradicts the documented rule that a
  posting is never lost over a missing detail. Closing the hole properly means a seen-set that
  excludes description-less rows — a change to the hydration contract and its SQL, shared by every
  hydrating adapter, not something to fork inside SEEK. Rare in practice: 31 of 31 details succeeded
  on the live verification run.

## Migration Plan

Additive: a new adapter, a new board file, one registration line. Deploy needs one ingest cron entry
for `sources/seek.yml`. Rollback is removing that entry — the crawled jobs then age out through the
normal unseen sweep.

## Open Questions

None blocking. Whether the five over-window slices need a state split is a question the first weeks
of production data answer, not a design-time one.
