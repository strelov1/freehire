## Context

The brainstormed design, with the full prod measurement table behind every number
here, is at
[docs/superpowers/specs/2026-09-04-job-requirements-surface-design.md](../../../docs/superpowers/specs/2026-09-04-job-requirements-surface-design.md).

`enrich.Enrichment.Requirements` was added to the enrichment prompt on 2026-08-18.
It is bounded by `Sanitize` (30 entries, 200 runes each), passes through
`jobview.FromDomain` unchanged, and is generated into
`web/src/lib/generated/contracts.ts` as `Requirement[]`. Every piece of the
pipeline exists except the last one: no Svelte component reads the field.

Measured on prod 2026-09-04:

| Measure | Value |
| --- | --- |
| Open postings with a non-empty list | 133,404 of 4.6M open (2.9%) |
| Coverage among rows enriched since 2026-08-18 | 93% |
| Entries per posting | ~9.3 |
| `required` / `preferred` | 78% / 22% |
| Entry length | ~70 chars mean, 200 ceiling |
| Distinct entry texts | 86% of entries |
| Open postings with a requirements-shaped heading + list (regex estimate) | 23% |
| Open postings the shipped extractor actually yields on (164 live postings) | 12.8% |

Two of these decide the shape of the work. **86% distinct** means the list can never
be a facet, a filter, or a search field — it is display material and input to the
candidate-side stack, nothing else. **2.9% versus 12.8%** means the deterministic
parse is, today, the wider source by a factor of four.

The two coverage figures differ because the 23% was a regex sweep for a
requirements-shaped heading near a list, and the shipped extractor is stricter than
that on purpose (see "Dictionary-gated extraction" below). 12.8% is the measured
yield of the code that ships.

## Goals / Non-Goals

**Goals:**

- Render the stored list on the job detail page.
- Raise the share of open postings carrying a list from ~3% toward ~25% without a
  single additional model call.
- Keep one served field with one reader, so nothing downstream learns a second
  code path.
- Nothing downstream inherits anything: `matchanalysis` builds its own requirement
  list from a first LLM stage and `coverletter` takes that. An earlier draft claimed
  otherwise and leaned on it; the claim was wrong and is not load-bearing here.

**Non-Goals:**

- Filtering, faceting, or searching on requirements. Ruled out by the 86%-distinct
  measurement, not deferred.
