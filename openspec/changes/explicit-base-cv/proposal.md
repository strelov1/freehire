## Why

Whether a CV is a tailored copy is inferred from `cvs.job_id IS NOT NULL` — and `cmd/prune` removes
that link on purpose. `cvs_job_id_fkey` is `ON DELETE SET NULL` (`migrations/0024_cvs.sql:40`), and
the prune query says so in as many words: *"Every other reference to jobs cascades or nulls, so a
user's saved job goes with it. That is an accepted cost of the campaign, not an oversight."*

So pruning one junk vacancy silently converts its tailored copy into a plain CV — and the base CV
that tailoring seeds from is the **newest** plain CV (`GetBaseCVByUser`: `ORDER BY updated_at DESC`).
A just-tailored orphan is newer than a CV the candidate last touched weeks ago. The orphan wins.

One inference, four consequences:

1. **`cv.Store.Tailor` copies the base CV's document into every new tailored copy.** The candidate
   tailors for a new vacancy and gets a copy of the CV they tailored for a *different* one.
2. **The ATS delta compares against the wrong baseline** — silently wrong numbers, no error.
3. **`StartTailorSession` resolves the wrong base.**
4. **The orphan changes category in the UI.** `ListTailoredCVsByUser` inner-joins `jobs`, so the
   orphan drops out of the tailored list (already documented there) while entering the plain-CV pool.

Measured on production today: 68 CVs, 8 vacancy-less, 60 tailored, and **zero rows carrying an
orphan signature**. The defect is reachable but has not fired yet, so the fix needs no data repair.

## What Changes

- **Add `cvs.is_tailored boolean NOT NULL DEFAULT false`**, backfilled `true WHERE job_id IS NOT
  NULL`. The flag records what the row was *created* as — precisely the fact the foreign key
  destroys.
- **The base lookup excludes tailored copies** (`NOT is_tailored`) instead of requiring an empty
  vacancy link. Everything else about it is unchanged: "the base" stays the most recently edited
  non-tailored CV.
- **Both creation queries state the kind** rather than leaving it to be inferred.
- **The ATS delta's refusal names which case it hit**: the CV is a base CV, or it is a tailored copy
  whose vacancy no longer exists. Today both produce "not a tailored CV", which is false for the
  second.

Not in this change, deliberately:

- **A uniqueness constraint on base CVs.** An earlier draft of this proposal added
  `UNIQUE (user_id) WHERE is_base`. It is wrong: `cv-builder` requires that a user may *"create,
  list, read, update, and delete multiple CVs"*, and an existing integration test pins that
  behaviour. "The base" is a derived choice among a user's plain CVs, not an identity — which is why
  the flag marks tailored-ness rather than base-ness.
- **Surfacing orphans in the UI.** The tailored list's inner join already drops them — a documented
  decision that predates this change. Making a vacancy-less tailored CV visible and labelled needs a
  denormalised vacancy trace and a card that renders without a company: product work, its own change.
- **Touching the FK.** `RESTRICT` aborts prune campaigns; `CASCADE` deletes a candidate's work
  because we cleaned up a junk posting. Both are worse than the disease.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `cv-tailoring`: tailored-ness is recorded at creation, so a pruned vacancy no longer promotes its
  copy into the pool the base CV is chosen from.
- `tailor-ats-delta`: the baseline is a non-tailored CV; an orphaned copy is refused as a tailored
  CV whose vacancy is gone, not mistaken for a base.

## Impact

- **Schema**: one additive column plus a backfill, applied by `release.sh` (which runs `cmd/migrate`
  before the colour flips).
- **SQL**: `GetBaseCVByUser`, `CreateCV`, `CreateTailoredCV` in `internal/db/queries/cvs.sql`;
  regenerate with `make sqlc`. No Go signature changes — the flag is set in SQL, not passed in.
- **Go**: `internal/handler/cv_ats_delta.go` only, for the split refusal reason.
- **No API shape change and no web change**: `is_tailored` is a server-side predicate.
- **Deploy window**: between the migration and the colour flip, the old code inserts tailored copies
  without the flag, so they default to `false` and would join the base pool. One idempotent statement
  after the flip closes it — carried in tasks.
