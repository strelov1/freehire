## Context

`classify.IsTech` is a curated list of anchored phrases matched whole-word
(`internal/dict/wordmatch`). `jobderive.deriveIsTech` calls it: a technical title
wins outright, otherwise a technical category wins unless the non-tech dictionary
vetoes, otherwise the signal is `nil`. Nothing coerces `nil`, so the unrecognised
mass stays measurable — which is what made this measurement possible at all.

`is_tech = true` is worth more than a facet. `search.CategoryUnresolved`
(`document.go:179`) returns false the moment `is_tech` is true, **even with no
category**, so the signal alone puts a posting in the search index; `jobSitemapFilter`
requires it; and `EnqueuePendingJobs` gates enrichment on it, so a posting that gains
the signal also gains the LLM pass that will later give it a category. One dictionary
entry therefore opens all three doors, and no LLM spend is required to open them.

## Goals / Non-Goals

**Goals:**

- Recognise the title forms the catalogue demonstrably carries and the detector
  demonstrably misses.
- Keep every negative the corpus supplies: the lookalikes are real postings, not
  hypotheticals.

**Non-Goals:**

- Changing the matcher, the derivation, or the dictionary's structure.
- Normalising surface forms (stemming plurals, expanding abbreviations). The
  dictionary already answers this — `software design engineer` and `software design
  engineering` are separate entries, with a comment explaining that word boundaries
  cannot see past themselves. A normaliser would be a second, competing answer.
- Resolving these titles to a CATEGORY. `is_tech` alone is enough for all three
  gates; category is enrichment's job and arrives on its own.
- Running the backfill. That is a paced operation on a saturated host, not part of
  this change.

## Decisions

**Terms, not rules.** A generator (`{senior,lead,junior} × {developer,engineer}`)
would cover more combinations, but the dictionary's whole value is that every entry
was looked at. `Job Developer` and `Project Developer` are in the same corpus as
`Senior Developer`; a rule cannot tell them apart, a curator can. The repo's own
practice agrees — every existing entry is spelled out.

**The anchor rule is written into the spec, not just applied.** The requirement
previously listed what was excluded and left "what makes a term admissible" implicit.
That omission is the whole defect: three families of unambiguous titles were never
considered because nobody had stated the rule they satisfy.

**`senior developer` is admissible and `developer` is not.** The match is a phrase
with word boundaries, so `senior developer` cannot occur inside `Senior Business
Developer` — the words are not adjacent there. Verified against prod: of the twenty
most common open titles containing the phrase, all twenty are software roles.

**`software engr`, never bare `engr`.** The same corpus carries `Project Engr II`
(28) and `Field Service Engr II` (18). The abbreviation is not the anchor; `software`
is.

**`it <role>`, never bare `it`.** Lowercased for matching, `it` is the English
pronoun. Every IT term is the two-word form.

## Risks / Trade-offs

- **A curated list goes stale the moment the corpus moves** → it already has, which
  is why the spec now says gaps are discovered from production titles. `cmd/report-classify-drift`
  exists for this and is the documented way to re-run the measurement.
- **A false positive puts a non-technical posting in front of a crawler** → the
  negative cases from the corpus are tests, and the anchor rule refuses every bare
  term. The cost of a miss is asymmetric and understood: a false positive dilutes the
  catalogue, a false negative hides a real job entirely — which is the state 71,314
  postings are in today.
- **The yield is predicted, not measured** → deliberately. A term matches as a
  whole-word phrase anywhere inside a longer title — never as an arbitrary substring —
  so it collects a long tail no title-frequency query can size in advance. The honest
  number comes from counting `is_tech IS NULL` before and after the backfill.

## Migration Plan

1. Merge and deploy. New postings get the signal at ingest immediately.
2. Separately, and paced: `cmd/backfill-derive` to re-derive `is_tech` for stored
   rows. `BACKFILL_CONCURRENCY` stays at 2-3 — 6 has degraded prod before — and the
   pass is resumable via `BACKFILL_DERIVE_FROM_ID`, which it prints on every exit.
3. A full `make reindex` after that: `is_tech` is not in `content_hash`, so no
   incremental push carries it. Skipping this leaves the facet empty for every
   pre-existing posting — the trap `backfill-clearance` documents.
4. Re-count `is_tech IS NULL` to measure the real yield.

Rollback is removing the terms. The next backfill re-derives `is_tech` to NULL; no
data is destroyed at any point, since the derivation is a pure function of stored
columns.

## Open Questions

None.
