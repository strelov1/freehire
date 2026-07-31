## 1. Schema

- [x] 1.1 Add migration `0060_application_events.sql`: the `application_events` table
  (`user_id` FK users ON DELETE CASCADE, `job_id` FK jobs ON DELETE SET NULL, `company_slug`,
  `kind`, `signal`, `occurred_at`, `recorded_at`, `source`, `source_ref`, `retracted_at`), the
  partial unique index on `(user_id, kind, source_ref) WHERE source_ref IS NOT NULL`, and the
  aggregate-read index. Column comments carry the reasons: why the slug is denormalized, why
  `occurred_at` and `recorded_at` are separate, why retraction is a stamp and not a delete.
- [x] 1.2 Add migration `0061_insights_company_response_reply_time.sql`: the nullable
  `median_reply_days` column on `insights_company_response`, additive with no backfill. No
  unanswered column — it is `applications - answered`, computed by the serving layer.

## 2. Vocabulary and emission

- [x] 2.1 Create `internal/appevent`: the `Kind` and `Source` vocabularies with validation, in
  the shape `internal/userjob/stages.go` uses. Pure, no DB.
- [x] 2.2 Add the sqlc queries — record an event, retract by source reference — to
  `internal/db/queries/application_events.sql`; run `make sqlc`.
- [ ] 2.3 Emit `applied` from `jobtracking.MarkApplied`, inside the existing `LockJobForApply`
  transaction, only when `applied_at` was newly set.
- [ ] 2.4 Emit `stage_set` from `jobtracking.TrackJob`, only when the stage actually changed.
- [ ] 2.5 Emit `follow_up_sent` from the follow-up record action, one row per chase.
- [ ] 2.6 Emit `employer_reply` from `internal/maillink` when a message is both linked and
  classified.
- [ ] 2.7 Emit `employer_reply` from the `internal/inbox` paths that link outside the worker:
  suggestion confirmation, manual link, application-from-mail, external triage.
- [ ] 2.8 Retract on re-link: correcting an email's application retracts the prior event and
  records a new one at the same `occurred_at`.

## 3. Backfill

- [ ] 3.1 Add `cmd/backfill-application-events`: keyset pass in the manner of
  `cmd/backfill-derive`, replaying `applied`, `employer_reply`, and `follow_up_sent` from their
  existing timestamps. No `stage_set`.
- [ ] 3.2 Prove idempotency: a second full run over the same data adds no rows.

## 4. Aggregate

- [ ] 4.1 Rewrite `RebuildInsightsCompanyResponse` against `application_events`, keeping the
  connected-mailbox cohort gate on both sides and excluding retracted events.
- [ ] 4.2 Add the median days-to-first-reply and the unanswered count to the rebuild, computed
  over `occurred_at`.
- [ ] 4.3 Serve both in the company payload behind their own sample gates, absent below them.

## 5. Verification

- [ ] 5.1 Integration test: deleting a linked email leaves the company rate unchanged.
- [ ] 5.2 Integration test: re-linking an email moves the answered count between companies.
- [ ] 5.3 Integration test: two follow-ups on one application produce two readable events.
- [ ] 5.4 Integration test: a company above the gate with no replies serves a zero rate and no
  median.
- [ ] 5.5 Unit test: the backfill assigns no `stage_set` events and derives dates from the
  source timestamps, not from the run time.
- [ ] 5.6 Update `internal/userjob/AGENTS.md` and `docs/agents/mail-stack.md` with the ledger's
  place in each path, and note the retraction-vs-deletion distinction where the link rules live.
