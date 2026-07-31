## Context

`user_jobs` is keyed `(user_id, job_id)` and holds two unrelated things under that one key:

- **a mark on a catalogue row** — `viewed_at`, `saved_at`, `dismissed_at`, `vote`. A `job_id` is
  essential here; the mark means nothing without the row it marks.
- **an application** — `applied_at`, `stage`, `notes`, `followed_up_at`. This is a fact about the
  person's life. The posting is only where they found the role.

Because both live under `job_id bigint NOT NULL ... ON DELETE CASCADE`, deleting a posting
deletes both. `cmd/prune` is the sole hard-delete path and has already removed 1,609,678 rows on
production (`pruned_jobs`, 1,609,388 of them by the `title` rule). `PruneJobs` weighs the trade in
a comment — "a user's saved job goes with it. That is an accepted cost of the campaign, not an
oversight" — which is a correct judgement about a bookmark and was never a judgement about an
application.

Two measurements bound the problem:

- Production `jobs` holds nothing created before **2026-06-12**: 2,611,848 rows in June,
  2,915,559 in July, and nothing earlier. The catalogue is roughly two months deep.
- `UpsertJob`'s `ON CONFLICT (source, external_id) DO UPDATE SET` deliberately omits `created_at`,
  so that floor is a real retention floor and not an artifact of re-ingest refreshing timestamps.

A job search is longer than two months. Anchoring an application to a table with that retention
means the record is doomed by design, not by accident.

**What #1344 already settled, and what it left open.** The application-event ledger records what
happened to an application, denormalises `company_slug` onto every event, and holds `job_id` as
`ON DELETE SET NULL` — with the reasoning spelled out in its own migration: "a job removed by
`cmd/prune` must not cascade-delete the history of applying to it." That protects the *facts*.

