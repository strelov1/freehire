# Report the titles the dictionaries recognise as nothing at all

## Why

`tech-classification` now requires that the dictionary's gaps be discovered from
production titles rather than from memory. Nothing implements that. The two reports
that exist mine what the LLM said — `report-skill-gaps` ranks skill phrases
`skilltag` cannot resolve, `report-classify-drift` ranks titles where `classify.Parse`
disagrees with enrichment — and both are blind to the population that matters most
here, because **a title the dictionaries recognise as nothing never reaches enrichment
in the first place**: `EnqueuePendingJobs` gates on `is_tech IS TRUE`. There is no LLM
opinion to disagree with, so drift cannot see it.

That blind spot was measured on 2026-09-23 with hand-written SQL and a throwaway test
probe: **2,232,773** open canonical postings carried no `is_tech` signal, **71,314** of
them on titles reading as software or IT outright. Acting on it returned ~110 terms to
`classify.techTitleTerms` and lifted corpus coverage from 49% to 82%. The method
worked; it just lives in a shell history and a test file that skips unless an
environment variable points it at a file somebody produced by hand.

The gap will reopen. The catalogue holds 1,563,954 distinct unrecognised titles and
gains sources continuously, so this is a standing measurement, not a one-off — and the
next person to run it should not have to re-derive the query, the chunking, or the
rule that the recompute must use TODAY's dictionary rather than the stored column.

## What Changes

- Add `dictgap.UnclassifiedTitles`: given distinct titles with their posting counts,
  recompute `classify.IsTech` and `classify.Parse` against the CURRENT dictionaries and
  return only the titles both still fail to place, ranked by posting count.
- Add the two SQL reads it needs, following `ClassifyDriftReportBounds` /
  `ListTitlesForClassifyDrift` exactly: an id-span read and a per-chunk
  distinct-title-with-count read over open, canonical, public postings.
- Add `cmd/report-unclassified-titles`: a hand-run, read-only report. No `--apply`, no
  timer, no write of any kind — a curator reads the ranked list and decides by hand
  what belongs in the dictionaries, the same contract `report-skill-gaps` and
  `report-classify-drift` already state.

## Capabilities

### New Capabilities

_None._ This is a third report inside an existing reporting capability.

### Modified Capabilities

- `dict-gap-reporting`: the capability currently covers only the gaps visible in LLM
  enrichment output. It gains the gap that is invisible there BY CONSTRUCTION — a
  title no dictionary places is never enriched, so it can never appear in a drift or
  skill report — plus the rule both existing reports already follow and neither states
  as a requirement: the answer is RECOMPUTED from today's dictionary, never read from
  the stored column, so a report run before a backfill still tells the truth about the
  dictionary.

## Impact

- `internal/job/dictgap/` — one file, one exported function, pure.
- `internal/platform/db/queries/jobs.sql` + `make sqlc` regeneration.
- `cmd/report-unclassified-titles/` — new binary, `DATABASE_URL` only.
- `internal/platform/arch/layering/blocks.go` — no entry needed; `dictgap` is already
  in the table and no new package is introduced.
- Writes nothing, so there is no rollback to design: deleting the binary is the whole
  of it.
