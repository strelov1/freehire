## Context

WhatJobs is a CPC network where freehire is an approved publisher (`publisher=7065`). Its FeedAPI
answers `GET https://api.whatjobs.com/api/v1/jobs.json` with a keyword-searchable slice of US
inventory — 594k postings by its own count, of which the tech slices are modest (`software
engineer` 12.5k, `devops engineer` 1.3k, `golang` 108).

Two properties of the feed shape the whole design.

**The tracked URL is stable.** `url` is `whatjobs.com/pub_api__cpl__<jobID>__<publisherID>`, and
three different `user_ip` values return a byte-identical URL. This is the opposite of CareerJet,
whose `jobviewtrack.com/v2/<token>` is IP-bound and therefore forced a live per-click resolver.
Here the URL can simply be stored, so `whatjobs` rides the ordinary crawl path with no new
click-time machinery.

**Nothing about a posting can be verified.** The URL is a billing landing page, not the employer's
posting, so `cmd/liveness` can never confirm a role still exists — and it will not try, because it
only probes jobs whose source is *not* a registered board provider. Meanwhile the inventory is old:
56% of a 738-posting sample is over 90 days, the oldest 413 days. The only defence left is the
unseen sweep, which makes the sweep's correctness the central design problem.

The vendor documentation is unreliable and was corrected against the live API: `snippet` is the
full description rather than a highlighted excerpt, the documented `onmousedown` field does not
exist, `unique_id` does not deduplicate, an invalid publisher answers `410` rather than `422`, and
every code sample in the docs is broken because a `/` in the `user_agent` parameter makes the edge
redirect with the value corrupted to `Mozilla%215.0`.

## Goals / Non-Goals

**Goals:**

- Ingest the feed's tech slices through the existing pipeline, gaining dedup, enrichment outbox,
  board health and incremental search indexing for free.
- Keep the publisher id out of the repository, in the environment only.
- Never let an unverifiable posting masquerade as a fresh, salaried, or employer-hosted one.
- Do not let partial keyword coverage cause false closes.

**Non-Goals:**

