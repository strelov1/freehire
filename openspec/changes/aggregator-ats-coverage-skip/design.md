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
- Stay AS CLOSE AS THE LIVE BACKEND ALLOWS to the existing `aggregator-ats-dedup` reindex
  pass's notion of "coverage" (any non-aggregator source). Exact match only, NOT the reindex
  pass's folded-company match — see "Coverage definition" for why, and Risks for the gap this
  leaves for the reindex pass to close.

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

### Coverage definition: any open non-aggregator posting, EXACT company_slug match

A company (`company_slug`, exact string equality) is "covered" when it has at least one OPEN
posting whose `source` is NOT in the aggregator set. This mirrors
`SuppressAggregatorDuplicatesForCompanies`'s ATS-side predicate on the source condition
exactly (`closed_at IS NULL AND NOT (source = ANY(aggregators))`) — deliberately broader than
"ATS platforms only": a single-company careers-page adapter (Apple, Google, ...) also counts
as coverage, same as it does in the existing suppression pass.

**Deviation from the reindex pass: NO folding.** `aggregator-ats-dedup` compares
`replace(company_slug, '-', '')` (stripped of hyphens) because two sources can spell one
employer with a different word break (e.g. "Cfoinsights" vs "CFO Insights" → `cfoinsights` vs
`cfo-insights`) — it can afford this because it runs a Postgres query, where an expression
index makes the fold cheap. The live Meili lookup (see "Data source" below) cannot: Meili
filters match a stored field's LITERAL value, with no equivalent of a SQL expression index —
there is no way to ask "does any document's `company_slug`, with hyphens stripped, equal
this folded string?" **Folding the query value before sending it to Meili is not a partial
mitigation — it actively breaks the common case too**: if the pipeline folds its own posting's
slug from `cfo-insights` to `cfoinsights` before querying, that query no longer matches even a
document whose `company_slug` is genuinely stored as `cfo-insights` (the ordinary,
non-hyphenation-mismatched case). So the live gate compares `company_slug` values EXACTLY AS
COMPUTED (`normalize.Slug`, no fold) on both sides. A same-employer pair that only agrees
after folding is coverage this gate will miss — accepted, because `aggregator-ats-dedup`
already exists specifically to catch it (see Risks).

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

**Trade-off accepted:** `cmd/ingest` gains a new optional dependency on Meili — gated on
`MEILI_MASTER_KEY` being set (`cfg.MeiliKey != ""`, `cmd/server`'s existing convention for an
optional search dependency; `MEILI_URL` always defaults and can't gate anything), needed only
for aggregator-provider board files (ATS board files leave the port nil and are unaffected).
May require an env change on the aggregator ingest units at deploy time — see Migration Plan
for confirming whether `MEILI_MASTER_KEY` already reaches them.

**Query shape:** for the buffered path, once per board fetch (not per posting) — collect the
distinct `company_slug` values from the adapter's raw fetch result, batch them (~500 per
query, to keep payloads small), and for each batch:

```
filter: company_slug IN [batch] AND NOT source IN [aggregator list]
facets: [company_slug]
```

