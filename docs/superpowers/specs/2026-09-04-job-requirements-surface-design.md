# Job requirements: surface them, then widen their coverage

**Date:** 2026-09-04
**Status:** design approved, not implemented

## The problem

`enrich.Enrichment.Requirements` — the posting's own stated requirements, extracted
by the enrichment LLM as `{text, priority}` pairs — has been collected since
2026-08-18, is sanitised, is served over the API, and is shipped into every job
page's hydration payload. **Nothing in `web/` renders it.** The bytes travel to the
browser and are discarded.

Measured on prod, 2026-09-04:

| Measure | Value |
| --- | --- |
| First row carrying requirements | 2026-08-18 (before that: exactly zero) |
| Coverage among rows enriched since then | 93% |
| Open jobs with a non-empty list | 133,404 (2.9% of 4.6M open) |
| Items per job | ~9.3 |
| `required` / `preferred` split | 78% / 22% |
| Item length | ~70 chars mean, 200 ceiling (clipped by `Sanitize`) |
| Distinct texts | 86% of items |

The 86%-distinct figure is the load-bearing one: this is a long tail, not a
vocabulary. It can never be a facet or a filter. It is display material, and
material. (An earlier draft added "and material for the candidate-side stack" —
`matchanalysis` builds its own list from a first LLM stage and `coverletter` takes
that one, so neither reads this field.)

A bucket pass over 74,623 items from the 8,000 most recently enriched rows:

| Bucket | Share | Already a facet? |
| --- | --- | --- |
| Freeform prose | 75% | no — this is the whole value |
| Short token (`python`, `docker`) | 7% | yes, `skills` |
| "N+ years of…" | 6% | yes, `experience_years_min` |
| Language / communication | 5% | partly, `english_level` |
| Education | 4.5% | yes, `education_level` |
| Certification, licence | 2% | no |
| Citizenship / visa | 1% | partly |

## Decisions taken

1. **Right column, not the sidebar.** The sidebar is 20rem and already dense (match,
   salary, facet `<dl>`, provenance, votes). Nine items averaging 70 characters is
   roughly fifteen wrapped lines — it would double the card. The section goes in the
   right column between the description and Skills, where there is width to read it.
2. **Render every item verbatim.** A quarter of the items restate something already
   on screen (a `Docker` bullet beside a `[Docker]` chip). Filtering them would mean
   a second normaliser living in `web/`, drifting from `internal/dict`. The list is a
   quotation from the posting and is only honest whole.
3. **Store the deterministic derivation in a column,** filled by a dedicated one-off
   backfill, rather than parsing on the request path: the backfill needs somewhere to
   put its answer, and re-parsing every body on every read repeats work whose input
   never changes.
4. **One served field, one reader.** The derived list is merged into
   `enrichment.requirements` at write time, not served as a second field.
5. **LLM wins; the derivation fills the gap.** The model reads the 77% of postings
   whose requirements are prose with no list markup. Where it has run, its reading
   stands; where it has not, the parser's does. Coverage adds rather than competes.

## Design

### 1. The rendered section

`web/src/lib/components/JobView.svelte`, inside the `descriptionContent` snippet,
between `<JobDescription>` and the Skills section:

```
─── What they ask for ───
Required
 • 5+ years of call center experience
 • Intermediate Microsoft Excel: formulas, v-Lookups, pivot tables
Preferred
 • Bachelor's degree in business management, math or statistics
```

- Reads `job.enrichment.requirements`; the type is already generated into
  `web/src/lib/generated/contracts.ts` as `Requirement[]`.
- Two groups in fixed order — `required`, then `preferred`. A group with no items
  renders no heading.
- An empty or absent list renders nothing at all, the way `e.summary` already does.
- The section takes `contentLang` like the description body: unlike `summary`, the
  requirement text is lifted from the posting and keeps the posting's language.

### 2. The deterministic extractor

A new package under `internal/job` (block 5, so it may return
`enrich.Requirement` directly — `internal/ai` is block 3).

Input: the job's `description` HTML. Output: `[]enrich.Requirement`.

The walk:

1. Find a heading node — `<h1>`–`<h6>`, or a `<strong>`/`<b>`/`<p>` that is the whole
   line — whose text matches the heading vocabulary.
2. Take the `<li>` items of the first `<ul>`/`<ol>` that follows it, until the next
   heading.
3. Priority comes from the heading itself: `nice to have`, `preferred`, `bonus`,
   `plus` → `preferred`; everything else in the vocabulary → `required`.
4. Repeat for every matching heading in the document, so a posting with both a
   "Requirements" and a "Nice to have" block yields both priorities.

The heading vocabulary is dict-only in the house sense: **no heading, no items.**
There is no fallback that guesses which list in a posting is the requirements list —
a benefits list must never be read as requirements.

