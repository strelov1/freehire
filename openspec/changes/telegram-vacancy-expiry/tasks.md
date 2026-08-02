## 1. The close reason column

- [ ] 1.1 Write `migrations/0071_jobs_closed_reason.sql`: add `closed_reason text NOT NULL
  DEFAULT ''` to `jobs`, with a `CHECK` permitting `''`, `unseen`, `feed_removed`,
  `moderated`, `probe_expired`, `expired`. The empty string MUST be permitted — it is the
  default every existing row takes, so a constraint without it rejects the whole table.
  Additive with a default, so no table rewrite.
- [ ] 1.2 Add a failing integration test (`internal/db`, tag `integration`) asserting that
  each of the five close queries writes its own reason: `CloseUnseenJobs` and
  `CloseUnseenJobsBySource` → `unseen`, `CloseJobBySourceExternalID` → `feed_removed`,
  `CloseJobByID` → `moderated`, `MarkLivenessExpired` → `probe_expired`.
- [ ] 1.3 Set the reason in those five queries in `internal/db/queries/jobs.sql`, run
  `make sqlc`, update the Go callers for any changed signatures. Tests green.
- [ ] 1.4 Add a failing test that a reopened job carries no reason, then clear
  `closed_reason` alongside `closed_at = NULL` in `UpsertJob`, `UpsertManualJob` and
  `TouchJob`. `make sqlc`. Tests green.

## 2. The age rule

- [ ] 2.1 Add a failing integration test for the new query: a `telegram` row 46 days old
  closes with reason `expired`; one 44 days old does not; a `greenhouse` and a `manual` row
  a year old are untouched whatever their age. Include the exact-boundary case.
- [ ] 2.2 Write `CloseStaleUnsignalledJobs` in `internal/db/queries/jobs.sql` — closes open
  jobs whose `source = ANY($1)` and whose `COALESCE(posted_at, created_at)` is older than the
  window, setting `closed_reason = 'expired'`. Take the source list and the window as
  parameters rather than hardcoding, so the caller owns the policy. `make sqlc`. Tests green.
- [ ] 2.3 Call it from `cmd/liveness` under the existing advisory lock, passing
  `unprobableSources` and a 45-day window, and log the number closed. Rename or re-comment
  `unprobableSources` so its two roles are visible: excluded from the probe, and subject to
  the age rule instead.

## 3. Documentation

- [ ] 3.1 Update `docs/agents/job-lifecycle.md`: add the age rule as mechanism (4) to the
  "Always true" list and the "How it works" prose, record that every close now carries a
  reason, and remove the Telegram gap from Limitations now that it is closed.
- [ ] 3.2 Update `internal/telegram/AGENTS.md`, whose Limitations section currently states
  that Telegram jobs stay open until something else closes them.

## 4. Verification

- [ ] 4.1 `go test ./...` and `go test -tags=integration ./internal/db/`. Both green.
- [ ] 4.2 Dry-run the count against prod before the first live run: the age rule should match
  ~1,905 rows at a 45-day window. A number far from that means the window or the source
  scope is wrong, and it is cheaper to learn it from a `SELECT` than from 10,395 closed rows.
