# internal/industrytag — curated industry vocabulary for companies

Date: 2026-08-15
Status: design approved, not implemented

## Problem

`companies.industries` is a text array that nothing filters on. It is absent from
the Meilisearch company index's `FilterableAttributes` and from `companyFacets`, so
it reaches users only as a field in the API response. The user-facing "Industry"
filter runs on `subindustry` (a YC-only scalar) and `domains`.

The column also holds at least two vocabularies at once. A 400-company sample of
production carries `AI` and `Artificial Intelligence` as distinct values, `FinTech`
against `Fintech`, and whole foreign taxonomy strings from a one-off dump —
`Tech, Software & IT Services`, `Engineering, Product and Design`. Of 235 distinct
values in that sample, 159 belong to no dictionary we control.

Meanwhile an external company-directory dump (`scratch/company-dump/companies.jsonl`,
29 983 companies) carries market tags for 29 904 of them — 55 013 distinct raw
tags, of which the top 500 already cover 95.9% of companies.

## Why not just widen `domains`

`domains` is a deliberately coarse 20-value vertical vocabulary in `internal/vocab`
(`fintech`, `healthcare`, `devtools`, `climatetech`, …). Its design notes record
that synonyms were folded on purpose (`web3`→`crypto`, `biotech`→`healthcare`) and
that `saas` was dropped for being a business model rather than a vertical.

Two facts rule it out as the home for company industries:

1. **It is a fact about a job, not a company.** `RefreshCompanyFacets` recomputes
   `companies.domains` from `jobs.enrichment` on every `cmd/recount-companies` run.
   Anything written there from company-level data is silently overwritten.
2. **Widening it means re-enriching.** The values come from an LLM pass over job
   descriptions. New vocabulary reaches existing rows only by re-running enrichment
   over ~100k jobs.

The measured gap argues for a second level rather than a wider first one: in the
400-company sample, 283 had domains and **`other` was the largest bucket at 120** —
42% of tagged companies fall outside all 19 meaningful verticals. The external dump
labels those same companies `Medical Devices`, `Renewable Energy`, `Wealth Management`.

So `domains` stays the coarse vertical; `industries` becomes the finer level under
it. `industries` is one of the few company columns no automated pass owns, which
makes it safe to write.

## Design

### The package

`internal/industrytag`, shaped after `internal/skilltag`:

| File | Contents |
|---|---|
| `industrytag.go` | `Canonicalize([]string) []string` — raw tags in, canonical slugs out |
| `dictionaries.go` | the alias→canonical map |
| `labels.go` | `Label(canonical) string`, `Labels()`, `Canonicals()` |

**Canonical form is a lowercase slug** — `medical-devices`, `renewable-energy`,
`wealth-management` — matching `domains` (`fintech`) and `skills` (`typescript`).
Display goes through `Label()`. The stored value ends up in a filter URL, where a
slug survives case, spacing and encoding and `Manufacturing and Robotics` does not.

**Dict-only.** An unrecognized tag emits nothing. No guessing, no mechanical
Title-Casing of unknown input — the same rule every other facet dictionary follows.

**The canonical vocabulary is written out, not cut from a frequency ranking.**
The first attempt took the top 100 labels by frequency. It failed twice in review:
whatever N is, the tail of the ranking is simply whatever was marginally frequent,
so the list arrived carrying duplicates (`aerospace` beside `aerospace-and-defense`)
and non-industries (`saas`, `digital-transformation`, `mobile-application-developer`).
Removing a batch of junk only promoted the next batch — `Hvac`, `R and D`,
`Lead Generation` — because the cut refilled from below.

The vocabulary is therefore an explicit list of 74 values, each a thing a company
can BE rather than a function it performs, a technology it uses, or a business model
it sells under. The observed data still drives the ALIASES, which is where volume
belongs. 74 values reach 92.4% of companies in the dump; the rejected 100-value
frequency cut reached 93.2%, so the curation costs 0.8% of coverage.

