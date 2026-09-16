## Context

259 source adapters feed the catalogue (measured 2026-09-16 as `len(sources.Taxonomy())`). Three things already know something about them,
and none of them is a page a visitor can read:

- `sources.Taxonomy()` knows what KIND each adapter is (ATS platform, aggregator, company
  career page). `/api/v1/status` already reads it.
- `board_health` knows when each provider was last read, whether that read succeeded, and
  how much it returned. `ProviderHealthRollup` already folds it per provider.
- The dedup pass already knows, per posting, whether an aggregator copy was matched to a
  first-party ATS posting: `jobs.duplicate_of_aggregator`.

What is missing is the per-source job count, the per-source overlap arithmetic, and
anything that puts the three together in front of a person.

Two constraints shape everything below. The host's bottleneck is the crawl fleet, so a
scheduled scan is affordable and a per-request scan is not. And the page is public, so it
faces crawler traffic — which on this host is most of the traffic.

## Goals / Non-Goals

**Goals:**

- One public page listing every source with its job count, freshness and last yield.
- A per-aggregator answer to "what do we get here that an ATS crawl would not have given
  us", measured rather than guessed.
- One request per page load, no database scan on the request path.

**Non-Goals:**

- Per-source detail pages (`/sources/<key>`). Nothing yet needs a second level, and a page
  per adapter is 259 thin pages of duplicate content pointed at a search that already
  exists.
- Historical trend ("this source grew 12% this month"). The snapshot is a snapshot; a
  trend needs a ledger nobody has asked for.
- Changing how dedup, `/status` or `/open` work. This change reads what they produce.

## Decisions

### One new snapshot table, not an extension of `facetsnapshot`

`facetsnapshot` already stores a scheduled value→count distribution from Meilisearch, and
adding `source` to it would supply the de-duplicated count for free. It is the wrong home
anyway: the other three figures this page needs (raw open count, ATS-matched count, sample
URL) come from Postgres, and splitting a source's row across two tables written by two
passes means the page can render a raw count and a de-duplicated count measured hours
apart. The overlap arithmetic would then fail to close, visibly, and nobody would be able
to say why.

So: one `source_stats` table, all four figures, one writer, one `measured_at`.

### Both counts, and the page shows the de-duplicated one

`catalogstats` already carries this pair (`open_jobs` raw, `unique_open_jobs` from Meili)
for exactly this reason, and the same reason applies per source:

- The **raw** count is what the overlap arithmetic is computed from. `duplicate_of_aggregator`
  is a column on a row, so matched + unmatched must equal the raw row count — a
  de-duplicated denominator would make the two halves not add up.
- The **de-duplicated** count is what `/jobs?source=<key>` will show, so it is what the
  card must display. A card saying 12,000 next to a link that opens 4,000 is the trap
  this pair exists to avoid.

### The spine is the registry UNION what the catalogue actually holds

`ProviderHealthRollup` returns one row per provider that HAS a health record. An adapter
that has never been crawled on this host has none, and would silently vanish from a page
whose whole claim is "here is everything we read". So the registry (`sources.Taxonomy()`)
joins the health rollup and the snapshot onto it by key, not the other way round.

The registry alone is not enough either, and this was found rather than predicted:
`telegram` is absent from `Taxonomy()` — it is an extraction pipeline, not a registered
adapter — while carrying real postings, its own display label in the SPA and its own
figure on `/open`. A registry-only spine drops it, and the drop is invisible precisely
because it is a drop.

The two together are still not enough, and this one was found in review: a source in
neither set — `telegram` the day its last posting closes — falls out of the snapshot
entirely. The same silent drop, one closure later. So the rollup's spine is a union of
three: the registry, this run's scan, and the KEYS of the snapshot it replaces (every
figure is re-measured; only the names carry over).

A registered adapter with no postings appears carrying a measured zero; a source with
postings and no adapter appears classified as `other`; a source that has gone quiet keeps
reporting zero. Adding an adapter still requires no edit here.

`sourcestats.Union` is the single place that argues this, and both the rollup and the
endpoint call it — two hand-kept copies of the rule would eventually disagree about who
belongs on the page, and the page would be the last to know.

### The logo host is derived from a stored posting URL

