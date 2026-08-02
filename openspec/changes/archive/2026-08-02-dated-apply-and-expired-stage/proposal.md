## Why

An application history that already happened cannot be recorded truthfully. `POST /apply`
always stamps `now()`, so importing a month of past applications files all of them under
today: the silence ladder sees nothing overdue, the follow-up offer never appears, and the
per-company response rate measures the import rather than the employers. Correcting this
today means hand-written SQL against production — one `UPDATE` for the column and a second
for the ledger, because a caller who forgets the second gets a card dated July and a
statistic dated August.

The stage vocabulary has the matching gap. Its three settled outcomes are `accepted`,
`rejected` and `withdrawn` — the employer decided, or the candidate did. The most common
ending has neither: nobody ever answered, or the posting went away. Those applications stay
in the `applied` column forever, so the board fills with cards no one will ever move.

## What Changes

- `POST /jobs/{slug}/apply` accepts an optional body naming the day the application was sent.
  Without a body the endpoint behaves exactly as it does today.
- A date the person states wins over one we inferred — both when the application is new and
  when it is already recorded. The existing "a later recording is not a later application"
  rule stays where it belongs: the mail path, where the date is read off an employer's
  message rather than asserted by the candidate.
- Correcting the date moves the `applied` ledger event with it, in the same transaction. The
  event says when the person applied, not when we wrote it down, and every aggregate reads
  the ledger.
- A new terminal stage `expired` joins the `closed` group, labelled "Expired". It is set by
  hand only — no worker closes anything, and no inference from mail can move an application
  into or out of it.
- `freehire apply <slug> --on <YYYY-MM-DD>` in the CLI (separate repository). `freehire stage
  <slug> expired` needs no CLI change: it validates against the same vocabulary.

Not changing: `applied_count` (re-dating an application is not a second application), bulk
operations (the tracker has none by design — every write is one upsert per user-and-job), and
the SPA (the board, the funnel and the stage selector all read the generated vocabulary).

## Capabilities

### New Capabilities

None. Both parts extend behaviour that already has an owner.

### Modified Capabilities

- `user-job-tracking`: marking a job applied gains an optional stated date, with the bounds
  that date must satisfy and the rule that it overrides a date already recorded.
- `tracking-stage-vocabulary`: the `closed` group gains `expired`, and the vocabulary gains a
  settled outcome meaning nobody answered.
- `application-event-ledger`: correcting an application's date corrects its `applied` event,
  so the two records of one transition cannot disagree.

## Impact

**Backend.** `internal/db/queries/user_jobs.sql` gains a re-dating statement; `MarkJobApplied`
itself is left alone, because it is the one place deciding the `applied_count` transition and
the ledger insert together. `internal/jobtracking` gains the service method that runs create
and re-date in one transaction. `internal/handler/user_jobs.go` parses and bounds the body.
`internal/userjob` gains `expired` across its four tables — vocabulary, terminal set, group,
label — with no silence threshold, since settled applications accrue none.

**Frontend.** Regenerated contracts only. The board's columns, the funnel's bands and the
stage selector already derive from `STAGE_GROUPS` / `STAGE_LABELS`; a type check in the
required `pnpm run check` gate fails if the new stage is left out of a group.

**Docs and CLI.** `docs/API.md` gains the apply body and the widened stage list. The CLI's
`Apply` client call and `apply` command take the date; its tests cover the parse.

**Not affected.** No migration: `applications.applied_at` and `application_events.occurred_at`
already exist and already hold what this change writes.
