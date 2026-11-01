## Why

The `ats_absent` criterion of the ghost signal is stamped by `cmd/ghost-crosscheck`, which ran
once (2026-07-30) and is scheduled nowhere. A dry run on prod (2026-10-31) said it would stamp
87,120 of 1.8M aggregator postings, and the report showed it unfit to open: the largest
"company" was the empty company slug (10,559), staffing agencies filled the rest of the top
list, and a hand-checked sample found most "absent" roles were the same role retitled between
the board and the aggregator ("Account Executive - Fraud" against "Account Executive Fraud").

## What Changes

- Skip postings with no company, postings that say they are published on a client's behalf
  (`via_recruiter`), and every posting of a company with two or more such postings — each its
  own reason in the report.
- Match titles leniently: a punctuation-blind key with an optional trailing clause
  (`jobhash.TitleKeys`), or two thirds of the shorter title's stemmed words shared.
- Judge one company's postings from one source together: no stamp unless the board carries at
  least one of them and at most half are absent; no board of fewer than five titles counts as
  coverage. The worker holds a whole source before judging so the batch is never a fragment.
- A stamp is the latest run's verdict: anything a run does not re-stamp is cleared.
- The report breaks skips down by reason, lists the top sources beside the top companies, and
  samples stamped roles at random across the run.

## Impact

- `internal/job/ghost`, `internal/job/jobhash`, `cmd/ghost-crosscheck`,
  `internal/platform/db/queries/ghost_crosscheck.sql` (one new read, one new column read).
- No schema change, no write path outside `--apply`. The worker stays unscheduled.
