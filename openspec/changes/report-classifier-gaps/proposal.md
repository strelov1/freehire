## Why

LLM enrichment already writes two signals nothing else reads: `jobs.enrichment.skills`, an
open-vocabulary list the model free-writes per posting, and `jobs.enrichment.seniority`/
`category`, the model's own guess at the same facets our deterministic title dictionary
(`internal/dict/classify`) derives independently. Millions of enriched postings sit on this
data, but nobody has ever compared it against what `internal/dict/skilltag` and
`internal/dict/classify` actually resolve — so a skill the model names constantly and our
alias table has no entry for, or a title shape our seniority/category matcher keeps getting
wrong relative to the model, is invisible until a curator happens to notice a specific job.

## What Changes

- Add a new pure package, `internal/job/dictgap`, that turns raw enrichment facts into
  ranked dictionary-gap candidates:
  - `SkillGapCandidates`: given raw `enrichment.skills` phrases with their frequency, keeps
    the ones `skilltag.Parse` resolves to nothing, collapsing trivial case/punctuation/
    whitespace variants of the same phrase before ranking.
  - `ClassifyDriftCandidates`: given `(title, jobs.seniority, jobs.category,
    enrichment.seniority, enrichment.category)` rows, recomputes `classify.Parse(title)` and
    ranks titles where the dictionary's answer disagrees with what the model stored, tracked
    separately for seniority and for category.
- Add `cmd/report-skill-gaps`: a hand-run, read-only report over `jobs` (chunked by id range,
  the same shape `cmd/backfill-derive` uses) that feeds `dictgap.SkillGapCandidates` and
  prints the top-N uncovered skill phrases by frequency.
- Add `cmd/report-classify-drift`: the same chunked read, feeding
  `dictgap.ClassifyDriftCandidates`, printing the top-N titles where the dictionary and the
  model disagree.
- Add the sqlc queries these two commands need to page through
  `jobs.enrichment`/`title`/`seniority`/`category` by id range.

Both commands are report-only: neither writes to the database, neither has an `--apply`
flag, and neither is wired to a systemd timer. They exist to hand a curator a ranked list;
deciding what enters `dictionaries.go`/`labels.go`/`descriptions.tsv` or the `classify`
title table stays a manual, reviewed edit, exactly as every other curated dictionary change
in this repo already works.

## Capabilities

### New Capabilities
- `dict-gap-reporting`: read-only reporting that mines LLM enrichment output to rank
  candidate gaps in the deterministic skill-tagging and title-classification dictionaries.

### Modified Capabilities
(none — `skilltag`'s and `classify`'s own matching behavior is unchanged; this change only
adds a consumer that reads their existing output)

## Impact

- New code: `internal/job/dictgap` (job block, layer 5; imports `internal/dict/skilltag` and
  `internal/dict/classify`, both layer 2 — no layering violation), `cmd/report-skill-gaps`,
  `cmd/report-classify-drift`.
- New sqlc queries in `internal/platform/db/queries/*.sql` (regenerated via `make sqlc`).
- No schema migration, no change to `cmd/enrich`, `cmd/ingest`, or any serving path.
- No new operational surface: nothing scheduled, nothing that needs `MEILI_*` or `LLM_*` env,
  only `DATABASE_URL`.
