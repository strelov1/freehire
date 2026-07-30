## Why

A user's base CV is identified by an **absence** — `cvs.job_id IS NULL` — and `cmd/prune`
manufactures that absence on purpose. `cvs_job_id_fkey` is `ON DELETE SET NULL`
(`migrations/0024_cvs.sql:40`), and the prune query says so in as many words: *"Every other
reference to jobs cascades or nulls, so a user's saved job goes with it. That is an accepted cost
of the campaign, not an oversight."*

So pruning one junk vacancy silently converts its tailored copy into a base CV. Worse, the base is
resolved as the **newest** such row (`GetBaseCVByUser`: `ORDER BY updated_at DESC`), and a
just-tailored orphan is newer than a base CV the candidate last touched weeks ago. The orphan wins.

One predicate, four consequences:

1. **`cv.Store.Tailor` copies the base CV's document into every new tailored copy.** The candidate
   tailors for a new vacancy and gets a copy of the CV they tailored for a *different* one.
2. **The ATS delta compares against the wrong baseline** — silently wrong numbers, no error.
3. **`StartTailorSession` resolves the wrong base.**
4. **The orphan changes category in the UI.** `ListTailoredCVsByUser` inner-joins `jobs`, so the
   orphan drops out of the tailored list (already documented there) while simultaneously appearing
   as the base.

Measured on production today: 68 CVs, 8 with `job_id IS NULL`, 60 tailored, **0 users with more
than one base-looking CV** and 0 rows carrying an orphan signature. The defect is reachable but has
not fired yet — which is exactly when it is cheap to fix, and when the uniqueness constraint below
can still be added without a data cleanup.

## What Changes

- **Add `cvs.is_base boolean NOT NULL DEFAULT false`**, backfilled `true WHERE job_id IS NULL`. A
  CV is a base because it says so, not because a foreign key happens to be empty.
- **Enforce the invariant in the database**: a partial unique index `(user_id) WHERE is_base`. "One
  base CV per user" stops being an `ORDER BY … LIMIT 1` convention and becomes something the
  schema refuses to violate. It builds clean on today's data.
- **Move all four readers off `job_id IS NULL`** onto `is_base`: the base lookup, the tailoring
  seed, the tailor-session bootstrap, and the ATS delta's baseline.
- **Name the orphan case where it is now reachable**: a tailored copy whose vacancy was pruned is
  still a tailored copy, so the ATS delta refuses it as such — with a reason that says the vacancy
  is gone, rather than the misleading "not a tailored CV".

Not in this change, deliberately:

- **Surfacing orphans in the UI.** The tailored list's inner join already drops them, and that was
  a known, documented decision before this change. Making a vacancy-less tailored CV visible and
  labelled needs a denormalised vacancy trace and a card that renders without a company — product
  work, and its own change.
- **Touching the FK.** `RESTRICT` would abort prune campaigns; `CASCADE` would delete a
  candidate's work because we cleaned up a junk posting. Both are worse than the disease.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `cv-tailoring`: the base CV is defined by an explicit flag rather than by a NULL `job_id`, and
  exactly one base per user is a database-enforced invariant rather than a query convention.
- `tailor-ats-delta`: the baseline is the CV flagged as the base; a tailored copy whose vacancy was
  pruned is refused as a tailored copy with no vacancy, not mistaken for a base.

## Impact

- **Schema**: one new column, one partial unique index, one backfill — additive, and applied by
  `release.sh` (which runs `cmd/migrate` before the colour flips).
- **SQL**: `GetBaseCVByUser`, `CreateCV`/`CreateTailoredCV` (set the flag), and the base-CV
  predicate in `internal/db/queries/cvs.sql`; regenerate with `make sqlc`.
- **Go**: `internal/cv/store.go` (`BaseCV`, `Create`, `CreateTailored`, `Tailor`),
  `internal/handler/cv_ats_delta.go` (the conflict reason).
- **No API shape change** and no web change: `is_base` is a server-side predicate, not a wire field.
- **Ordering note**: the migration must land before the code that selects on `is_base`, which is
  the normal `release.sh` order (migrate, then restart the new colour). The old colour keeps
  running the `job_id IS NULL` query against a table that now has an extra column — harmless,
  since sqlc enumerates the columns it knows and the new one is additive.