- Normalising near-duplicate phrasings ("excellent written and verbal communication
  skills" vs "strong written and verbal communication skills"). Real, but a
  clustering problem, and this list is display-only.
- Non-English heading vocabulary. Additive to one vocabulary later.
- De-duplicating requirement entries against the skill chips, the experience facet,
  or the education facet already on the page. A quarter of entries restate one of
  those; filtering them would put a second normaliser in `web/` that drifts from
  `internal/dict`.

## Decisions

### The section goes in the reading column, not the sidebar

The sidebar is `lg:grid-cols-[20rem_...]` and already carries the match score,
salary, the facet `<dl>`, provenance and votes. Nine entries averaging 70
characters is roughly fifteen wrapped lines — it would more than double the sticky
card. The section joins `descriptionContent` in `JobView.svelte`, between
`<JobDescription>` and the Skills section, where the column is wide enough to read
a sentence.

*Alternative considered:* a compact sidebar list with a "+5 more" disclosure.
Rejected: the entries are sentences, and truncating them to fit 20rem loses the
part that makes them worth reading.

The grouping is a pure function in `web/src/lib/enrichment.ts` and the component is a
loop over it, because `web/vitest.config.ts` runs plain Node with no Svelte
compilation — its header says so, and all 128 test files are plain TS. That seam is
what makes the branching testable. It also means three of the display spec's
scenarios (the `{#if}` guard, the `lang` attribute, and that the text is escaped
rather than `{@html}`) are enforced by review and by Svelte's own semantics rather
than by a test. Closing that gap needs component tests, which is a change to the
repository's test architecture and belongs in its own change.

### The derived list is stored, not parsed per request

*Alternative considered:* parse `description` in `jobview.FromRow` on the request
path. Cheaper to ship — no migration, no backfill — and a parser fix would reach
every row instantly.

Rejected on smaller but real ground: the backfill needs somewhere to put its answer,
and parsing every job body on every read repeats work whose input never changes.

An earlier draft argued instead that `matchanalysis` and `coverletter` read this
field and would inherit the coverage. They do not — `matchanalysis` builds its own
list from a first LLM stage and `coverletter` takes that one. The decision survives
losing that argument; the argument should not survive being wrong.

### The fold happens on the READ path, not only at write time

`jobview.FromDomain` folds `requirements_derived` into `enrichment.requirements` when
the model stated none — the same dict-wins-over-LLM fold `seniority`, `category` and
`cities` already get there.

The first version of this change had only the `SetJobEnrichment` overlay, and that was
wrong in a way worth recording: **the overlay runs only when the model runs.** A
posting the model has never reached stored a derived list and served nothing, and a
posting already enriched at the current version is never re-queued (the version
deliberately stays at 2), so it would have served its list never rather than
eventually. Since the coverage this feature exists for IS the postings the model has
not reached, the overlay alone delivered none of it.

The overlay stays, for anything reading `jobs.enrichment` directly, but the projection
is what makes the feature true.

### Two producers, one served field

The derived list lives in its own column, `jobs.requirements_derived`, and is
merged into `enrichment.requirements` at write time. The alternative — writing the
derivation straight into the enrichment blob — does not survive contact with
`SetJobEnrichment`, which assigns `enrichment = <payload>::jsonb || …overlays`,
replacing the blob wholesale. A derived list written into the blob would be erased
by the next enrichment run.

The overlay is not a new mechanism: `SetJobEnrichment` already chains two of them,
for the ATS's structured salary and a moderator's manual salary, precisely so a
non-LLM source can win over the model payload. This is a third link in that chain,
with the opposite precedence (fill-if-absent rather than win-if-present) expressed
by its own guard.

`UpsertJob`'s conflict branch carries the same shape for the moderator re-create
path, where it already overlays the manual salary onto the existing enrichment.

### LLM wins, derivation fills

77% of postings state their requirements as prose with no list markup — the model
reaches those and the parser cannot. Where the model has run, its reading stands.
Where it has not, the parser's does. Precedence is expressed as a guard on the
incoming payload's own `requirements` being empty, so it is decided by the data,
not by a version number or a flag.

### Dictionary-gated extraction, with no fallback

The parser matches a heading against a controlled vocabulary and takes the list
that follows. If no heading matches, it yields nothing. There is deliberately no
"take the longest list in the document" fallback: the most common list in a job
posting after the requirements is the benefits list, and reading perks as
requirements is worse than reading nothing. This is the same dict-only posture the
facet dictionaries take.

Matching the vocabulary as a bare prefix is not strict enough, and a run over 164
live postings proved it: `MUST HAVE MORNING/DAYTIME AVAILABILITY` begins with
`must have` and heads a list of **employee benefits**, and `Preferred Hours:` begins
with `preferred` and heads a scheduling note. Both swept their lists in. So a
vocabulary phrase opens a section only when the rest of the heading is itself
vocabulary — connectives and the nouns a posting uses to name the same section twice
(`Requirements & Qualifications`, `Preferred competencies and qualifications`). This
trades recall for precision deliberately: `Required Equipment & Licenses` is now
missed, and a missed section is a blank space, while a benefits list under a
"Requirements" heading is a false claim the reader cannot detect.

The bounds come from `enrich`'s own `maxRequirements` / `maxRequirementTextRunes`
constants rather than being restated, so one ceiling governs both producers.

### A dedicated backfill, not `cmd/backfill-derive`

*Alternative considered:* add the column to `UpdateJobDerived` and let
`cmd/backfill-derive` fill it.

Rejected on three counts: that pass walks all ~11M rows where only the 4.6M open
ones matter; it rewrites every derived column when one is wanted; and it takes
~15h, so this work would have to wait on its schedule. `cmd/backfill-clearance`
set the precedent for a narrow one-off, and this follows its shape — keyset over
`id`, chunked, `IS DISTINCT FROM`-guarded so a re-run writes nothing and stopping
part-way is free.

### Nothing here touches Meilisearch or the enrichment version

The job detail page is served by `jobview.FromRow` off a Postgres row, and the
field is never a filter, so the `is_tech` / `requires_clearance` trap — a new
column staying invisible until a full rebuild, because `content_hash` did not move
— does not apply. No reindex.

The enrichment version stays at 2. Bumping it would re-queue 1.3M rows through the
model to deliver a change the model had no part in.

## Risks / Trade-offs

- **The parser reads a benefits list as requirements** → the heading vocabulary is
  a closed list and there is no fallback; a heading outside it yields nothing. The
  "benefits list is not read as requirements" scenario is a test, not a hope.
- **`UpsertJob` gains a per-row HTML parse on the ingest hot path** → the parse is
  a single pass over a string already in memory and already being scanned by the
  skill and location dictionaries in `jobderive`. If it measures badly on a large
  crawl, it moves behind the same structure the other derivations use rather than
  changing the storage decision.
- **The backfill de-TOASTs a description per row over 4.6M rows** → the same load
  shape that made `BACKFILL_CONCURRENCY=6` degrade prod. Concurrency stays at 2–3,
  `BACKFILL_REQUIREMENTS_MAX` bounds a run, and the response time is watched with
  `curl` while it runs.
- **The section makes the duplication between a requirement entry and a skill chip
  visible** → accepted deliberately. The list is a quotation from the posting and
  is only honest whole.
- **A stale derived list after a posting's description changes** → `UpsertJob`
  re-derives on every write, the same as every other deterministic column, so a
  re-crawl that changes the body refreshes it.

## Migration Plan

1. Migration 0139 adds `jobs.requirements_derived jsonb NOT NULL DEFAULT '[]'::jsonb`.
   Additive, defaulted, no rewrite of existing rows.
2. Ship the extractor, the `UpsertJob` write, and the two overlays. From this point
   newly crawled and re-crawled postings carry a derived list, and the served field
   starts filling on its own.
3. Ship the web section. It renders whatever the field holds, so it is correct
   before, during and after the backfill.
4. Run `cmd/backfill-requirements` over the open catalogue, bounded per run,
   watching prod response time.

**Rollback:** the web section is one component change and reverts alone. The
overlays are fill-if-absent, so reverting them returns the served field to exactly
the model's own reading. The column can stay in place unused; dropping it is a
separate, later migration.

## Open Questions

None. The heading vocabulary's initial membership is an implementation detail
settled by the extractor's tests, and extending it later is additive.
