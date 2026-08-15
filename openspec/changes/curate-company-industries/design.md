## Context

`companies.industries` was written by two sources that never agreed on spelling. A
400-company production sample carries `AI` and `Artificial Intelligence` as
distinct values, `FinTech` against `Fintech`, and whole foreign taxonomy strings —
`Tech, Software & IT Services`, `Engineering, Product and Design`. Of 235 distinct
values in that sample, 159 belong to no dictionary we control. Coverage is 41.2%.

Nothing filters on the column. It is absent from the Meilisearch company index's
`FilterableAttributes` and from `companyFacets`; the user-facing "Industry" filter
runs on `subindustry` (a YC-only scalar, 0% coverage in that sample) and `domains`.

An external company-directory dump provides market tags for 29 904 companies —
55 013 distinct raw tags, of which the top 500 cover 95.9% of companies and the
top 100 cover 86.5%. Matched against production it reaches 18 611 of our 381 907
companies.

## Goals / Non-Goals

**Goals:**

- One controlled vocabulary for `companies.industries`, owned by us.
- A filterable industry facet finer than `domains`.
- Writers that cooperate instead of overwriting each other.
- A dictionary that grows from evidence rather than guesswork.

**Non-Goals:**

- Importing the 14 035 dump companies absent from our catalogue. That means
  company pages with no jobs — a product question, not a dictionary one.
- Widening `vocab.DomainValues`. See the decision below.
- Cleaning the stale `saas` value still stored in `companies.domains` after being
  dropped from the vocabulary. Real, but a separate cleanup.

## Decisions

**A second, finer facet rather than a wider `domains`.** `domains` is a
deliberately coarse 20-value vertical vocabulary whose design folded synonyms on
purpose (`web3`→`crypto`, `biotech`→`healthcare`) and dropped `saas` as a business
model rather than a vertical. Two facts rule it out as the home for company
industries. It is a fact about a *job*: `RefreshCompanyFacets` recomputes
`companies.domains` from `jobs.enrichment` on every `cmd/recount-companies` run, so
company-level writes there are silently erased. And widening it would require
re-running enrichment over ~100k jobs for the new vocabulary to reach existing
rows. The measured gap argues for depth, not width: of 283 sampled companies with
domains, `other` was the largest bucket at 120 — 42% fall outside all 19 meaningful
verticals. `industries` is one of the few company columns no automated pass owns,
which is what makes it safe to write.

**Canonical values are lowercase slugs.** `medical-devices`, not
`Medical Devices` — matching `domains` (`fintech`) and `skills` (`typescript`).
The stored value lands in a filter URL, where a slug survives case, spacing and
encoding. Display text comes from a separate `Label()` lookup. The alternative —
storing display text, as the column does today — is what produced `FinTech` beside
`Fintech` in the first place.

**Dict-only, with a report.** An unrecognized label emits nothing; no mechanical
Title-Casing of unknown input, which would only add a third spelling. The bulk
passes report what they dropped — distinct count, occurrences, most frequent — so
the vocabulary grows from production evidence. Without the report, dict-only and
silent data loss look identical from the outside.

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

**Merge semantics in the YC upsert.** `UpsertYCCompany` writes `tagline`,
`company_info` and `industries` by replacement, which was correct while it was the
only writer and is wrong now. Operand order in the JSONB merge is load-bearing:
`a || b` keeps **b** on key collision, so `EXCLUDED.company_info ||
companies.company_info` lets new keys land while stored values win. Reversed, it
restores exactly the behaviour being removed.

**Matching on two keys.** Dump slugs are domain-derived (`circle.com` →
`circle-com`); ours come from `normalize.Slug(name)`. Measured on a 291k-company
sitemap sample, their slug matched 15 108 rows and a name-derived slug matched
15 167, but the union matched 15 948 — neither key alone is sufficient.

## Risks / Trade-offs

- **The normalization pass is destructive.** Stored values outside the dictionary
  are dropped, and a too-small dictionary drops values that mattered. → Back up
  the column before the run; read the dropped-value report before believing the
  result; restore with one UPDATE if it looks wrong.
- **Two industry facets may confuse users.** `domains` (coarse) and `industries`
  (fine) both read as "industry". → Label them distinctly in the UI; the fine one
  is presented as a refinement, not an alternative.
- **A new filterable Meilisearch attribute 500s until the index is rebuilt.** →
  Reindex immediately after deploy, and only when no other reindex is in flight —
  Meilisearch serves a single task queue, so a second rebuild queues behind the
  first and presents as a hang.
- **The dictionary is a hand-owned artifact that will drift from reality.** → The
  dropped-value report is the intended feedback loop; expect the first production
  run to name a few hundred unknown values and to be followed by a dictionary edit.

## Migration Plan

1. Ship the dictionary, the merge-semantics fix and the worker (independent of the
   facet).
2. Back up `companies.industries`, run the worker's normalization pass, read the
   dropped-value report, extend the dictionary, re-run.
3. Merge the external dump.
4. Ship the facet wiring and reindex companies.

Rollback: the column backup restores the pre-change values in one UPDATE. The
merge-semantics change is backward compatible — it only ever writes less.

## Open Questions

None blocking. The dictionary's exact 100 entries are expected to change during
implementation as the invariant tests and the first dropped-value report land.