Bounds mirror `enrich.Sanitize` exactly (`maxRequirements = 30`,
`maxRequirementTextRunes = 200`), so both producers obey one ceiling. Text is
extracted as plain text, whitespace-collapsed, entity-decoded.

Measured ceiling on 3,000 recent open postings: 29% carry a requirements-shaped
heading, 23% carry one followed by a list. Those were a regex sweep, and the shipped
extractor is stricter than a regex on purpose — a vocabulary phrase opens a section
only when the rest of the heading is itself vocabulary, which is what stops
`MUST HAVE MORNING/DAYTIME AVAILABILITY` (a real prod heading, over a list of
employee benefits) from opening one. Run over 164 live postings, the extractor yields
on **12.8%**. That is the number to hold: it lifts coverage from 2.9% to 12.8%
immediately, four-fold, and the two sources union.

### 3. Storage and the merge

**Migration 0139** adds `jobs.requirements_derived jsonb NOT NULL DEFAULT '[]'::jsonb`.

Writers:

- `UpsertJob` — ingest computes it from the description alongside the other
  `jobderive` facets, so every newly crawled posting carries it.
- `cmd/backfill-requirements` — a dedicated one-off for the existing catalogue,
  following `cmd/backfill-clearance`'s shape rather than folding into
  `cmd/backfill-derive`. Reasons: it walks **open** postings only (4.6M, not 11M),
  it writes one column instead of rewriting every derived column, and
  `backfill-derive` is a ~15h pass whose schedule this should not have to wait on.
  Keyset-paced over `id`, chunked, and idempotent — the chunk UPDATE is
  `IS DISTINCT FROM`-guarded, so a re-run writes nothing and stopping it mid-way is
  free. `BACKFILL_REQUIREMENTS_CHUNK` sizes the id span per statement and
  `BACKFILL_REQUIREMENTS_MAX` caps one run. Needs only `DATABASE_URL`.
  Concurrency stays at 2–3: `BACKFILL_CONCURRENCY=6` has degraded prod before, and
  this pass de-TOASTs a description per row, which is the same shape of load.

Reader — `SetJobEnrichment` gains a **third overlay**, chained after the two salary
overlays that are already there and following their shape exactly:

```sql
enrichment = <llm payload>::jsonb
    || <salary_source overlay>
    || <salary_manual overlay>
    || CASE
        WHEN jsonb_array_length(COALESCE(sqlc.arg(enrichment)::jsonb->'requirements', '[]'::jsonb)) = 0
             AND jsonb_array_length(requirements_derived) > 0
        THEN jsonb_build_object('requirements', requirements_derived)
        ELSE '{}'::jsonb
    END
```

`UpsertJob`'s existing `enrichment = CASE …` on the moderator re-create path gains
the same overlay, so a re-created posting does not lose its derived list.

The consequence that matters: `enrichment.requirements` stays the single field every
consumer reads, and a later `cmd/enrich` run cannot erase the derived list — it
either replaces it with a better one or the overlay puts it back.

### 4. What this does NOT need

**No reindex.** The job detail page is served by `jobview.FromRow` off a Postgres
row, not off the search document, and the field is never a filter. The
`is_tech` / `requires_clearance` trap — where a new column stays invisible until a
full rebuild because `content_hash` did not move — does not apply.

**No enrichment version bump.** The derivation is orthogonal to the LLM payload and
must not re-queue 1.3M rows through the model.

## Testing

Extractor, table-driven:

- A posting with `<h3>Requirements</h3><ul>…` yields the items, all `required`.
- A posting with both `Requirements` and `Nice to have` yields both priorities.
- A benefits-only posting (`<h3>What we offer</h3><ul>…`) yields nothing.
- A heading with no list after it yields nothing.
- A prose posting with no headings yields nothing.
- Over-long text is clipped at 200 runes; over-long lists at 30 items — the same
  numbers `enrich.Sanitize` enforces, asserted against the same constants.
- Entities and nested markup (`<li><strong>Go</strong> — 5 years</li>`) come out as
  clean plain text.

Merge, integration (`//go:build integration`, `internal/platform/db`):

- `SetJobEnrichment` with a payload carrying requirements leaves them alone.
- `SetJobEnrichment` with a payload carrying none picks up `requirements_derived`.
- `SetJobEnrichment` with neither leaves `requirements` absent.

Web: a component test that the section is absent on a job with no requirements, and
that both groups render in order when present.

## Open items deliberately not in scope

- Normalising near-duplicate phrasings ("excellent written and verbal communication
  skills" vs "strong written and verbal communication skills"). Real, but it is a
  clustering problem and this list is display-only.
- Non-English heading vocabulary. The catalogue is majority English; adding more
  languages is a later, additive change to one vocabulary.
- Feeding the derived list into `matchanalysis` / `coverletter`. Neither reads
  `enrichment.requirements`: the first builds its own list from an LLM stage and the
  second takes that. Wiring them to this column is a separate change.
