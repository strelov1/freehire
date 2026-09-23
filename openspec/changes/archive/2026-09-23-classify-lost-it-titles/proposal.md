# Teach the tech-title detector the forms the catalogue actually carries

## Why

2,232,773 open, canonical, public postings carry `is_tech IS NULL` — neither the
title dictionary nor the non-tech dictionary recognised them. That is correct for
almost all of them: a random sample of 100 is nurses, seamstresses, chambermaids,
German retail apprenticeships, electricians and dairy operators. The catalogue is
right to hide them.

It is wrong about **71,314 of them (3.2%)**, whose titles carry an outright software
or IT signal. Those postings are absent three times over: `search.CategoryUnresolved`
keeps them out of the search index, `jobSitemapFilter` keeps them out of every
sitemap, and `EnqueuePendingJobs`' `is_tech IS TRUE` gate never queues them for
enrichment — so nothing downstream can rescue them either. Against ~691k technical
postings in the index, this is on the order of a tenth of the catalogue that no
visitor, no crawler and no model can reach.

The gaps are specific and measurable, not a general weakness. Taken from the 160
most common lost titles on prod (2026-09-23):

| gap | postings | titles | example |
|---|---|---|---|
| platform/technology + Developer | 1,289 | 49 | `Mulesoft Developer`, `Flutter Developer`, `SQL Developer` |
| IT + role | 987 | 30 | `IT Officer`, `IT Trainer`, `Head of IT` |
| level + Developer | 336 | 5 | `Senior Developer`, `Lead Developer` |
| the `Engr` abbreviation | 194 | 7 | `Software Engr II`, `Advanced Software Engr` |
| plural | 58 | 3 | `Software Engineers`, `Data Engineers` |

Those counts cover only titles appearing 12+ times — 4,366 of the 71,314. A term is
matched as a whole-word PHRASE anywhere inside a longer title — never as an arbitrary
substring — so each one also collects its long tail (`flutter developer` takes
`Senior Flutter Developer` and `Flutter Developer (Remote)` with it, but nothing where
those two words are not adjacent), and the real yield is measured after the backfill
rather than predicted here.

## What Changes

- Add curated terms to `classify.techTitleTerms` in five groups: vendor platforms,
  `IT`-anchored roles, level-qualified `Developer`, the `Engr` abbreviation anchored
  to `software`, and the plural forms of already-listed terms.
- Add the negative tests the corpus demands: `Business Developer`, `Job Developer`
  (a placement counsellor, not a programmer), `Project Developer` (real estate),
  `Project Engr II` and `Field Service Engr II` all stay unrecognised.
- No change to the matcher, the derivation, or any dictionary structure.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `tech-classification`: "Deterministic tech title detection" currently justifies its
  vocabulary by naming the ambiguous terms it excludes (bare `engineer`, bare
  `analyst`). It does not say what makes a term ADMISSIBLE, which is why three whole
  families of unambiguous titles were never in it. The requirement gains the positive
  rule — a term is admissible when it is anchored to the craft, and an anchor may be a
  technology, a platform, the word `software`, or a seniority word that makes the
  phrase unambiguous — plus the corpus rule: what is missing is discovered from
  production titles, not from memory.

## Impact

- `internal/dict/classify/tech.go` — terms only.
- `internal/dict/classify/tech_test.go` — positive and negative cases.
- No migration, no schema change, no new package, no API change.
- **Reaches new postings at ingest; reaches the 71k stored ones only through
  `cmd/backfill-derive`**, which re-derives `is_tech` from stored columns. That pass
  is ~15h over the whole catalogue and `BACKFILL_CONCURRENCY=6` has degraded prod
  before, so it is a separate, paced operation — not part of this deploy.
- **A full `make reindex` is required after that backfill**: `is_tech` is not part of
  `content_hash`, so an incremental push never carries it. The same trap
  `backfill-clearance` and `backfill-profession-it-tech` both document.
- Rollback is removing the terms: the detector returns to today's answers, and the
  next backfill re-derives `is_tech` back to NULL.
