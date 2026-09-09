## 1. Failing tests first

- [x] 1.1 In `cmd/backfill-remote-perk-false-positive/recompute_test.go`, add
      table-driven cases for `recomputeWorkMode`: the reported false positive
      (qualified perk phrase, no other signal → `""`), a location marker
      independently saying remote (untouched → `"remote"`), an unqualified remote
      phrase elsewhere in the description (still `"remote"`), a genuine denial
      after a location-silent description phrase (→ `"onsite"`), and no signal at
      all (`""`).
- [x] 1.2 Run `go test ./cmd/backfill-remote-perk-false-positive/...` and confirm
      the cases fail (red) before `recomputeWorkMode` exists.

## 2. Implementation

- [x] 2.1 Add `internal/platform/db/queries/jobs.sql` queries:
      `JobsForWorkModeRecheckByIDs` (location, description, work_mode for a named
      id set) and `SetJobWorkMode` (`IS DISTINCT FROM`-guarded write). Run
      `make sqlc`.
- [x] 2.2 Implement `cmd/backfill-remote-perk-false-positive/main.go`: candidate
      gathering from Meilisearch (`work_mode = "remote"` filter + "work from
      anywhere" query, paged, mirroring `cmd/backfill-clearance`'s `candidateIDs`),
      batched reads, `recomputeWorkMode`, and the guarded write. `BACKFILL_REMOTE_PERK_MAX`
      caps one run via `worker.EnvInt64`, unset = unbounded.
- [x] 2.3 Run `go test ./cmd/backfill-remote-perk-false-positive/...` and confirm
      all cases pass (green).

## 3. Verification

- [x] 3.1 `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- [x] 3.2 `go test ./...` green, no regressions.
- [x] 3.3 `go test ./internal/platform/arch/...` green (the new binary is
      `.gitignore`d, per `TestEveryCmdBinaryIsGitignored`).
- [ ] 3.4 Run the pass against production (`DATABASE_URL`, `MEILI_URL`,
      `MEILI_MASTER_KEY`), confirm the reported posting (freehire#2696,
      `paveakatroveinformationtechnologies/4730123005`) is corrected, then run a
      full `make reindex`.
