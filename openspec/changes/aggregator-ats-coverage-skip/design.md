## Context

`cmd/ingest` runs one board file per invocation (cron-scheduled, staggered). For an
aggregator provider (Himalayas, EchoJobs, RemoteOK, HH, ~103 total per
`sources.AggregatorProviders`), the fetch returns postings for many companies in one call,
regardless of whether each company already has a first-party ATS posting. Every posting is
written (`internal/pipeline.saveOne` → `Store.Save`), enqueued into the search outbox, and
only later — in a separate `cmd/reindex` pass (`aggregator-ats-dedup`) — matched against an
ATS twin (same folded company, normalized title, compatible country) and marked
`duplicate_of`, hiding it from search. The write, index churn, and outbox entry already
happened by then.

A prod measurement (2026-08-13) against the `jobs` table: 18,580 distinct companies have a
Himalayas posting; 10,531 (57%) have no non-aggregator posting at all. The complementary
8,049 companies (43%) are companies Himalayas duplicates for no benefit — repeated, to
varying degrees, across every other aggregator provider.

## Goals / Non-Goals

**Goals:**
- Skip the write for an aggregator posting when its company already has an open posting from
  any non-aggregator source, before that write happens.
- Apply uniformly to every `KindAggregator` provider, not just Himalayas.
- Stay consistent with the existing `aggregator-ats-dedup` reindex pass's notion of
  "coverage" (any non-aggregator source, folded company match) so the two mechanisms don't
  disagree about what counts as covered.

**Non-Goals:**
- Not replacing `aggregator-ats-dedup` in `cmd/reindex`. It remains the correctness backstop
  for races this gate cannot close (see Risks).