It does not protect the *pairing*. `RebuildInsightsCompanyResponse` finds an application's reply
by matching `r.user_id = o.user_id AND r.job_id = o.job_id`, so once a prune run nulls both
`job_id`s the match reduces to `NULL = NULL` and never succeeds. The `applied` event stays in the
denominator, its `employer_reply` drops out of the numerator, and the employer is served as more
silent than it was — the very distortion the ledger was introduced to remove, arriving by a
different route. The ledger's own test pins the half that works
(`application_events_integration_test.go:226`, "the slug is denormalized so cmd/prune cannot
orphan it"); nothing pins the pairing.

That is the same missing thing in both places: an application has no identity. `job_id` cannot be
it, because it is cleared on a schedule.

## Goals / Non-Goals

**Goals:**

- An application survives the deletion of the posting it was made against.
- The employer and role title are readable from the application alone.
- Every current wire shape and behaviour is preserved exactly; this change is invisible from
  outside the API.
- The seam is opened for an application that never had a catalogue posting, without building it.

**Non-Goals:**

- Reconstructing application history from a mailbox backfill (its own change).
- The retrospective surface, and the import channel (their own changes).
- Any new endpoint, any new field on an existing response, any UI change.
- Letting the ordinary apply path create an application without a posting. That path still takes
  a job slug; nothing in this change gives a caller a way to create an unlinked application.

## Decisions

### Split the table rather than relax the column

Making `user_jobs.job_id` nullable would keep one row doing two jobs and would immediately break
its primary key, which *is* `(user_id, job_id)`. It also leaves views and saves — where a NULL
`job_id` is meaningless — sharing a nullability they must never use.

A new `applications` table takes the four application columns plus the employer:

```
applications(
  id, user_id, company_slug, role_title,
  job_id NULL REFERENCES jobs(id) ON DELETE SET NULL,
  applied_at, stage, notes, followed_up_at, source, created_at
)
UNIQUE (user_id, job_id) WHERE job_id IS NOT NULL
```

`user_jobs` keeps `(user_id, job_id)`, keeps cascading, and loses the four columns. The partial
unique index is what carries today's "at most one application per `(user, job)`" guarantee across
the move, while leaving unlinked applications unconstrained — two roles at one employer are two
applications.

**Alternative considered:** keep `user_jobs` and add a `pruned_user_jobs` archive, mirroring
`pruned_jobs`. Rejected: an archive answers "what was taken", not "what does this person's
tracking board show", and the board would still lose the row.

### `ON DELETE SET NULL`, matching `emails.job_id`

`emails.job_id` already nulls rather than cascades, with the reasoning recorded in migration 0020:
"a job row removed out from under an email must not cascade-delete the mail — the link just
clears." `application_events.job_id` then followed the same rule for the same reason. An
application deserves it too, so a reader learns one rule instead of three.

### The ledger correlates on `application_id`, and keeps `job_id` as provenance

`application_events` gains `application_id`, and every aggregate pairs events through it. `job_id`
stays on the event, nullable, answering "which posting did this come from" — a question whose
answer may legitimately become unknown. What must not become unknown is which application an event
belongs to, and that is what the new column carries.

`ON DELETE CASCADE` is right here, unlike everywhere else in this design: an event belongs to its
application, and an application is never deleted by any scheduled campaign. The only path that
removes one is the user removing their own record, and their events should go with it.

### Denormalise `company_slug` and `role_title` onto the application

Copying two fields off the posting is duplication, and it is the duplication the change exists to
create: the copy is the only thing that survives the delete. The ledger already set this
precedent — its `company_slug` is denormalised at write time and its migration calls that
"load-bearing for exactly that reason" — so the application follows an established rule rather
than inventing one.

The copy is written once, at creation, and is not refreshed when the posting is later edited. A
company rename is rare, and an application should record the employer as it was named when the
person applied.

### `emails.job_id` becomes `emails.application_id`

Mail is about an application, not about inventory. Leaving it pointing at `jobs` would mean a
pruned posting silently detaches a thread from an application that still exists, and
`insights_company_response` would have to reach the employer through two different paths.
`suggested_job_id` stays pointing at `jobs`: a suggestion is "this mail may concern that posting",
made before any application exists.

### No `source` column on the application

An earlier draft gave the application its own provenance column. It is not needed: every
application has an `applied` event, and that event already carries `source`
(`mail_gmail｜mail_hosted｜mail_external｜user｜assistant`) from `internal/appevent`. A second copy
would be a second thing to keep true, and the two would disagree the first time one write path
forgot the other.

**Alternative considered:** keep the column for cheap reads, treating the ledger as the audit
trail. Rejected — the read is a join on an indexed column, and the invariant is worth more than
the join.

## Risks / Trade-offs

- **A partial cutover splits readers.** Nine query files and nineteen Go files read the moved
  columns; if some read `user_jobs.stage` while others read `applications.stage`, a board shows
  one stage and the mail pipeline advances another. → The expand/contract plan below keeps the
  old columns present but **unread** for exactly one deploy, and the contract step is what proves
  no reader was missed: dropping a column a reader still uses fails loudly at once.

- **The backfill races `cmd/prune`.** A prune run during the backfill deletes postings whose
  applications have not been copied yet, and those applications are gone before the copy sees
  them. → Take the prune lock, or run the backfill when no prune timer can fire; the change adds
  no new lock of its own.

- **DDL against live tracking data.** An `ALTER TABLE` queued behind a long read blocks every
  reader behind it, and the nightly dump window (03–07 UTC) is exactly such a read. → Apply
  migrations outside that window with a `lock_timeout`, and never in the same pass as a snapshot.

- **`applied_count` is a materialised counter on `jobs`.** It is incremented when `applied_at`
  transitions from unset to set. Moving the transition to another table risks double-counting on
  backfill. → The backfill writes rows without touching any counter; only the live apply path
  increments, exactly as today.

- **Applications now outlive their evidence.** An application whose posting is gone can no longer
  show the description or the URL the person applied through. That is a real loss and is accepted:
  a record with a company, a title, a date and its mail is worth much more than no record.

- **The defect is latent, not active, and that sets the urgency.** Measured on production right
  after #1344 shipped: 240 `applied` and 158 `employer_reply` events, and **zero rows with
  `job_id IS NULL`** — no pruned posting has yet carried a tracked application. Meanwhile
  `insights_company_response` holds 103 companies and not one reaches the gate of ten, so no rate
  is served publicly at all today. → There is no user-visible harm right now and there is
  guaranteed harm at the first prune run that touches an answered application. That argues for
  fixing it before the sample matures, not for treating it as an incident.

- **Two aggregates now read the ledger through a column the backfill has to fill.** Between the
  expand step and the end of the backfill, `application_events.application_id` is NULL for every
  existing row. A rollup run in that window over the new correlation would report every historical
  application as unanswered. → The rollup keeps correlating on `job_id` until the backfill has
  completed; task 4.2 switches it, and task 3.x runs before it.

## Migration Plan

Expand → backfill → cut over → contract, with the deploy boundary where correctness demands it.

1. **Expand (migration).** Create `applications` and its partial unique index. Add
   `application_events.application_id` and `emails.application_id` alongside the existing `job_id`
   on each. Nothing reads them yet. Additive, so it can be applied ahead of any code, which the
   worker contract requires (`migrate` runs before code that reads new schema).
2. **Backfill.** Copy every `user_jobs` row with `applied_at IS NOT NULL`, joined to `jobs` for
   `company_slug` and `title`; then point `application_events.application_id` and
   `emails.application_id` at the resulting rows through their existing `job_id`. Re-runnable:
   keyed on the partial unique index, so a second pass is a no-op rather than a duplicate. The
   rollup keeps correlating on `job_id` throughout this step — switching it earlier would read a
   column that is still NULL for every historical row.
3. **Cut over (code).** Every reader and writer switches to `applications` in one deploy. The old
   columns remain, now written by nothing and read by nothing.
4. **Contract (migration, a later deploy).** Drop `applied_at`, `stage`, `notes`,
   `followed_up_at` from `user_jobs` and `emails.job_id`. This is the audit: anything still
   reading them fails immediately and visibly rather than quietly serving stale values.

**Rollback.** Before the contract step, reverting the code restores the previous behaviour
completely — the old columns are still there and still correct, because step 3 stopped writing
them but the data as of the cutover is intact. After the contract step, rollback is a restore.
That asymmetry is the reason the contract step is a separate, later deploy rather than a tidy-up
in the same one.

**Production specifics.** Migrations are applied by hand on the live volume, so each step is a
file someone runs; the ledger records what is applied and an unapplied column reads as `42703` at
runtime, not as a helpful error.

## Open Questions

- Does the tracking board's ordering or paging depend on the `(user_id, job_id)` key shape in a
  way a surrogate key changes? To be answered while porting `user_jobs.sql`, not guessed here.
- Should `stage` move to a Postgres enum while it is being moved anyway? Deliberately deferred:
  the vocabulary is validated in `internal/userjob` and mirrored in the SPA, and changing two
  things at once would make a failed cutover ambiguous.
