## Context

`/insights/roles/<category>` (web/src/routes/insights/roles/[category]/) ranks a
category's seniorities from `insights_role_stats`. Every insights figure is precomputed
by `cmd/rollup-stats` in one transaction and served by a public aggregate-only endpoint;
none is counted on a request path. This change adds three per-category figures with a
30-day direction, and builds on `insights-role-country-matrix` (#3947), which introduced the
existence rule (`LEAST(posted_at, created_at)`) and the all-seniority
`insights_role_country_stats` table.

## Measurements (prod, 2026-10-30, canonical public categorized postings)

- `job_daily_views` holds days from 2026-08-25, so two 30-day windows fit (66 days).
  Page views per category over 30 days range from ~2k (blockchain, developer_relations)
  to 1.1M (healthcare). Canonical public postings carry 89% of all page views.
- Closures in the last 60 days by `closed_reason` (last 30 / the 30 before, median days
  from existence to close):

  | reason | last 30 | before | median |
  |---|---|---|---|
  | unseen | 1,345,606 | 727,657 | 32.3 |
  | probe_expired | 60,632 | 4,272 | 15.2 |
  | expired | 55,002 | 118,987 | 45.3 |
  | feed_empty | 13,181 | 0 | 134.2 |
  | board_unreachable | 4,968 | 2,891 | 88.7 |
  | feed_removed | 2,716 | 2,952 | 21.8 |
  | board_retired | 1,569 | 0 | 76.8 |
  | moderated | 60 | 9 | 31.6 |
  | '' (unknown) | 0 | 268 | 367.0 |

- With the qualifying reasons below, every category but three has more than 1,000
  closures in each window; the smallest are blockchain (172 / 177) and
  developer_relations (130 / 90). `it_support` has 0 in the earlier window (the
  category is recent) and `management` 0 in the later one (retired) — the floors below
  suppress their direction.
- Inflow by existence date, last 30 vs the 30 before: healthcare 344,346 vs 135,130,
  software_engineering 85,703 vs 77,631, data_engineering 33,365 vs 41,660 — see the
  Inflow decision for why only the first figure ships.

## Goals / Non-Goals

**Goals:** three honest per-category figures with a direction; the page's open count
covering the whole category.

**Non-Goals:** per-country or per-seniority pulse (the page is per category); a history
chart; changing `insights_company_lifetime` (see Open Questions).

## Decisions

### Competition: page views per posting-day, not per open posting now

`page_uniques`, never `uniques` (AGENTS.md "View counts": `uniques` includes unfiltered
API reads, mostly crawlers). It counts one visitor once per posting per day, so the sum
is visitor-posting-days, not distinct people, and the page says "views".

The denominator is **exposure**: the posting-days the category's postings were visible on
freehire inside the window, i.e. the overlap of `[created_at, coalesce(closed_at, now))`
with the window. Dividing 30 days of views by "open now" would charge a posting added
this morning a month of attention it could not have received, and dividing the earlier
window by `insights_role_country_stats.open_count_prev` would be wrong twice over: that
count reconstructs existence (`LEAST(posted_at, created_at)`), so it includes postings
freehire had not found yet and nobody could have viewed here. **This is the one place
`created_at` alone is the right date**: the question is when the posting was on our
site, not when it existed. Published as views × 30 / exposure — "views per open posting
per month". Measured: software_engineering 4.5 vs 4.5, healthcare 3.4 vs 1.4.

Both windows are anchored on the freshest day `job_daily_views` holds (the rule
`cmd/social-digest` follows), not on `now()`: `cmd/rollup-views` lands a day after it
ends, so a `now()`-anchored window would compare 29 days of views with 30 days of
exposure. Same population on both sides (canonical, public, categorized) — a numerator
and a denominator counting different populations is a quietly wrong ratio.

### Time to close: median existence-to-close over closes that describe the posting

Counted reasons: `unseen` (dropped off a board we crawl), `feed_removed` (the source
reported it taken down), `probe_expired` (two expired liveness probes of the source page),
`employer_closed`. Excluded: `expired` (the 45-day age rule for sources with no close
signal — its median is 45.3 by construction, it measures our rule), `feed_empty` and
`board_unreachable` (our crawler's health), `board_retired`, `moderated` and
`source_misattributed` (our own decisions), `''` (closed before reasons were recorded).

The lifetime starts at existence (`LEAST(posted_at, created_at)`), not first-seen: a
posting published 60 days ago and found five days before it closed lived 60 days, and a
board onboarded in the window would otherwise shorten the figure. A close dated before
existence (clock skew, a re-stamped repost) is left out, as the company rollup does.

Reuse of `insights_company_lifetime`: it fits in spirit, not in shape — it is per company,
over 180 days, first-seen-based, and has no window pair — so the rule (which reasons
count) is reused and the query is not.

### Inflow: an existence-dated count with NO comparison

New postings = existence date inside the last 30 days. Counting by `created_at` would
publish our onboarding schedule as hiring (the whole point of
`insights-role-country-matrix`).

There is deliberately no earlier window, because every way of measuring one tracked our
coverage growth more than the market. A board first crawled this month shows its
postings published in the earlier window only if they were still open when we found it,
so the earlier count is a floor wherever coverage grew. Measured 2026-10-30, new
postings last 30 days vs the 30 before:

| category | existence rule | stable employers | stable (source, employer) | found ≤3 days after publication |
|---|---|---|---|---|
| healthcare | +155% | +100% | +107% | +392% |
| skilled_trades | +116% | +64% | +53% | +182% |
| retail | +100% | +40% | — | +86% |
| sales | +51% | +3% | +3% | +44% |
| software_engineering | +11% | −9% | −16% | +8% |
| devops | −1% | −16% | −21% | −2% |
| data_engineering | −20% | −33% | −40% | −21% |

Where coverage barely moved (devops, data_engineering, software_engineering) the rules
stay within ~20 points of each other; where boards were onboarded they swing by 50 to
250 points, so the sign of the change is the only stable part and only in the stable
categories. The panels do not remove the effect, and the "fresh discovery" rule
makes it worse: it drops late-found postings, which on a newly onboarded board are
exactly its earlier-window ones (healthcare: it removes 67% of the earlier window and
37% of the current one), and the result moves with the threshold (1 day +400%, 7 days
+468%). The count alone is honest; its direction is not, so none is published.

### Raw figures on the wire, floors in the page helper

The endpoint returns the raw figures (views/exposure and closes/median for both windows,
inflow for the last 30 days) plus the
category's all-seniority open count, like `/insights/role-countries` returns raw cells:
the floor below which a ratio is noise is a presentation choice. The floors live in one
pure helper (`web/src/lib/rolePulse.ts`): competition needs ≥ 300 posting-days and ≥ 50
views in a window; time to close ≥ 30 closes; a direction needs both windows to clear
the floor, and a change under 5% reads as "about the same".

### One table, one row per category

`insights_role_pulse` (category PK). Counts are bigint (squawk's
`prefer-bigint-over-int`; page-view sums are the largest figures in the family),
exposure is `double precision`, medians `real` and nullable (no qualifying close is
"no figure", not zero). `views_through` records the anchor day so the page can say
which days the views cover.

### The intro count

`rolesIntro` takes the category's open count from `insights_role_country_stats`
(country `''`, canonical public postings of every seniority), served in the pulse
response — one call, and the figure a search for the category can show.

## Risks / Trade-offs

- [Rollup runtime] The rebuild adds one grouped scan of `jobs` limited to postings open
  or closed within ~60 days and one join of 60 days of `job_daily_views` (~26M rows).
  The final query, run as a plain SELECT on prod (2026-10-30, load ~20 on 12 cores),
  took 7m28s. → It runs inside the existing `work_mem = 256MB` transaction, so readers
  keep the old snapshot meanwhile; watch the first run's duration, and split the views
  pass out if the rollup's schedule cannot absorb it.
- [Inflow without context] A count with no direction says less → it is the part that
  survives measurement; revisit once coverage has been stable for two windows.
- [Category reclassification] A dictionary change moves past postings between categories
  only where `backfill-derive` re-derived them, so a window pair can straddle two
  classifications (`it_support`, `management`). → The floors hide a direction whose
  earlier window is empty or tiny.

## Migration Plan

Additive: new table, then the binary. `release.sh` applies migrations before the flip.
Until the first rollup run after deploy the table is empty and the endpoint answers zeros,
which the page renders as "not enough data yet". Rollback: drop the route; the table is
inert.

## Open Questions

- `insights_company_lifetime` excludes `probe_expired` although its own comment counts
  "the posting left its source" — the liveness probe is exactly that. Out of scope here;
  flagged for the owner.