- Filtering staffing intermediaries (~35% of the feed's companies). Explicitly declined — the
  postings are real.
- A click-proxy endpoint hiding the publisher id from the public API. Declined; the source name
  and tracked URL are published openly.
- Non-US inventory. The publisher id is per-country and this one serves US only; `country`,
  `locale` and `geo` parameters are ignored by the API.
- Recovering the employer's own posting URL. Not obtainable from the feed.

## Decisions

### Board is a search keyword, modelled on `hh`

`hh` already establishes this shape: a multi-company aggregator enumerated by a slice id
(`professional_role`), whose `company` in the board file is a display label while the real
employer comes from each posting. `whatjobs` reuses it with the keyword as board, so it is
`aggregator()` but neither `boardless()` (the keyword is required) nor `fullCatalog()` (a crawl
sees one slice).

*Alternative considered:* a boardless adapter with keywords hardcoded in Go. Rejected — it would
lose per-keyword `board_health`, independent cooldown, and the ability to tune the keyword list
without a deploy.

### Provider-declared sweep grace window, not a new closing mechanism

The 48-hour sweep is unsafe here: a posting that drifts past the crawl's page budget looks unseen,
gets closed, and reopens on a later run when it drifts back — churn that also pollutes
`job_daily_stats`. But `CloseUnseenJobs` already takes its cutoff from the caller ("The caller
passes the crawled slugs and owns the grace window"), so no new SQL and no new closing path is
needed. An adapter declares a wider window; `cmd/ingest` uses it when computing the cutoff.

The marker follows the existing family (`selfClosing`, `fullCatalog`, `aggregator`) but carries a
value, so it needs an accessor for `cmd/ingest` outside the package — mirroring
`SelfClosingProviders`:

```go
type sweepGrace interface{ sweepGrace() time.Duration }

// SweepGraceWindows returns the widened sweep windows the registry's adapters declare.
func SweepGraceWindows(reg map[string]Source) map[string]time.Duration
```

`whatjobs` declares **14 days**: long enough that page drift and a skipped cron cannot close a live
posting, short enough that a role withdrawn from the feed leaves the catalogue within a fortnight.

*Alternatives considered:* (a) a `ttlBound` marker closing on `created_at + N` regardless of the
feed — rejected, it would close postings the feed still lists, contradicting the reappearance rule;
(b) crawling every keyword to full depth so coverage is complete — rejected, `software engineer`
alone is 250 pages and the feed's ceiling (~2000 pages) makes completeness unattainable anyway;
(c) marking it `selfClosing` to skip the sweep entirely — rejected, that marker means the adapter
emits authoritative removal events, which this feed does not.

### Duplicates across keywords are accepted, not engineered away

`external_id` is namespaced `"<board>:<native-id>"` by the pipeline unconditionally, so a posting
matching two keywords becomes two rows. Measured overlap is small — 742 rows across 8 keywords
collapsed to 738 unique postings, 0.5% — and the existing cluster dedup
(`aggregator-ats-dedup`, `job-cluster-copies`) is what collapses copies for display. Keywords are
chosen to minimise overlap rather than changing a namespacing rule the rest of the catalogue
depends on.

### Page budget is bounded and logged when hit

Pagination stops on an empty page, at the feed's ceiling, or at a per-board page budget
(**40 pages**, ≈2000 postings). A page returning fewer rows than requested is *not* an
end-of-results signal: the feed post-filters duplicates, so `limit=50` routinely yields 44. When
the budget is what stopped a crawl, the run logs it — a bounded crawl must not read as complete
coverage.

`limit` is sent as **50** and never as `1`: values above 50 are silently clamped, and `limit=1`
combined with a keyword returns an empty `data` with `per_page: 0`, a pagination bug in the feed.

### Identity from the URL, junk fields dropped

The native id is the `pub_api__cpl__(\d+)__` capture; a posting whose URL lacks it is skipped
rather than stored under a guessed id. `salary` (always `0.000000 - 0.000000`), `job_type` (always
empty) and `logo` (always null) are ignored. The trailing `#J-<digits>-Ljbffr` reseller signature —
on 96% of descriptions — is stripped.

`age_days` is dropped rather than mapped to `posted_at`: 15 postings from unrelated companies
shared `age_days: 109`, which shows it measures the record's age in the reseller's index, not the
role's publication date. Leaving `posted_at` unset lets `EffectivePostedAt` fall back to
`created_at`, so these postings sort by when freehire first saw them — true, if less precise.

### Country is asserted from the publisher id, not guessed from the city

`location` carries a bare city and `postcode` a US ZIP; no country field exists. The publisher id
is per-country by the vendor's own design and this one serves US inventory (625 of 626 postcodes
are US ZIPs). The adapter therefore states the country for the geography dictionary rather than
leaving it to infer "Vienna" or "London" — both of which appear in this feed as US cities
(London, Ohio 43140). This is a fact about the configured account, not a guess about a posting, so
it does not breach the dict-only rule.

Measured against `location.Parse` over the 738-posting sample (102 of which carry no location at
all):

| | bare city | city + `", United States"` |
|---|---|---|
| no country resolved | 218 | 0 |
| a foreign country resolved | 11 | 11 |
| US resolved | 407 | 636 |

So the suffix rescues 229 postings and regresses none. It does not *fix* the 11 homonyms — the
dictionary unions the tokens rather than letting the country arbitrate, so "Dublin, United States"
resolves to `ie` **and** `us`. That is still strictly better than the bare city, which resolves to
`ie` alone: the posting becomes findable under the US filter it belongs to, while remaining wrongly
findable under Ireland's. Narrowing that last 1.7% needs the dictionary to treat an explicit
country as authoritative over a city-name match, which is a change to `internal/location` and out
of scope here.

### Request hygiene

`user_agent` is omitted entirely — it is optional, it only improves click attribution on shared
IPs, and any slash in it breaks the request. `user_ip` is mandatory, but a crawl is not a user
viewing a page, so it carries a fixed placeholder; the vendor ignores tracking on values it
dislikes and still serves results. `unique_id` is not used.

## Risks / Trade-offs

- **Prod datacenter IP may be challenged by the edge.** Verification ran from a residential IP; a
  prod crawl could meet the Cloudflare validation page the click-through already redirects to.
  → `sources.ApplyProxyEgress` exists for exactly this; if the board fails on prod, add `whatjobs`
  to `proxiedProviders`. Board health surfaces it within one cycle rather than silently.
- **Ghost jobs persist for up to 14 days after withdrawal.** The widened window is the price of not
  false-closing, and no probe can shorten it.
  → Accepted deliberately. The 14 days bound the damage; without the window the churn would be
  worse and would corrupt daily stats.
- **City-multiplied postings survive dedup.** One remote role appears under several cities with
  words reordered in the title (CAI ×3), so `role_fingerprint`, which hashes visible text, will not
  collapse them.
  → Out of scope here; it is the same class of problem `fuzzy-description-role-dedup` addresses.
  Logged as a known limitation rather than patched inside this adapter.
- **A silently truncated crawl looks like a small keyword.** The feed answers `200` with fewer rows
  rather than erroring.
  → The provider is not `fullCatalog`, so the source-scoped close that punishes truncation is never
  reached; the company-scoped sweep plus the 14-day window absorbs it.
- **Terms of use permit caching only presumptively.** The publisher terms link in the vendor's
  documentation is an unfilled placeholder, so the right to store descriptions is unconfirmed.
  → Ask the network directly before the board list grows; storing a reseller's full descriptions is
  the plausible cause of an account being closed.

## Migration Plan

1. Merge with `WHATJOBS_PUBLISHER_ID` unset everywhere — the provider stays unregistered and
   nothing changes for any existing crawl.
2. Set the variable in the prod ingest worker's env file, then run one board file on prod
   (`sources/whatjobs.yml`) and inspect counts before adding it to cron.
3. Rollback is unsetting the variable: the provider disappears from the registry, and its jobs
   close on the ordinary sweep once nothing re-lists them.

No migration is needed — the change adds no column and alters no stored shape.

## Open Questions

- Confirm with WhatJobs whether caching descriptions and tracked URLs is permitted, since their
  documented terms link is empty.
- The keyword list in `sources/whatjobs.yml` is seeded from the slices measured during
  verification; its shape should be revisited once real click-through data shows which slices
  convert.