- Not per-title matching. `aggregator-ats-dedup` matches on (folded company, normalized
  title, compatible country); this gate is coarser — company-level only. A company covered
  for role A but not role B still has role B's aggregator copy skipped. Accepted: simpler,
  cheaper, and the common case (an ATS board lists a company's whole hiring) dominates.
- No new Postgres migration or table.
- No change to ATS-provider ingest (greenhouse.yml et al.) — the gate only evaluates for
  `KindAggregator` providers.

## Decisions

### Coverage definition: any open non-aggregator posting, folded company match

A company (folded `company_slug`, i.e. `replace(company_slug, '-', '')` — the same fold
`aggregator-ats-dedup` already uses, since two sources can spell one employer with a
different word break, e.g. "Cfoinsights" vs "CFO Insights") is "covered" when it has at
least one OPEN posting whose `source` is NOT in the aggregator set. This mirrors
`SuppressAggregatorDuplicatesForCompanies`'s ATS-side predicate exactly (`closed_at IS NULL
AND NOT (source = ANY(aggregators))`) — deliberately broader than "ATS platforms only": a
single-company careers-page adapter (Apple, Google, ...) also counts as coverage, same as it
does in the existing suppression pass.

The aggregator set MUST come from `sources.Taxonomy()` (`All(nil)`), never the
credential-gated `sources.All(client)` registry the ingest host crawls with. This is the
exact trap already documented for the reindex pass: a keyed adapter (e.g. `whatjobs`) whose
credential lives only on a different host would be misclassified as non-aggregator wherever
the credential is unset, and its copies would go ungated.

### Data source: live Meili lookup, not a cached Postgres table

**Considered: a small Postgres table** (`fcompany text primary key`), refreshed every 3h by
`cmd/reindex`. Rejected — querying it fresh at every aggregator ingest run would mean a scan
over the ~2.9M-row open-jobs partial index per run (before the `source` residual filter),
repeated across ~100 aggregator board files; a cached table avoids the repeated scan but adds
a new migration, a new write path needing cheap-write discipline (per the `jobs`/`companies`
write-amplification history, PR #1520), and 3h-stale coverage.

**Considered: Redis.** Rejected — no cron worker uses Redis today (`cmd/ingest` needs only
`DATABASE_URL` per repo convention); it's used solely by `cmd/server`'s rate limiter. Adding
it to ~100 ingest units for this alone is a new dependency for a system nothing else in the
batch/cron tier touches.

**Chosen: the live Meili `jobs` index.** `company_slug` and `source` are already filterable
attributes (`internal/search/client.go`), and the index is kept near-real-time by the
`search_outbox` drain (minutes, not hours) — fresher than a 3h table, no new migration, no
new write path. Meili is already a dependency of four other workers (embed, search-drain,
rollup-facets, reindex-companies), so adding it to aggregator-provider ingest is a smaller
operational step than introducing Redis to the batch tier.

**Trade-off accepted:** `cmd/ingest` gains a new optional dependency on `MEILI_URL`, needed
only for aggregator-provider board files (ATS board files leave the port nil and are
unaffected). Requires a systemd/env change on the aggregator ingest units at deploy time.

**Query shape:** once per board fetch (not per posting) — collect the distinct
`company_slug` values from the adapter's raw fetch result, batch them (~500 per query, to
keep payloads small), and for each batch:

```
filter: company_slug IN [batch] AND NOT source IN [aggregator list]
facets: [company_slug]
```

The returned facet distribution's keys are the covered subset of the batch. Union across
batches into one `map[string]bool` for the run.

### Plumbing: an optional Runner port, explicit parameter threading

`internal/pipeline.Runner` already has this shape for optional capabilities —
`BoardHealth`/`Closer`/`Toucher`/`SeenLookup` — nil disables the feature, so non-aggregator
boards and every existing test fake are unaffected by default. This change adds one more:

```go
type CoverageLookup interface {
    NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error)
}
```

`Runner.Coverage CoverageLookup` (new field, parallel to `Runner.BoardHealth`). In
`ingestFetched`, once per board: if `r.Coverage != nil` AND
`sources.ProviderKind(sources.Taxonomy(), e.Provider) == sources.KindAggregator`, resolve the
covered set and thread it down through `saveOne` as an explicit parameter — the same style
already used for `rej *rejections` / `firstErr *error`, not a new mutable Runner field set
mid-run.

In `saveOne`, after the existing `outOfCatalogue` check and before `r.save`: skip the write
if the posting's company is in the covered set, and increment a new `Stats.ATSCovered`
counter — kept separate from `Rejected` so a board's log line doesn't conflate "non-technical"
with "already covered elsewhere":

```go
type Stats struct {
    Ingested   int
    Failed     int
    Skipped    int
    Cooled     int
    Rejected   int
    ATSCovered int // aggregator postings skipped: company already covered by a non-aggregator source
}
```

`Stats.add` gains `s.ATSCovered += o.ATSCovered`. Logging mirrors `rejections.log` — one line
per board, only when `ATSCovered > 0`:

```
ingest: %s board %q (%s): skipped %d/%d postings — company already covered by a non-aggregator source
```

### Where the Meili adapter lives

A small adapter in `internal/search` implementing `pipeline.CoverageLookup` against the
existing Meili client, doing the batched filter+facet query above. `cmd/ingest/main.go` wires
it into `Runner.Coverage` when `MEILI_URL` is set; leaves it nil otherwise (degrades to
today's behavior — write everything, let the reindex pass suppress later).

## Risks / Trade-offs

- **[Risk] Aggregator run precedes a brand-new company's first ATS crawl.** Meili doesn't
  know about the company yet, so the gate doesn't fire. → Mitigation: identical to today's
  behavior (write it; `aggregator-ats-dedup` suppresses it once the ATS row lands and is
  indexed). Not a regression.
- **[Risk] Search-outbox drain lag.** A non-aggregator posting saved moments before the
  aggregator run queries Meili may not be indexed yet, so the gate misses it. → Mitigation:
  same fallback as above — `aggregator-ats-dedup` catches it on the next reindex cycle.
- **[Risk] New operational dependency.** Aggregator ingest units need `MEILI_URL` added,
  where they previously needed only `DATABASE_URL`. → Mitigation: the port is nil-safe; a
  misconfigured or Meili-down environment degrades to current behavior (over-ingest,
  suppressed later) rather than failing the crawl.
- **[Trade-off] Company-level, not title-level.** A company-wide skip can drop a genuinely
  unique aggregator-only posting (a role the ATS board doesn't list) for an otherwise-covered
  company. Accepted per the proposal discussion — the common case dominates, and title-level
  matching at ingest time would need to parse and normalize every posting before deciding,
  eroding most of the write savings this change exists to capture.

## Migration Plan

No schema migration. Deploy order:
1. Ship the code (new port defaults to nil — no behavior change until wired).
2. Add `MEILI_URL` to the aggregator-provider ingest systemd units.
3. Wire `Runner.Coverage` in `cmd/ingest/main.go` when `MEILI_URL` is set (same deploy as
   step 1, gated by the env var already being absent everywhere until step 2 lands — so the
   safe order is actually: ship code + wiring together, then add `MEILI_URL` per-unit
   whenever convenient; each unit gets the gate the next time it's crawled after the env
   var is added).

Rollback: unset `MEILI_URL` on the affected units (or revert the deploy) — the port reverts
to nil and ingest behaves exactly as it does today.

## Open Questions

None outstanding — scope, coverage semantics, and data source were resolved during
brainstorming (see proposal.md's linked prod measurement and the discussion it cites).
