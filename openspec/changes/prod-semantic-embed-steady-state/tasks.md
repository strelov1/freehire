## 1. Author systemd units (freehire-ops repo)

- [x] 1.1 Create `provision/host2/systemd/freehire-embed.service` in
      `freehire-ops`, modeled on `freehire-search-drain.service`: `Type=oneshot`,
      `User=freehire`, `WorkingDirectory=/opt/freehire/src/hire-current`,
      `EnvironmentFile=/opt/freehire/.env`, `CPUWeight=10`, `IOWeight=10`, a
      reindexw guard (skip this tick if a facet reindex is running — design.md
      Decision 4), `ExecStart=/opt/freehire/src/hire-current/embed`.
      Did NOT touch the already-modified `freehire-search-drain.service` in the same
      directory — confirmed via `git status`/`git diff origin/main` (proposal.md
      Impact note).
      **Code review + empirical host2 testing found the initial guard
      (`! systemctl is-active --quiet freehire-reindexw.service`) silently
      no-ops during reindexw's actual multi-hour run (state=`activating`, not
      `active`) — fixed to match on the known-clear states instead. Also verified
      the timer reschedules cleanly regardless of ExecStartPre outcome (no
      start-limit-hit risk). See design.md Decision 4.**
- [x] 1.2 Create `provision/host2/systemd/freehire-embed.timer`: hourly
      (`OnUnitActiveSec=1h`, `OnBootSec=5min`, `Persistent=true`,
      `WantedBy=timers.target`) — design.md Decision 6.
- [x] 1.3 Stage ONLY the two new files (`git add` by exact path, not `-A`) — the
      working tree has unrelated uncommitted changes that must not be bundled
      (proposal.md Impact note). Verified: worktree branched fresh from
      `origin/main`, isolated from the parent checkout's WIP branch entirely.

## 2. One-time backfill (host2 ops)

- [x] 2.1 Confirm current backlog size and `freehire-reindexw.service` is not
      active before starting (don't stack with a facet reindex — same class of
      hazard this session hit repeatedly). Confirmed: 378,604 pending, reindexw
      inactive.
- [x] 2.2 `systemctl stop freehire-search-drain.timer` (design.md Decision 2). Its
      already-running service instance (not just the timer) also had to be
      stopped directly — same permanent-daemon pattern hit earlier this session.
- [x] 2.3 Launch `cmd/embed` as a transient `systemd-run` unit with
      `EMBED_BATCH_SIZE=2000` (design.md Decision 1). Do NOT use the old
      `embed-supervisor.sh`. First real batch: 1941 docs in 5m12s (~6.2/s) — far
      below the ~33/s expected. Retried at `EMBED_BATCH_SIZE=5000`: 4857 in
      5m46s (~14/s) — better but still not the expected rate. `EXPLAIN ANALYZE`
      isolated the real cause to `ClaimSemanticBatch` itself (109s/call,
      independent of batch size) — see the new Section 2a below; the backfill
      was paused (worker stopped) pending that fix.
- [ ] 2.4 Once Section 2a ships, relaunch the backfill (same command) and monitor
      to completion (journal + `semantic_outbox` claimable count trending to 0).
      If the process dies, just relaunch the same command — safe per design.md's
      lease-based risk mitigation.
- [ ] 2.5 `systemctl start freehire-search-drain.timer`.

## 2a. Fix ClaimSemanticBatch's O(claimable-set) claim cost (hire repo, code)

Discovered mid-backfill (task 2.3) via `EXPLAIN (ANALYZE, BUFFERS)` on the live
query: see design.md Decision 8 for the full root-cause writeup and design.

- [ ] 2a.1 RED: add a failing integration test in `internal/db` asserting
      `ClaimSemanticBatch` returns rows in `job_posted_at DESC` order using the
      new column (not a join to `jobs`), and that `EnqueuePendingSemanticJobs`
      populates `job_posted_at` correctly (`COALESCE(posted_at, created_at)`) for
      both the add/update and closed-removal enqueue paths.
- [ ] 2a.2 Add migration `0080_semantic_outbox_job_posted_at.sql`:
      `ALTER TABLE semantic_outbox ADD COLUMN job_posted_at timestamptz`
      (nullable — no `SET NOT NULL`, design.md Decision 8), a one-time
      `UPDATE ... FROM jobs` backfill for existing rows, and
      `CREATE INDEX semantic_outbox_claim_idx ON semantic_outbox
      (job_posted_at DESC, job_id DESC) WHERE failed_at IS NULL`. Apply to prod
      BEFORE deploying the code that reads/writes the column (existing repo
      convention for outbox-table migrations, per 0004/0076's own header
      comments).
