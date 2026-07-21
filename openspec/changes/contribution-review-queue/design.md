## Context

`link_contributions` records crowdsourced company boards, keyed on `UNIQUE (source, board)`
with `source`/`board` `NOT NULL` and status `pending | onboarded | rejected`. `Submit`
resolves a pasted URL to `(source, board)`; on failure it calls `logUnrecognized` and returns
`ErrUnsupportedATS` (handler → 422), storing nothing. The handler awards an AI credit after any
successful `Record`. We want unrecognized-but-plausible links captured for manual triage
without polluting the credit ledger.

## Goals / Non-Goals

**Goals:**
- Persist every well-formed unrecognized link as a `review` row instead of dropping it.
- Keep credit exclusive to recognized novel boards; a review row credits nothing at submit.
- Surface review rows in the contribute UI with an explicit "not credited yet" state.
- Dedup the review queue by URL.

**Non-Goals:**
- No automated adapter generation or ingestion of review links.
- No new binary/worker/CLI. Manual promotion + credit top-up is a human step, documented in
  the `onboard-contributions` skill.
- No change to the recognized-board or already-tracked paths.

## Decisions

**One table, nullable source/board (per user).** Reuse `link_contributions` rather than a
separate `link_submissions` table. `source`/`board` drop `NOT NULL`; a `review` row leaves them
NULL. Alternative (separate table) was cleaner for the invariant but the user chose one table to
keep everything in one place and reuse `ListByUser`/reward-by-id.

**Dedup review rows by URL via a partial unique index.** `UNIQUE (source, board)` can't dedup
NULL/NULL rows (Postgres treats NULLs as distinct). Add
`CREATE UNIQUE INDEX ... ON link_contributions (url) WHERE source IS NULL`. Recognized rows keep
their `(source, board)` uniqueness. `Submit` checks for an existing review row by URL first and
returns `ErrBoardAlreadyContributed` (409) on a hit, so the collision surfaces as a clean error
rather than a raw constraint violation.

**Credit gate moves to the recognized path.** The handler currently rewards after any `Record`.
Change it to reward only when the recorded row is a recognized board. `Submit` already returns
the recorded `Contribution`; the handler gates on `rec.Status == "pending"` (recognized) vs
`"review"`. Alternative — a separate `awardable bool` return — is redundant with the status.

**Garbage still 422.** The existing `logUnrecognized` URL guard (valid `http(s)`, non-empty
host) becomes the gate: valid URL → review row; otherwise → `ErrUnsupportedATS` (422). This
keeps non-URL noise out of the queue.

## Risks / Trade-offs

- [Review queue fills with single-tenant / junk links] → Acceptable: no credit is spent on
  them, dedup caps repeats, and manual triage rejects them. Keeps the recognizer simple (no
  "known-but-unsupported host" distinction).
- [Table invariant weakened — some rows have no board] → Contained: `review` is the only such
  status; existing board queries filter by `source`/`board` and are unaffected by NULL rows.
- [Manual credit top-up could double-credit] → The existing reward is idempotent by contribution
  id; a manual award uses the same key, so a retry is safe.

## Migration Plan

- New migration file alters `link_contributions`: drop `NOT NULL` on `source`/`board`, replace
  the status CHECK to include `review`, add the partial unique index on `url`.
- Prod: apply by hand under `SET ROLE hire` (repo convention; migrations are initdb-only on
  fresh volumes). No backfill — existing rows already satisfy the looser constraints.
- Rollback: revert the code; the added `review` status and NULL columns are backward-compatible,
  so the schema can stay.

## Open Questions

None — scope and storage model settled with the user.
