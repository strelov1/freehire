## Why

#2710 (freehire#2696) fixed `WorkModeFromDescription` so a bounded travel/PTO perk
phrase ("work from anywhere ... for up to a month") no longer fills
`work_mode=remote` on a hybrid or onsite posting. That fix only reaches postings
ingested from now on.

Neither existing propagation tool reaches the jobs already mis-tagged before the
fix:

- `cmd/backfill-derive` feeds a row's CURRENTLY STORED `work_mode` back into
  `jobderive.Derive` as the structured-signal input
  (`cmd/backfill-derive/main.go:173`, "preserves a set work_mode") — correct for
  its own purpose (never clobber a genuinely structured ATS signal), but it means
  a value the old, buggy detector wrote is indistinguishable from a real one and
  is carried forward unchanged. A full ~15h pass over the catalogue would fix
  nothing for this bug.
- Ordinary re-ingest does not reach them either: once a posting is stored with a
  description, ingest's seen-set only confirms liveness on later crawls
  (AGENTS.md, "BODY_REFRESH_DAYS ships UNSET"), so the description is never
  re-read and `jobderive` never re-runs against it for an open, still-live
  posting.

Measured against production (read-only Meilisearch queries, the same 143/61 figures
#2710's proposal.md cites): the affected population is real and will stay wrong
indefinitely without a dedicated pass.

## What Changes

- Add `cmd/backfill-remote-perk-false-positive`, a one-off pass: gathers candidate
  jobs from Meilisearch (`work_mode = "remote"` and the text query "work from
  anywhere" — the same over-fetch-and-let-the-dictionary-decide shape
  `cmd/backfill-clearance` uses, for the same reason), re-derives `work_mode` from
  each candidate's stored `location` and `description` using ONLY the
  location-marker and description-phrase steps of the precedence chain (no
  structured-signal input — the whole point is to stop trusting the currently
  stored value), and writes the recomputed value only when it differs from
  `"remote"`.
- Idempotent (`SetJobWorkMode` is `IS DISTINCT FROM`-guarded): a re-run writes
  nothing for a row already corrected.
- No reindex is bundled into the command; `work_mode` is not part of
  `content_hash`, so a corrected row's facet only reaches Meilisearch on a full
  `make reindex` — the same gap `cmd/backfill-clearance`'s own docs describe. Run
  one after this pass.

## Known limitation

A row whose current `work_mode=remote` came from a genuine, structured ATS signal
that also happens to be a Meilisearch hit for "work from anywhere" cannot be told
apart here from one the bug produced — the structured signal is folded into the one
`work_mode` column at write time and is not recoverable from stored fields alone.
Such a row is a false correction in principle, but not for long: the next ordinary
crawl of that posting passes ingest the adapter's LIVE structured signal directly
(`internal/ingest/pipeline.normalizeJob`), overwriting whatever this pass wrote. A
genuinely wrongly-tagged row (the actual bug) has no such backstop — nothing else
ever revisits it — which is why erring toward correction here is the right side to
be wrong on.
