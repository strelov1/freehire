## Context

`cvs.job_id` carries two meanings at once: *which vacancy this CV is for* and *whether this CV is
the base*. The second meaning is an inference from the first being absent, and `cmd/prune` produces
that absence deliberately — `ON DELETE SET NULL` on `cvs_job_id_fkey`, with the prune query stating
outright that nulling references is an accepted cost of the campaign.

Four readers depend on the inference:

| Reader | What it does with "the base" |
|---|---|
| `cv.Store.Tailor` (`store.go:306`) | copies its **document** into every new tailored CV |
| `cvHandlers.GetCVATSDelta` (`cv_ats_delta.go:58`) | scores it as the comparison baseline |
| `cvHandlers.StartTailorSession` (`cv_tailor.go:156`) | resolves it to re-establish a workspace |
| `GetBaseCVByUser` (`cvs.sql:67`) | `job_id IS NULL … ORDER BY updated_at DESC LIMIT 1` |

Production today: 68 CVs, 8 vacancy-less, 60 tailored, zero users with more than one vacancy-less
CV. The hazard has not fired, so the fix needs no data repair and the uniqueness constraint can go
in as part of the same migration.

## Goals / Non-Goals

**Goals:**

- A CV is the base because it is marked as the base, and the database refuses a second one.
- Deleting a vacancy leaves its tailored copy a tailored copy.
- The ATS delta's refusal tells the caller which of the two no-comparison cases it hit.

**Non-Goals:**

- Making orphaned tailored CVs visible or labelled in the UI. `ListTailoredCVsByUser` inner-joins
  `jobs` and already drops them — a documented decision that predates this change. Surfacing them
  needs a denormalised vacancy trace and a card that renders without a company: product work, its
  own change.
- Changing `cvs_job_id_fkey`. `RESTRICT` aborts prune campaigns; `CASCADE` deletes a candidate's
  work because we cleaned up a junk posting.
- Any wire-shape change. `is_base` is a server-side predicate; no client learns about it.

## Decisions

### An explicit `is_base` flag, not a cleverer query

**Chosen:** `cvs.is_base boolean NOT NULL DEFAULT false`, backfilled `true WHERE job_id IS NULL`.

**Alternative rejected:** keep inferring, but exclude orphans by their traces — an orphan usually
carries `agent_session_id` or `autopilot_report`. It fails on the case that matters: a tailored CV
that was never opened in the workspace carries neither, and is indistinguishable from a base. An
inference that is right most of the time is what this change exists to remove.

**Alternative rejected:** a `kind text` enum (`'base' | 'tailored'`). Same information, more
surface: a CHECK constraint, a Go enum, and a decode path, to express one bit.

### The invariant lives in the schema

A partial unique index — `CREATE UNIQUE INDEX … ON cvs (user_id) WHERE is_base` — makes "one base
per user" something the database refuses to break. Today it is an `ORDER BY updated_at DESC LIMIT 1`
convention, which cannot fail loudly; it just picks one. It builds clean on current data (verified
above), and after it exists the `ORDER BY` in the base lookup becomes belt-and-braces rather than
the mechanism.

### The deploy window is closed by a reconciliation, not by code

`release.sh` applies migrations **before** the new colour starts, and the old colour keeps serving
until nginx flips — roughly two and a half minutes on the last release. In that window the old code
inserts base CVs without setting `is_base`, so the column defaults to `false` and that user's base
would be invisible to the new code: the next tailoring would seed a *second* base from their résumé,
and the partial unique index would then have two rows to disagree about (it would reject the second
insert, surfacing as a failed bootstrap).

**Chosen:** a post-flip reconciliation statement, run once as part of the release:

```sql
UPDATE cvs SET is_base = true
WHERE job_id IS NULL AND NOT is_base
  AND NOT EXISTS (SELECT 1 FROM cvs c2 WHERE c2.user_id = cvs.user_id AND c2.is_base);
```

Idempotent, safe to run at any time, and it cannot create a second base because of its own
`NOT EXISTS` guard. One statement beats the alternative.

**Alternative rejected:** a transitional fallback inside `GetBaseCVByUser` (`is_base OR (job_id IS
NULL AND the user has no is_base row)`). It works, but it leaves the exact inference this change
removes sitting in the code, plus a follow-up change to delete it — and the window it protects is
two minutes on a service with 68 CVs in total.

### The delta names which no-comparison case it hit

`GetCVATSDelta` currently refuses anything with `JobID == 0` as "not a tailored CV". After this
change that sentence is wrong for an orphan, which *is* a tailored CV. The refusal splits: the base
CV gets "this is your base CV"; a tailored copy whose vacancy is gone gets "the vacancy for this CV
no longer exists". Same 409, an honest reason — a caller cannot act on a message that describes the
wrong situation.

## Risks / Trade-offs

- **Migration number collision** → `migrations/` on main already carries two `0056_*` and two
  `0057_*` files from parallel branches. `cmd/migrate` keys on the filename, so duplicates coexist,
  but the next number is genuinely ambiguous. Take `0058_` and, if another branch takes it first,
  renumber before merge rather than after — a migration already applied under one name cannot be
  renamed.
- **The reconciliation is a manual release step** → If it is forgotten, the only users affected are
  those who created a base CV inside the flip window; the symptom is a failed tailoring bootstrap,
  not silent corruption, and running the statement later fixes it. Recorded in tasks so it travels
  with the change.
- **`Create` now decides `is_base`** → Every CV-creating path must say which kind it is. There are
  two (`Create`, `CreateTailored`), and the unique index catches a third that gets it wrong.

## Migration Plan

1. `0058_cvs_is_base.sql`: add the column, backfill `true WHERE job_id IS NULL`, create the partial
   unique index. Additive — the running old colour is unaffected, since sqlc enumerates the columns
   it knows.
2. `make sqlc` after the query changes; deploy through `release.sh`, which applies the migration
   before flipping.
3. Run the reconciliation statement once after the flip.

Rollback: the code change reverts cleanly; the column and index can stay (they are inert to the old
predicate). Nothing needs undoing in the data.
