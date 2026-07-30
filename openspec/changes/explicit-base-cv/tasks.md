## 1. Schema

- [ ] 1.1 Add `migrations/0058_cvs_is_base.sql`: `cvs.is_base boolean NOT NULL DEFAULT false`,
      backfill `UPDATE cvs SET is_base = true WHERE job_id IS NULL`, and a partial unique index on
      `(user_id) WHERE is_base`. The comment must say why the flag exists — that `job_id IS NULL`
      is produced by `cmd/prune`, so absence cannot mean "base" — because the next reader will
      otherwise see a redundant column.
- [ ] 1.2 Verify the migration applies to a database seeded with the current schema, and that the
      partial index is rejected when a user is given two base rows (prove the constraint bites).

## 2. Queries

- [ ] 2.1 Point `GetBaseCVByUser` at `is_base` instead of `job_id IS NULL`; keep the
      `ORDER BY … LIMIT 1` as a belt-and-braces guard now that the index enforces the invariant, and
      say so in the comment.
- [ ] 2.2 Set the flag explicitly in both creation queries: `CreateCV` writes `is_base = true`,
      `CreateTailoredCV` writes `is_base = false`. Leave `ListTailoredCVsByUser` on its inner join —
      orphan visibility is out of scope (see design Non-Goals).
- [ ] 2.3 `make sqlc` and confirm the regenerated `internal/db` compiles with the new column.

## 3. Store

- [ ] 3.1 Thread the flag through `internal/cv/store.go` (`Create`, `CreateTailored`, `BaseCV`,
      `Tailor`) so the kind is stated at every write. Tests: a base CV round-trips as base; a
      tailored copy round-trips as not-base.
- [ ] 3.2 Integration test the defect itself, and watch it fail before the fix: a user with a base
      CV and a tailored copy whose vacancy row is then DELETEd — `BaseCV` must return the base, and
      `Tailor` for a second vacancy must copy the base's document, not the orphan's. This is the
      test that would have caught the bug, so it must fail against the old predicate.
- [ ] 3.3 Test that a user whose only vacancy-less CV is an orphan is treated as having NO base:
      `Tailor` seeds a fresh base from the résumé and leaves the orphan untouched.

## 4. The delta's refusal

- [ ] 4.1 Split the 409 in `internal/handler/cv_ats_delta.go`: the base CV gets a reason saying it
      is the base; a tailored copy whose vacancy is gone gets a reason saying the vacancy no longer
      exists. Integration tests for both, asserting the reason text distinguishes them — a shared
      409 with the wrong sentence is the defect being fixed, not a cosmetic detail.
- [ ] 4.2 Test that an orphaned copy edited more recently than the base is never used as the
      baseline for a third, live tailored CV's delta.

## 5. Close out

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...`, plus
      `go test -tags=integration ./internal/handler/ ./internal/cv/`. Web is untouched — no wire
      change — but run `pnpm run check` if anything in `web/` moved.
- [ ] 5.2 Record the flag in `internal/cv/AGENTS.md`: what `is_base` means, why the absence of
      `job_id` cannot carry it, and that the invariant is a partial unique index rather than a
      convention.
- [ ] 5.3 Carry the post-flip reconciliation statement (design → Deploy window) into the PR
      description, so whoever releases runs it once after the colour flip.