- [ ] 2a.3 GREEN: update `EnqueuePendingSemanticJobs` to write `job_posted_at =
      COALESCE(posted_at, created_at)` on insert; update `ClaimSemanticBatch`'s
      CTE to `ORDER BY o.job_posted_at DESC NULLS LAST, o.job_id DESC` with no
      join (the outer `UPDATE ... RETURNING` join to `jobs` for the `closed` flag
      stays — it's only over the small claimed batch, not the whole claimable
      set). `make sqlc` (or the pinned local binary — Docker timed out earlier
      today) to regenerate.
- [x] 2a.4a Code review found two real issues, both fixed:
      **(1)** the migration used a plain `CREATE INDEX` on a 900k+ row table under
      continuous prod write traffic — no lock safety. Fixed to
      `-- migrate: no-transaction` + `CREATE INDEX CONCURRENTLY IF NOT EXISTS`,
      matching the exact precedent `migrations/0078_jobs_source_id_idx.sql`
      already set. **(2)** design.md's "jobs.posted_at is immutable post-ingest"
      claim was checked against the code and is false — `UpsertJob`'s
      `ON CONFLICT DO UPDATE` overwrites it every re-ingest. Impact is low
      (ordering staleness only, self-heals, no data loss/duplication — the same
      class of staleness `created_at` already accepts under the same
      `ON CONFLICT DO NOTHING`), corrected in design.md rather than requiring a
      redesign. Re-ran the full integration suite after both fixes — all green,
      including `CREATE INDEX CONCURRENTLY` inside `no-transaction` applying
      cleanly against a fresh test Postgres.
- [ ] 2a.4b Verify the fix live: re-run `EXPLAIN (ANALYZE, BUFFERS)` against prod
      with the new query/index and confirm claim time drops from ~109s to
      near-instant.
- [x] 2a.5 Full verification suite: `go build/vet ./...`,
      `go test ./...`, `go vet -tags=integration ./...`,
      `go test -tags=integration ./internal/db/... ./cmd/embed/...`. All green.
- [x] 2a.6 PR, merge, deploy (`release.sh freehire`). First attempt FAILED live:
      `CREATE INDEX CONCURRENTLY cannot run inside a transaction block` —
      `release.sh` correctly aborted without touching the live color (verified
      prod's `semantic_outbox` had no `job_posted_at` column and no
      `schema_migrations` row for 0080 afterward — clean rollback, no partial
      state). Root cause: a migration file's statements are sent to Postgres as
      one multi-statement message, which Postgres itself implicitly wraps in a
      transaction regardless of the `no-transaction` marker (that marker only
      stops `internal/migrate`'s own `BEGIN`/`COMMIT` wrapping) — `CONCURRENTLY`
      forbids running inside ANY transaction block, implicit or explicit. 0078
      worked because it has exactly one statement per file; this migration had
      three. Fixed by splitting: `0080` keeps `ADD COLUMN` + the `UPDATE`
      backfill (ordinary transactional DML, no change needed there); `0081`
      is `CREATE INDEX CONCURRENTLY IF NOT EXISTS` alone, matching 0078's
      exact working shape. Re-verified locally (fresh testcontainer Postgres
      applies both files cleanly) before redeploying.

## 3. Publish a clean jobs_semantic (host2 ops)

- [ ] 3.1 Run `reindex --semantic --from-pg`, monitor to completion.
- [ ] 3.2 Verify: `jobs_semantic` doc count is sane (close to the eligible-jobs
      count from proposal.md's measurement, adjusted for backlog closed since);
      spot-check a few ids that were confirmed stale/closed before this change no
      longer appear.

## 4. Retire the old supervisor (host2 ops)

- [ ] 4.1 `systemctl disable --now freehire-embed-supervisor.service`.
- [ ] 4.2 `rm /opt/freehire/embed-supervisor.sh /opt/freehire/embed-supervisor.log`
      (and `/etc/systemd/system/freehire-embed-supervisor.service`) — confirmed not
      tracked in `freehire-ops` (only mentioned in an unrelated runbook doc), so no
      repo-side deletion needed.

## 5. Install and verify the new steady-state timer (host2 ops)

- [ ] 5.1 Copy the two new unit files from the merged `freehire-ops` checkout to
      `/etc/systemd/system/`, `systemctl daemon-reload`,
      `systemctl enable --now freehire-embed.timer`.
- [ ] 5.2 Wait for (or manually trigger) one tick; verify it completes cleanly with
      a small delta (journal shows a normal drain-to-empty, not a multi-hour run —
      confirms the backlog from Section 2 actually cleared).

## 6. Verify

- [ ] 6.1 `semantic_outbox` claimable count near 0 in steady state.
- [ ] 6.2 `jobs_semantic` doc count matches the eligible-jobs count within normal
      drift.
- [ ] 6.3 No `freehire-embed-supervisor` unit or leftover files remain on host2.
- [ ] 6.4 `web/src/lib/api.ts`'s `semantic_ratio` unchanged (still 0) — confirms
      scope discipline (proposal.md's explicit out-of-scope item).
