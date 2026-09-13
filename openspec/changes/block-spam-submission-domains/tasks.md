## 1. Migration and generated queries

- [x] 1.1 Add migration `migrations/0160_submission_domain_blocklist.sql`: table
      `submission_domain_blocklist` (`id bigserial pk`, `host text not null`,
      `blocked_by bigint not null references users(id)`, `reason text not null default ''`,
      `created_at timestamptz not null default now()`), unique index on `host`.
- [x] 1.2 Add sqlc queries: `IsHostBlocked`/`BlockHost` in a new
      `submission_domain_blocklist.sql`; `ListPendingSubmissionURLs` (id+url of every
      pending row) and `MarkSubmissionsRejectedByIDs` (bulk `UPDATE ... WHERE id = ANY($1)
      AND status='pending'`) in `job_submissions.sql`. (Host matching is done in Go via
      `normalizeHost`, not in SQL — see 2.1's note.)
- [x] 1.3 Run `make sqlc`; verify the diff is only the new generated code.

## 2. Domain logic

- [x] 2.1 **Deviates from the original sketch**: folded the blocklist into the existing
      `submission.Repository` interface (`IsHostBlocked`, `RejectAndBlockHost`) instead of
      a separate `HostBlocklist` port. `RejectAndBlockHost` needs one Postgres transaction
      spanning `submission_domain_blocklist` and `job_submissions` (see design.md's
      "same DB transaction" decision) — a second port would still need the same adapter to
      implement it for atomicity, so the extra interface bought no isolation `Repository`
      didn't already give. Design.md's *behavior* (atomicity, exact-host match, moderator-
      only mutation) is unchanged.
- [x] 2.2 Added `normalizeHost`/`hostOf` in `internal/ingest/submission/submission.go`
      (lowercase, strip one leading `www.`); exercised via `Submit`/`Reject` unit tests
      rather than standalone table tests, since the package exports neither function.
- [x] 2.3 `Service.Submit`: checks `repo.IsHostBlocked` after `Validate()`; returns
      `ErrBlockedDomain` without calling `repo.Create` when blocked.
- [x] 2.4 `Service.Reject` gained `blockDomain bool`; when true it calls
      `repo.RejectAndBlockHost`, which blocks the host, bulk-rejects every other pending
      submission on that host, and rejects the target — all inside one transaction in the
      adapter (`repository.go`).
- [x] 2.5 Unit-tested `Submit` (blocked host refused, `www.`/case variants via
      `normalizeHost`, non-blocked unaffected) and `Reject` (plain reject untouched;
      `blockDomain=true` routes to `RejectAndBlockHost` with the normalized host; an
      already-decided target never touches the blocklist) with a `fakeRepo`. The
      blocklist's own already-blocked-is-a-no-op behavior is DB-level (`ON CONFLICT DO
      NOTHING`) and is covered by the integration test in section 3, not a fake.

## 3. HTTP handler

- [x] 3.1 `internal/api/handler/submissions.go`: `submissionError` maps `ErrBlockedDomain`
      to `403`, so both `CreateSubmission` (via `Submit`) and any other caller through it
      get the mapping automatically.
- [x] 3.2 `RejectSubmission`: `rejectRequest` gained `BlockDomain bool` (`json:"block_domain"`),
      passed through to `Service.Reject`; response shape unchanged (still the target
      submission only — the frontend re-fetches to see bulk-rejected siblings, see 5.2).
- [x] 3.3 Added `TestSubmissionDomainBlocklistEndToEnd` to
      `submissions_integration_test.go` (`//go:build integration`, ran locally against
      Docker/testcontainers): blocked-host submission refused with no row written;
      `block_domain: true` blocks the host and bulk-rejects a pending sibling; a
      `www.`-prefixed resubmission is also refused; a pre-existing pending row on an
      already-blocked host rejects cleanly with no duplicate blocklist row.

## 4. Repository adapter

- [x] 4.1 Folded into `submission.QueriesRepository` (`internal/ingest/submission/repository.go`)
      rather than a separate adapter — see 2.1's note. `IsHostBlocked` delegates straight to
      the generated query; `RejectAndBlockHost` runs the block + candidate fetch + bulk
      reject in one `pool.Begin`/`Commit` transaction, mirroring the existing
      `moderation.QueriesRepository.Create` transactional pattern.
- [x] 4.2 `QueriesRepository` now needs a `*pgxpool.Pool` (like `moderation`'s adapter
      already does), so `NewQueriesRepository(q, pool)`'s call sites were updated:
      `newSubmissionHandlers` (now takes `pool` too, wired from `cfg.Pool` in
      `internal/api/handler/handler.go`) and the three integration-test constructions.

## 5. Frontend

- [x] 5.1 `web/src/lib/api.ts`: `rejectSubmission` gained an optional `blockDomain`
      parameter, sent as `block_domain` in the request body.
- [x] 5.2 `web/src/lib/components/ModerationView.svelte`: after the existing reason
      prompt, a `window.confirm` (using the URL's host via a small `hostOf` helper) asks
      whether to also block the domain; on success with `blockDomain` set, the queue is
      re-fetched (`queueData.run(...)`) instead of only dropping the one id, since other
      rows may have been bulk-rejected too.
- [x] 5.3 `pnpm install` in `design-system/` and `web/` (fresh worktree, no
      `node_modules`); `pnpm run check` (svelte-check): 0 errors, 39 pre-existing warnings
      unrelated to the touched files; `eslint` on both changed files: clean.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean; `CGO_ENABLED=0 go vet ./...` clean (see note below); `CGO_ENABLED=0
      go test ./...` all green except `cmd/billing-sync`'s
      `TestTheStoreProviderAloneKeepsTheWorkerRunning`, pre-existing and unrelated (that
      package is untouched by this change).
      **Environment note**: this machine's `go build`/`go vet`/`go test` fail to LINK any
      package that pulls in CGO frameworks (`ld: ... tapi error: malformed file` against
      `MacOSX27.0.sdk` — a broken/mismatched Xcode Command Line Tools install, unrelated to
      this change). `CGO_ENABLED=0` sidesteps it for every command in this task list;
      worth fixing the toolchain separately since it would otherwise block `go build ./...`
      for anyone on this machine.
- [x] 6.2 `CGO_ENABLED=0 go vet -tags=integration ./...` clean. `go test -tags=integration
      ./internal/api/handler/...` run against real Postgres via Docker/testcontainers:
      all green, including the new `TestSubmissionDomainBlocklistEndToEnd` and the
      existing submission suite (updated for the new `NewQueriesRepository(q, pool)`
      signature).
- [x] 6.3 Not done as a manual browser session — covered instead by the integration test
      (3.3) exercising the same HTTP endpoints `/moderation` calls, plus `svelte-check`/
      `eslint` on the UI change (5.3). Flagging per the repo's UI-testing convention: this
      was not clicked through in an actual browser, so treat the visual confirm-dialog
      wording/flow as unverified until someone does.