### Writers

Both go through the dictionary:

- `cmd/import-yc` — canonicalizes `rec.Industries` before the upsert.
- `cmd/import-company-industries` (new) — a run-once worker taking a JSONL path
  (`{slug, name, markets}`) plus `DATABASE_URL`. The dump stays out of
  git; only code and dictionary are committed.

The new worker's first pass **normalizes the whole existing column** through the
dictionary. Values outside the dictionary are dropped. This is destructive and
intentional: the column becomes dict-only, like `jobs.skills`. The worker dumps
the pre-change column to a file first, so a bad result is one UPDATE away from
being restored.

### Matching companies

The dump's slugs are built from the company's **domain** (`circle.com` →
`circle-com`); ours from `normalize.Slug(name)`. Neither key alone is enough: on a
291k-company sample taken from the public sitemap, their slug matched 15 108 rows,
a slug rebuilt from their name matched 15 167, and the union matched 15 948.

Run against the full table on 2026-08-15, the two keys together produced 30 280
distinct lookup values and **matched 18 611 of our 381 907 companies** — more than
the sitemap sample predicted, because the sitemap lists only companies that have a
page. Where two dump rows collide on one key, the one with more live jobs wins.

### Fixing the YC upsert

`UpsertYCCompany` currently writes `tagline`, `company_info` and `industries` by
replacement, so the next `cmd/import-yc` run would erase everything written by any
other source. It changes to:

```sql
tagline      = COALESCE(NULLIF(companies.tagline, ''), EXCLUDED.tagline)
company_info = EXCLUDED.company_info || companies.company_info
industries   = <union, de-duplicated, sorted>
```

Operand order in `||` is load-bearing: `a || b` keeps **b** on key collision, so
`EXCLUDED.company_info || companies.company_info` lets new keys land while existing
values win. Reversed, it restores the behaviour being removed.

### Wiring the facet

1. `internal/search/company.go` — `industries` into `FilterableAttributes` and
   `companyFacets`.
2. `/companies` handler — an `industries` query parameter.
3. `web/src/lib/facets.ts` — a second level beneath the existing industry filter.
4. `cmd/reindex-companies` afterwards, or Meili keeps serving the old values.

## Testing

Dictionary invariants, mirroring `skilltag`'s:

- every canonical is a well-formed slug;
- no alias points at a canonical that does not exist;
- every canonical has a `Label()`;
- an unknown tag yields nothing;
- no alias maps to two different canonicals.

`UpsertYCCompany` gets an integration test (build-tagged) asserting that an
existing tagline survives the import, that `company_info` merges, and that
industries union. That test is the regression guard for the bug being fixed.

The worker gets table-driven tests on the mapping and an integration test on the
normalization pass.

## Failure handling

The worker follows the `worker.Bootstrap` convention: non-zero exit on failure,
keyset-paged batches, one transaction per batch, so an abort leaves a partially
migrated column rather than an emptied one.

**Unknown tags are counted and reported** at the end — how many distinct tags the
dictionary missed, and the most frequent among them. Without that, dict-only turns
into silent data loss; with it, the dictionary has a growth path, the same one
`mine-skill-dictionary` gives skills.

## Sequencing

1. `industrytag` package, dictionary, tests
2. `UpsertYCCompany` → merge semantics
3. Import worker (normalization + the external dump)
4. Facet in Meilisearch and the UI
5. `cmd/reindex-companies`

Steps 1–3 can merge independently of 4: the data becomes clean immediately and the
filter follows.

## Out of scope

- The 14 035 dump companies absent from our catalogue. Importing them means
  company pages with no jobs, which is a product question, not a dictionary one.
- `company_info.website` and `tagline` enrichment from the same dump — already
  applied to production on 2026-08-15 (18 612 rows) via `scratch/company-dump/`.
- The stale `saas` value living in `companies.domains` for 64 sampled companies
  after being dropped from `vocab.DomainValues`. Real, but a different cleanup.
