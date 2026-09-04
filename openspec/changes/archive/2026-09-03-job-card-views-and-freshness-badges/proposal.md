## Why

A job card today tells you what the role is and how old the posting is, but
nothing about whether it is worth being early to. Two things that answer that
already exist and already work — `view_count`, and the `New` / `Be an early
applicant` badge pair — and neither reaches the card. A fresh posting nobody has
opened looks identical to a three-month-old one with four thousand views.

The badges are the notable case: `web/src/lib/freshness.ts` computes them,
`freshness.test.ts` covers them, and `JobView.svelte` renders them — on the job
detail page only. The card, which is where jobs are actually scanned, is the one
surface the signal never reaches.

The search side is likewise nearly there: the index document already carries
`view_count`, so ordering by it costs two allowlist entries rather than a
pipeline.

## What Changes

**The card shows how many people opened the posting.** `JobRow.svelte`'s header
rail renders an eye glyph and the count beside the existing relative timestamp
(`231 · 2d`). Thousands abbreviate via the existing `formatCount` helper. A count
of `0` renders nothing rather than a dead "0 views".

**The rail's "you have viewed this" marker changes glyph.** It is an eye today,
and the count is also an eye — two eyes in one rail, meaning different things, is
unreadable. The personal marker becomes a check.

**The existing freshness badges are rendered on the card.** The card calls the
same `freshnessBadges` the detail page calls. Its rules are unchanged:

- `New` — posted within 7 days.
- `Be an early applicant` — posted within 3 days AND at most 3 users have marked
  the job applied here.
- Neither badge when the reality signal reports anything but `fresh`, or reports
  `fake_freshness`.

Reusing the rule rather than writing a second one is the point: the card and the
detail page must not tell different stories about the same posting, and the rule
already handles a case a fresh implementation would have walked into — a source
that rewrites its posting date on every crawl would otherwise print `New` on the
oldest job in the catalogue.

**On the card, an absent reality signal suppresses the badges.** This is the one
behavioural addition. `freshnessBadges` lets the date stand alone when the signal
is missing, which is correct on the detail page (where the signal is always
computed) and wrong on a card: the browse feed is served from `/jobs/search`,
whose hits carry `reality`, but the plain `/jobs` list and the tracking/assistant
card projection do not. Trusting the date there would print `New` on exactly the
postings the guard exists to catch. The card therefore requires the signal, and
surfaces that do not carry it show no badges.

**These badges are captured in a spec for the first time.** No existing
capability owns them — the rule, its thresholds and the reasoning behind them
live only in source comments. The new capability records them alongside the card
surface, so the next person to retune a threshold sees the argument.

**The feed gains a `Most viewed` ordering.** A new `views` value in the client
sort vocabulary, serialized as `sort=view_count`, added to the endpoint's sort
allowlist and the index's sortable attributes. It needs neither query text nor a
profile, so it is always on offer.

No new column, no migration, no queue, no backfill, and no change to any existing
badge rule. Nothing is removed and no wire field changes shape, so there are no
breaking changes.

## Capabilities

### New Capabilities

- `job-freshness-badges`: the `New` / `Be an early applicant` pair — the existing
  thresholds and the reality gate behind them, the honesty constraint on what
  `Be an early applicant` may claim, the surfaces that render them, and the
  card's stricter requirement that the reality signal be present.

### Modified Capabilities

- `job-engagement-counts`: the requirement covering where the SPA displays the
  counters names the job detail page only. It gains the job card as a second
  display surface, showing `view_count` alone, abbreviated, with the same
  zero-omission rule.
- `web-frontend`: the requirement marking already-viewed jobs in the browse list
  specifies an eye glyph for the personal marker. That glyph becomes a check,
  because the eye now carries the public view count in the same rail. The dimming
  behaviour is unchanged.
- `jobs-list-controls`: the feed's ordering vocabulary is `relevance` / `newest`
  / `match`. It gains `views`, serialized as `sort=view_count`, always offered.
- `job-search`: the endpoint's accepted `sort` values and the index's sortable
  attributes both gain `view_count`.
- `saved-searches`: the canonical query that identifies a saved set stops
  carrying the ordering. This closes a pre-existing hole that this change would
  otherwise make routine: `savedSearchQuery` included `sort` while the comment
  above its call site said it did not and the digest matcher ignored it, so
  choosing an ordering marked a saved search dirty and saving again created a
  duplicate set that mailed identical jobs. `views` is never a contextual
  default, so it always serializes — and the sort control, previously hidden on
  a signed-out browse, is now always rendered.

## Impact

**Frontend** (`web/`)

- `web/src/lib/components/JobRow.svelte` — the rail gains the count, the viewed
  marker's glyph changes, the signal row gains the badges and its render guard
  widens (a job whose only signal is a badge must still open the row).
- `web/src/lib/freshness.ts` — one added export: the card's reality-requiring
  wrapper. The existing `freshnessBadges` and all three thresholds are untouched.
- `web/src/lib/utils.ts` — `formatCount` moves here from `activityChart.ts`, so
  the card and the chart share one abbreviation rule rather than the card
  importing a chart module. `activityChart.ts` imports it from its new home.
- `web/src/lib/facetModel.ts` — `JobSort`, `SORT_PARAM`, `SORT_LABEL`,
  `sortOptionsFor`.

**Backend** (Go)

- `internal/api/handler/search.go` — one entry in `searchSortable`. `searchSort`
  already resolves `order` and defaults to `desc`.
- `internal/search/search/client.go` — one entry in `SortableAttributes`.

**Not touched**

- No `jobs` column, no migration, no sqlc regeneration. `view_count` is already a
  column, already on the public projection, and already in the index document —
  `JobDocument` embeds `jobview.Job`.
- No badge threshold, and no change to `JobView.svelte`. The detail page keeps
  rendering exactly what it renders today.
- No `search_outbox` plumbing. `cmd/rollup-views` bumps the counter daily without
  enqueueing, but the scheduled full rebuild reads from Postgres every few hours,
  so the indexed figure is fresher than the daily rollup that produces it.
- `GET /api/v1/jobs` accepts no `sort` at all (hardcoded `created_at DESC`). The
  sort control lives entirely on `/jobs/search`, which is what the browse feed
  calls.

**Deploy hazards**

1. The handler must not accept `sort=view_count` before the live index declares
   the attribute sortable. Meilisearch rejects the **whole query** for an
   undeclared sort attribute, and the handler maps any Meili error to 500 — so
   the failure is a broken search, not an ignored parameter. This is the same
   ordering hazard `internal/search/search/AGENTS.md` records for filterable
   attributes and for the match embedder.
2. There is no operator script that patches live index settings — `EnsureIndex`
   is called only from tests, and settings otherwise reach production through a
   rebuild's swap. Patching by hand means sending the **complete**
   `sortableAttributes` list: Meilisearch replaces that setting wholesale, so
   sending only `view_count` would drop `posted_at` and break the feed's default
   ordering for everyone.
3. A rebuild silently refuses below the free-disk floor. As of 2026-09-03 one is
   in flight with the floor temporarily lowered; confirm it has landed before
   relying on it to carry the setting.
