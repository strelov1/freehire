## Why

`application_events` (the ledger added by #1344) records what happened to an application and
denormalises `company_slug` at write time, expressly so that `cmd/prune` cannot orphan the fact.
That protects the denominator of the public company response rate. It does not protect the
numerator, because an application has no identity of its own: the ledger correlates a reply to
its application through `(user_id, job_id)`, and `application_events.job_id` is
`ON DELETE SET NULL`.

So when `cmd/prune` removes a posting:

- the `applied` event survives and still counts — its `company_slug` is its own;
- the `employer_reply` event survives too, but can no longer be found, because the correlation
  reduces to `NULL = NULL`, which is never true;
- an application the employer **answered** is served as unanswered.

The direction of that error is the one #1344 exists to remove. Its own header says the served
rate had been "a function of the candidate's inbox hygiene… someone clearing old mail made an
employer that had answered them look silent, on a public page, about a named company." A prune
run produces the identical distortion by a different route, and `cmd/prune` has already deleted
1,609,678 rows on production (`pruned_jobs`).

The same missing identity costs the candidate their record. `user_jobs` is keyed
`(user_id, job_id)` with `ON DELETE CASCADE`, so pruning also deletes their `applied_at`,
`stage`, `notes` and `followed_up_at` outright. `PruneJobs` weighs this in a comment — "a user's
saved job goes with it. That is an accepted cost of the campaign, not an oversight" — a correct
judgement about a bookmark that, through one shared cascade, silently covered applications too.
The ledger cannot stand in for the record: it is content-free by design ("no subject lines, no
bodies, no addresses"), so notes were never in it.

Both failures have one cause. An application is a durable fact and is stored as a property of a
row the catalogue deletes on a schedule.

## What Changes

- **New `applications` table** — the application's own identity and working state: `user_id`,
  `company_slug`, `role_title`, `applied_at`, `stage`, `notes`, `followed_up_at`, and an
  **optional** `job_id` naming the posting it came from while one exists. Uniqueness
  `(user_id, job_id) WHERE job_id IS NOT NULL` carries today's one-application-per-posting rule.
- **`application_events.application_id`** — the ledger correlates events by application, not by
  posting, so a reply stays attached to its application after the posting is gone. `job_id` stays
  as the (nullable) provenance of where the application came from.
- **`user_jobs` keeps only its catalogue-relation duties** — view, save, dismiss, vote; still
  keyed `(user_id, job_id)`, still cascading, which is right for a bookmark.
  **BREAKING** (internal): `applied_at`, `stage`, `notes`, `followed_up_at` move out.
- **Pruning stops destroying applications** — the link clears, the record stands.
- **`emails.job_id` becomes `emails.application_id`**, so a linked thread cannot be detached from
  an application that still exists.
- Migration backfills `applications` from `user_jobs WHERE applied_at IS NOT NULL` and points the
  existing ledger events and emails at the resulting rows.
- No user-visible behaviour changes, with one exception that is a bug fix: a company whose
  postings were pruned stops being reported as more silent than it was.

**Correction, made after reading `ListUserJobs`:** the sentence above holds for the storage
cutover and NOT for the goal. The board is driven by `user_jobs` rows and embeds a posting in
every card, and `cmd/prune` cascades that row away — so an application whose posting was removed
survives in the database and in every aggregate while staying invisible to its owner. Delivering
the stated goal therefore requires a wire-shape change, and this proposal's promise not to make
one was written before that was known. The change is scoped and specified rather than made
quietly: see the posting-less card requirement in `application-record`.

Out of scope, each its own change: reconstructing application history from a mailbox backfill,
the retrospective surface, and the import channel.

## Capabilities

### New Capabilities
- `application-record`: the application as an entity independent of the catalogue — its identity,
  its optional link to a posting, the guarantee that it outlives the posting, and the requirement
  that aggregates correlate events through it rather than through the posting.

### Modified Capabilities
- `catalog-pruning`: gains a requirement that removal MUST NOT destroy a user's application, nor
  change what the aggregates say about the employer. The cascade it relies on today is stated
  only in a SQL comment, never in the spec, so this is a new guarantee rather than a rewritten
  one.

`user-job-tracking` is deliberately **not** listed: its requirements describe wire behaviour —
idempotent apply, the stage vocabulary, the fields an interaction record carries — and each still
holds, because the backfill leaves every existing application carrying its `job_id`. Only its
Purpose prose ("one row per `(user, job)`") goes stale, and prose is not a requirement.

`company-hiring-signal` is not listed either, and the reason matters: its response-rate
requirement is already correct in words — "An application counts as answered when a non-retracted
`employer_reply` event exists **for it**". Nothing about that sentence needs changing. What
failed is the implementation, in a case nobody had a test for. So this change fixes a defect
against a standing requirement rather than revising one, and the new normative content — that the
correlation runs through an identity which survives the posting — belongs to `application-record`,
which owns that identity.

## Impact

- **Builds on #1344**, merged as `4b6ba903` and archived by #1345. The ledger, `internal/appevent`
  and the rebuilt rollup are the starting point.
- **Schema**: new `applications` table; `application_events.application_id`;
  `emails.job_id` → `emails.application_id`; `user_jobs` loses four columns. Next free migration
  number is **0064** — #1344 landed as 0062/0063 after its own rebase, and 0059 is already used
  twice.
- **SQL layer**: `user_jobs.sql`, `application_events.sql`, `insights.sql`, `mail_linking.sql`,
  `mail_classification.sql`, `ghost.sql`, `reminders.sql`, `stats.sql`, `jobs.sql`,
  `company_votes.sql`. `make sqlc` regenerates `internal/db`.
- **Go**: `internal/jobtracking`, `internal/inbox`, `internal/maillink`, `internal/appevent`
  callers, `internal/handler` (`user_jobs.go`, `me_tracking.go`, `inbox_linking.go`,
  `followup.go`, `stats.go`, `company_response.go`, `assistant_tracking_tools.go`),
  `internal/ghostreport`, `cmd/classify-mail`, `cmd/backfill-application-events`, `cmd/prune`.
- **Consumers**: the SPA tracking board and inbox, `freehire-cli`, `freehire-mcp`. Wire shapes are
  unchanged; only what backs them moves.
- **Production**: the backfill runs over live tracking data, so it must be re-runnable, must take
  the prune lock or run when no prune timer can fire, and must not be applied inside the nightly
  dump window — a DDL lock queued behind a long read takes readers down with it.
