## 1. Schema

- [x] 1.1 Add `migrations/0058_cvs_is_tailored.sql`: `cvs.is_tailored boolean NOT NULL DEFAULT
      false`, backfilled `UPDATE cvs SET is_tailored = true WHERE job_id IS NOT NULL`. The comment
      must say why the flag exists — that `cmd/prune` removes the vacancy link, so its absence
      cannot mean "not tailored" — because the next reader will otherwise see a redundant column.
- [x] 1.2 Verify the migration applies to a database seeded with the current schema. (The partial
      unique index this task originally called for was dropped: users legitimately own several plain
      CVs, so there is no uniqueness to prove — see design, "corrected mid-implementation".)

## 2. Queries

- [x] 2.1 Point `GetBaseCVByUser` at `NOT is_tailored` instead of `job_id IS NULL`, keeping the
      `ORDER BY … LIMIT 1` exactly as it was — it is what chooses among a user's several plain CVs,
      not a workaround.
- [x] 2.2 Set the flag explicitly in both creation queries: `CreateCV` writes `is_tailored = false`,
      `CreateTailoredCV` writes `is_tailored = true`. Leave `ListTailoredCVsByUser` on its inner join
      — orphan visibility is out of scope (see design Non-Goals).
- [x] 2.3 `make sqlc` and confirm the regenerated `internal/db` compiles with the new column.

## 3. Store

- [x] 3.1 Expose the flag on `cv.Record` (`IsTailored`) so a reader can tell a base CV from an
      orphaned copy. The kind itself is written in SQL, not passed from Go, so `Create`/`CreateTailored`
      keep their signatures.
- [x] 3.2 Integration test the defect itself at the layer that owns the predicate
      (`internal/db/cvs_base_integration_test.go`), watched failing first: with a base CV and a
      tailored copy whose vacancy is then DELETEd, `GetBaseCVByUser` returned the ORPHAN. A
      store-level test over the in-memory fakeRepo would have tested the fake, not the SQL.
- [x] 3.3 Test that a user whose only vacancy-less CV is an orphan is treated as having NO base
      (`TestBaseCVAbsentWhenOnlyAnOrphanRemains`). The follow-on — `Tailor` then seeding a fresh base
      from the résumé — is the existing seed path reached by that same no-row answer, and is left to
      its own tests rather than re-asserted here.

## 4. The delta's refusal

- [x] 4.1 Split the 409 in `internal/handler/cv_ats_delta.go`: the base CV gets a reason saying it
      is the base; a tailored copy whose vacancy is gone gets a reason saying the vacancy no longer
      exists. Integration tests for both, asserting the reason text distinguishes them — a shared
      409 with the wrong sentence is the defect being fixed, not a cosmetic detail.
- [x] 4.2 Baseline selection is covered by the two ends of the chain rather than a third test:
      `TestBaseCVSurvivesAPrunedVacancy` proves the lookup skips a newer orphan, and
      `TestATSDelta_ComparesTheTailoredCopyAgainstTheBase` proves the delta reports the CV that
      lookup returned as its baseline.

## 5. Close out

- [x] 5.1 `go build ./... && go vet ./... && go test ./...`, plus
      `go test -tags=integration ./internal/handler/ ./internal/cv/`. Web is untouched — no wire
      change — but run `pnpm run check` if anything in `web/` moved.
- [x] 5.2 Record the flag in `internal/cv/AGENTS.md`: what `is_tailored` means, why the absence of
      `job_id` cannot carry it, that there is deliberately no `is_base` and no uniqueness, and the
      backfill's limit (a row orphaned before the migration cannot be recovered from `job_id`).
- [x] 5.3 Carry the post-flip reconciliation statement (design → Deploy window) into the PR
      description, so whoever releases runs it once after the colour flip.