The returned facet distribution's keys are the covered subset of the batch — exact
`company_slug` values, unfolded, matching what the pipeline sent (see "NO folding" above).
Union across batches into one `map[string]bool` for the run. The streaming path (see "Two
call sites, one abstraction" below) issues the same query shape with a single-element batch,
once per distinct company encountered rather than once per board.

### Plumbing: an optional Runner port, explicit parameter threading

`internal/pipeline.Runner` already has this shape for optional capabilities —
`BoardHealth`/`Closer`/`Toucher`/`SeenLookup` — nil disables the feature, so non-aggregator
boards and every existing test fake are unaffected by default. This change adds one more:

```go
type CoverageLookup interface {
    NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error)
}
```

`Runner.Coverage CoverageLookup` (new field, parallel to `Runner.BoardHealth`).

**Two call sites, one abstraction.** `saveOne` is shared by both the buffered path
(`ingestFetched`, which has the whole board's raw postings up front) and the streaming path
(`ingestStream`, which only ever has the one posting `FetchStream`'s callback just emitted —
`jobtech` is the one aggregator provider on this path today). A single precomputed
`map[string]bool` fits only the buffered caller, so `saveOne` instead takes a small resolver:

```go
// aggregatorCoverage answers whether a company is already covered by a non-aggregator
// source, for the board being ingested. nil when the gate does not apply (non-aggregator
// provider, or no Coverage port wired) — saveOne treats a nil resolver as "never covered".
type aggregatorCoverage func(companySlug string) bool
```

- **Buffered path**: if `r.Coverage != nil` AND `sources.ProviderKind(sources.Taxonomy(),
  e.Provider) == sources.KindAggregator`, `ingestFetched` collects the distinct company slugs
  from `raw` up front, resolves them in the batched Meili call (one round trip, or a few at
  ~500/batch), and wraps the resulting map in a closure — a cheap in-memory lookup per
  posting, same cost profile as today's plan.
- **Streaming path**: under the same gate condition, `ingestStream` wraps a closure that
  calls `r.Coverage.NonAggregatorCompanies` with a single-element slice per distinct company,
  memoized in a small map local to that board's stream (guarded by the same mutex `emit`
  already holds) so a company with many postings in one stream is looked up once, not once
  per posting. This trades one Meili round trip per distinct company (not per posting) for
  supporting a source that never hands over a full list to batch against.

Both closures share the `aggregatorCoverage` type, so `saveOne` itself does not know or care
which path built it.

In `saveOne`, after the existing `outOfCatalogue` check and before `r.save`: skip the write
if `covered != nil && covered(dj.Fields().CompanySlug)`, and increment a new
`Stats.ATSCovered` counter — kept separate from `Rejected` so a board's log line doesn't
conflate "non-technical" with "already covered elsewhere":

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

Implemented directly as a `NonAggregatorCompanies` method on the existing `*search.Client`
(`internal/search/coverage.go`) — Go's structural interfaces mean `internal/search` needs no
import of `internal/pipeline` for this, so no separate wrapper type is needed. `cmd/ingest/
main.go`'s `coverageLookup(cfg)` wires it into `Runner.Coverage` when `cfg.MeiliKey != ""`;
leaves it nil otherwise (degrades to today's behavior — write everything, let the reindex
pass suppress later).

## Risks / Trade-offs

- **[Risk] Aggregator run precedes a brand-new company's first ATS crawl.** Meili doesn't
  know about the company yet, so the gate doesn't fire. → Mitigation: identical to today's
  behavior (write it; `aggregator-ats-dedup` suppresses it once the ATS row lands and is
  indexed). Not a regression.
- **[Risk] Search-outbox drain lag.** A non-aggregator posting saved moments before the
  aggregator run queries Meili may not be indexed yet, so the gate misses it. → Mitigation:
  same fallback as above — `aggregator-ats-dedup` catches it on the next reindex cycle.
- **[Risk] New operational dependency.** Aggregator ingest units need `MEILI_MASTER_KEY`
  reaching their environment, where they previously needed only `DATABASE_URL` (it may
  already be there — see Migration Plan). → Mitigation: the port is nil-safe; a misconfigured
  or Meili-down environment degrades to current behavior (over-ingest, suppressed later)
  rather than failing the crawl.
- **[Trade-off] Company-level, not title-level.** A company-wide skip can drop a genuinely
  unique aggregator-only posting (a role the ATS board doesn't list) for an otherwise-covered
  company. Accepted per the proposal discussion — the common case dominates, and title-level
  matching at ingest time would need to parse and normalize every posting before deciding,
  eroding most of the write savings this change exists to capture.
- **[Risk] No folded-slug matching on the live gate (found during implementation of the Meili
  adapter, 2026-08-13).** Unlike `aggregator-ats-dedup`'s Postgres query, Meili cannot compare
  `company_slug` with hyphens stripped at filter time — see "Coverage definition"'s "NO
  folding" note for why folding the query value would actively break even the common,
  correctly-matching case. So a same employer spelled with different word breaks across two
  sources (e.g. "Cfoinsights" vs "CFO Insights") is NOT caught by this gate — the aggregator
  copy is still written. → Mitigation: unchanged from the other gaps above —
  `aggregator-ats-dedup` still runs in `cmd/reindex` and catches this case on its own schedule
  via the fold it can afford in SQL. This narrows what "coverage" means for THIS gate
  specifically to exact `company_slug` equality; the periodic reindex pass remains the one
  mechanism with the fuller (folded) definition.

## Migration Plan

No schema migration. The gate is wired on `cfg.MeiliKey != ""` (i.e. `MEILI_MASTER_KEY` set) —
NOT `MEILI_URL`, which always resolves to a default (`http://localhost:7700`) and so can never
signal "search is configured." This corrects an earlier draft of this plan, which named
`MEILI_URL`; `cmd/ingest`'s `coverageLookup` mirrors the exact `cfg.MeiliKey != ""` gate
`cmd/server`'s `searchClient` already uses (see the "Where the Meili adapter lives" section).

**Before deploying, confirm whether `MEILI_MASTER_KEY` is already present fleet-wide** on the
host(s) `cmd/ingest` runs on — four other workers (embed, search-drain, rollup-facets,
reindex-companies) already require it, so it may already sit in a shared env file every ingest
unit sources, rather than being absent until added per-unit. If it's already fleet-wide, this
gate activates for EVERY aggregator ingest process on the very next deploy (all ~103 providers
at once), not gradually per-unit as a staged rollout would — a deliberate decision to make
explicitly, not something to discover after the fact. This is task 6.1's job; do it before
task 6.2's post-deploy check.

Deploy order:
1. Ship the code (new port defaults to nil — no behavior change until `MEILI_MASTER_KEY` is
   present in the environment `cmd/ingest` runs with).
2. Confirm (per the note above) whether that activates the gate immediately or needs adding.

Rollback: unset `MEILI_MASTER_KEY` in the environment `cmd/ingest` runs with (or revert the
deploy) — the port reverts to nil and ingest behaves exactly as it does today.

## Open Questions

None outstanding — scope, coverage semantics, and data source were resolved during
brainstorming (see proposal.md's linked prod measurement and the discussion it cites).