`logo.freehire.me` resolves a brand mark from a name or host. The honest host for a source
is the host its own postings live on: `greenhouse.io` for Greenhouse, the employer's own
domain for a single-company career-page adapter. That host is already in `jobs.url`, so
`min(url)` in the same grouped scan gets it for free — `min` rather than `mode` because a
hash aggregate needs no sort and every posting of a source shares a host.

The alternative was a hand-written `provider → domain` map of 259 entries. Rejected: such a
list goes stale silently, and the entry it is missing is invisible precisely because it is
missing. Deriving it means a source with no postings has no logo, which is the correct
answer rather than a gap.

### The overlap figure is named for what it measures

The wire shape carries `ats_matched_jobs` and `ats_unmatched_jobs`, not `exclusive_jobs`.
The dedup pass not finding a pair is the absence of evidence, not evidence of absence — its
fuzzy and role passes both have real miss rates. A field called `exclusive` would convert
that absence into a claim the first time anyone read it, and the claim would then be quoted
back at us. The page's visible text follows the same rule.

### One aggregate pass, no description column

```sql
SELECT source,
       count(*)                                                    AS open_jobs,
       count(*) FILTER (WHERE duplicate_of_aggregator IS NOT NULL) AS ats_matched,
       min(url)                                                    AS sample_url
FROM jobs
WHERE closed_at IS NULL AND NOT is_private
GROUP BY source;
```

A sequential scan with a hash aggregate over a few hundred groups, touching only narrow
columns, and excluding the private jd-tailor-intake postings — one user's pasted job
description is not part of a public figure. No
index is added for it: an index on `jobs(source)` would be built and maintained on an 11M-row
table for one scheduled query, and `ADD`ing anything to `jobs` on this host has its own hazards.

### The page is a server-rendered payload, memoized

`/open` already establishes the pattern: assemble once, memoize module-side for a short TTL,
serve every visitor in the window from it, cache the degraded build too. Same here. Search
filters the loaded array in the browser — every source is already on the page, so a request
per keystroke would buy nothing and would hand a crawler a way to make us work.

## Risks / Trade-offs

- **The scan is a sequential scan over the jobs table on a saturated host, and it runs
  every 3 hours, not daily.** `deploy/systemd/freehire-rollup-stats.timer` is
  `OnCalendar=*-*-* 00/3:20:00` — eight runs a day. → It reads no description, so no
  de-TOAST; it is read-only, so an over-running pass degrades nothing but its own
  freshness; and it is not a NEW sweep of `jobs` on that cadence — the same run already
  performs several full-table aggregates for the insights rollups, so this narrow scan
  rides a warm page cache rather than adding a sweep. It does run OUTSIDE that
  transaction's `SET LOCAL work_mem = '256MB'`, which is fine at a few hundred groups and
  is the thing to revisit if it ever groups by something wider.

- **A figure up to 3 hours stale, and an overlap figure staler than that.** The dedup
  passes that write `duplicate_of_aggregator` run on their own, deliberately rarer
  schedule, so the overlap bar can lag the job counts beside it by days. → The page states
  when the counts were measured. A source
  catalogue is not a dashboard; freshness at the hour scale is what `/status` is for, and
  this page links there.

- **The overlap figure will be quoted as "exclusive postings" anyway.** → Mitigated by
  naming, not by hope: neither the field name nor the visible text contains the word, so a
  reader who wants to make that claim has to make it themselves.

- **A few hundred rows, and as many logo requests.** → Lazy loading, and the proxy 404s cleanly into a
  monogram. A source with no postings requests nothing at all.

- **A source with a large raw count and a small de-duplicated count will look broken.** →
  That is the finding, not the bug: it means the aggregator is mostly reposting what we
  already hold. The overlap block on the card is what explains it.

## Migration Plan

1. Migration adds `source_stats`. Nothing reads it yet.
2. The rollup pass ships inside `cmd/rollup-stats` (already on a timer, already holds both
   `DATABASE_URL` and the Meilisearch credentials). First run populates the table.
3. The endpoint ships; with an unpopulated table it answers 200 with absent counts.
4. The page ships, linked from the footer and the pages sitemap.

Rollback is per layer and needs no migration: unlink the page, or drop the route. The table
is inert if nothing reads it.

## Open Questions

None blocking. Whether a per-source detail page earns its keep is a question for after the
page has traffic to measure.
