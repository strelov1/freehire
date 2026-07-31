## 1. Pin the defect before touching anything

- [x] 1.1 Integration test against the ledger as it stands: an application with an `applied` and an `employer_reply` event, whose posting is then deleted, must still be counted answered by `RebuildInsightsCompanyResponse`. It fails today — `(user_id, job_id)` correlation reduces to `NULL = NULL` — and staying red until task 4.2 is what proves the fix. **Verified red:** `answered = 0`, `applications = 1`
- [x] 1.2 Integration test: two applications by one user to the same employer, one answered, both postings deleted — exactly one stays answered. Guards against correlating on `(user, company)`, which would survive 1.1 by crediting a reply to every application to that employer. **Verified red:** `answered = 0`

> Groups 1–4 must ship in one PR. A knowingly-red integration test merged on its own turns the CI gate red for everyone.

## 2. Expand the schema

- [x] 2.1 Migration creating `applications` (`id`, `user_id`, `company_slug`, `role_title`, `job_id NULL REFERENCES jobs(id) ON DELETE SET NULL`, `applied_at`, `stage`, `notes`, `followed_up_at`, `created_at`) with `UNIQUE (user_id, job_id) WHERE job_id IS NOT NULL` and an index serving the per-user board read, as `migrations/0064_applications.sql` (#1344 landed as 0062/0063)
- [x] 2.2 Add `application_events.application_id` (nullable, `REFERENCES applications(id) ON DELETE CASCADE`) beside the existing `job_id`, which stays as provenance
- [x] 2.3 Add `emails.application_id` (nullable, `ON DELETE SET NULL`) beside `emails.job_id`, with a partial index mirroring `emails_job_id_idx`
- [x] 2.4 Integration test: deleting a `jobs` row clears `applications.job_id` and leaves the application, its ledger events and its linked mail intact

## 3. Carry over what exists

- [x] 3.1 Backfill query: one application per `user_jobs` row with `applied_at IS NOT NULL`, joined to `jobs` for `company_slug` and `title`, conflict-free against the partial unique index
- [x] 3.2 Point existing `application_events` at the carried-over applications through their `(user_id, job_id)`
- [x] 3.3 Point existing `emails.application_id` at them through `emails.job_id`
- [x] 3.4 Integration test: a viewed-but-not-applied interaction yields no application; a second run creates no duplicates and moves no event twice
- [x] 3.5 Extend `cmd/backfill-application-events` (or add a sibling worker) to run all three steps in order, following the worker contract

## 4. Give the ledger an application to correlate on

- [x] 4.0 `MarkJobApplied` creates the application in the same statement that records the apply, so the two cannot diverge — discovered while implementing 4.1, which has no application to name until this exists
- [x] 4.1 Every write path that records an event — `internal/maillink`, `internal/inbox`, `internal/jobtracking` — carries `application_id`; `internal/appevent` validates it is present for kinds that belong to an application
- [x] 4.2 `RebuildInsightsCompanyResponse` pairs `applied` with `employer_reply` on `application_id`; tasks 1.1 and 1.2 go green
- [x] 4.4 Integration test: the median reply time for a company is unchanged by deleting its postings

## 5a. Cut over the tracking core — storage only, behaviour identical

> **Indivisible.** Reads and writes of `applied_at`/`stage`/`notes`/`followed_up_at` flip in one
> pass; the design rejected dual-writing. Half of this port leaves the board reading one place
> and the apply path writing another.

- [x] 5a.0 `migrations/0065_applications_applied_at_optional.sql` — `applied_at` becomes nullable. `PATCH /jobs/:slug/track` sets a stage on a job never marked applied, and that row needs a home here; the alternative gave `stage` two homes, the duplication this change exists to remove. Decision recorded in the `application-record` spec

- [x] 5.1 Port `internal/db/queries/user_jobs.sql` to read and write `applications` for the application columns, keeping `user_jobs` for view/save/dismiss/vote; run `make sqlc`
- [x] 5.2 Port `internal/jobtracking` (`MarkApplied`, `TrackJob`, the board read) — `applied_count` stays incremented only on the live transition, never by the backfill
- [x] 5.3 Port `internal/userjob` silence reads so the stage ladder resolves from the application record
- [x] 5.4 Port `internal/handler/user_jobs.go`, `me_tracking.go` and `assistant_tracking_tools.go`; assert the wire shapes are identical to before
- [x] 5.5 Port `stats.sql` and `internal/handler/stats.go`

## 5b. The board renders an application with no posting — **CONTRACT CHANGE**

> Found while reading `ListUserJobs`: the board is driven by `user_jobs` rows and does
> `SELECT sqlc.embed(jobs)`, so every card requires a posting. `cmd/prune` cascades the
> `user_jobs` row away, so after 5a the record survives in the database and in every aggregate
> but the candidate still cannot see it. The stated goal is only half-delivered without this.
>
> `proposal.md` currently promises no wire-shape change. That promise has to be revised here,
> not quietly broken: the job fields become optional and a card falls back to the application's
> own `company_slug` and `role_title`.

- [x] 5b.1 Decide and record the wire shape. **Decided:** `job` becomes optional on a board row; `company_slug` and `role_title` ride at the top level, read from the application; and the row gains **an identifier of its own**. The last part is the finding — `JobBoard.svelte:54` keys every card, route and panel on `row.job.public_slug`, which a posting-less application does not have. Recorded as a requirement in `application-record`, and the proposal's "no wire-shape change" promise is corrected in place rather than quietly broken
- [x] 5b.2 Drive the board read from applications. **Solved without touching `ListUserJobs`:** the orphans come from their own query (`ListOrphanedApplications`) that joins nothing, so the `sqlc.embed` constraint below never applies and the posting-backed path is untouched. Merged in the repository.
  **Hard constraint, measured with a throwaway probe:** `sqlc.embed(jobs)` over a LEFT JOIN generates a
  non-pointer `Job` field, and scanning a row whose posting is absent fails at runtime —
  `can't scan into dest[0] (col: id): cannot scan NULL into *int64`. So the board query cannot keep
  `sqlc.embed`; it must select explicit nullable posting columns, and `jobview.FromRow` needs a variant
  that builds from them. That is the bulk of this task, and it was not visible from the query text
- [x] 5b.2a ~~FULL OUTER JOIN~~ — unnecessary once the orphans are a separate read. Source the rows from a FULL OUTER JOIN of `user_jobs` and `applications`: the board must
  still list a saved-but-never-applied job (only `user_jobs` has it) and an application whose posting
  is gone (only `applications` has it)
- [x] 5b.2b Emit one opaque row `id` the interface can key on — the posting's slug when there is one,
  otherwise derived from the application. Fits the house style already set by the opaque-id swap
- [x] 5b.3 SPA renders the fallback card and keys it on the row's own id, not on `job.public_slug`; `freehire-cli` and `freehire-mcp` tolerate the absent job
- [x] 5b.4 Integration test: an application whose posting was pruned still appears on its owner's board

## 6. Cut over the mail path

- [ ] 6.1 Port `mail_linking.sql` and `mail_classification.sql` to `emails.application_id`; run `make sqlc`. **Partly done in 5a:** their reads of `stage`/`applied_at` had to move with the rest of the cutover; the `emails.job_id` → `application_id` half is still open
- [ ] 6.2 Port `internal/inbox` (`RecordApplication`, the `?link=suggested|unlinked` filters) and `internal/maillink` so auto-link, suggestion and monotonic stage advance behave identically
- [ ] 6.3 Port `internal/handler/inbox_linking.go` and `followup.go`
- [ ] 6.4 Port `cmd/classify-mail/store.go`
- [ ] 6.5 Integration test: mail linked to an application whose posting is then deleted stays linked, and a forward-mapping signal still advances the stage
- [ ] 6.6 Port the link-correction path (the ordered two-statement retract-then-insert) to compare applications rather than postings, keeping the unique index's `retracted_at` term so a correction can still be recorded. **Moved out of group 4:** the retraction condition can only ask "did this mail move to another application" once the link paths maintain `emails.application_id`, which is 6.1–6.2

## 7. Cut over the remaining derived reads

- [x] 7.1 Port `ghost.sql` and `internal/ghostreport` so silent-application evidence reads applications, keeping both gates (two criteria, two distinct witnesses)
- [x] 7.2 Port `reminders.sql`, `jobs.sql` and `company_votes.sql`
- [ ] 7.3 Integration test: over a fixture with no deletions, the company response rate and median equal what they were before this change — the port moves the join, not the answer

## 8. Make pruning safe

- [x] 8.1 Integration test: a prune batch deleting a posting a user applied to leaves the application standing with its link cleared, leaves the company aggregates unchanged, and still removes the views, saves, dismissals and votes
- [x] 8.2 Replace `PruneJobs`'s blanket "accepted cost" sentence with the distinction it now makes
- [x] 8.3 Verify `cmd/prune`'s dry-run report and batch plan are unchanged

## 9. External consumers

- [ ] 9.1 Verify the SPA tracking board and inbox against a seeded local DB — no contract change is expected, so any diff is a bug in the port
- [ ] 9.2 Verify `freehire-cli` and `freehire-mcp`. **Checked by reading, not yet fixed:** `freehire-cli`'s `myJobRow.Job` is a value (`internal/cli/jobs.go:231`), and Go's json decoder treats `"job": null` as a no-op, so it does **not** error — it prints a row with a blank company and title. Graceful but wrong; the fix is `*jobRow` plus the `company_slug`/`role_title` fallback, in that repo's own PR. `freehire-mcp` not yet inspected

## 10. Contract (separate deploy)

- [ ] 10.1 Confirm no reader references `user_jobs.applied_at|stage|notes|followed_up_at` or `emails.job_id`, by grep and by a run with the columns renamed out of the way
- [ ] 10.2 Migration dropping those columns
- [ ] 10.3 Record in the deploy notes that groups 1–9 and group 10 are separate deploys, and that rollback is code-only until 10.2 is applied
