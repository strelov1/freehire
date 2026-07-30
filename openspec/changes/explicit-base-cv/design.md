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

A user may own several plain CVs, so that lookup is a *choice among* them — which is what makes an
orphan joining the pool a defect rather than a mere mislabel.

Production today: 68 CVs, 8 vacancy-less, 60 tailored, and no row carrying an orphan signature. The
hazard has not fired, so the backfill can read `job_id` and be right about every existing row — which
it could not do once a single prune has run.

## Goals / Non-Goals

**Goals:**

- A CV is a tailored copy because it was created as one, and stays one when its vacancy is deleted.
- Deleting a vacancy leaves its tailored copy a tailored copy.
- The ATS delta's refusal tells the caller which of the two no-comparison cases it hit.

**Non-Goals:**

- Making orphaned tailored CVs visible or labelled in the UI. `ListTailoredCVsByUser` inner-joins
  `jobs` and already drops them — a documented decision that predates this change. Surfacing them
  needs a denormalised vacancy trace and a card that renders without a company: product work, its
  own change.
- A uniqueness constraint on base CVs — see the corrected decision below; users legitimately own
  several plain CVs.
- Changing `cvs_job_id_fkey`. `RESTRICT` aborts prune campaigns; `CASCADE` deletes a candidate's
  work because we cleaned up a junk posting.
- Any wire-shape change. `is_tailored` is a server-side predicate; no client learns about it.

## Decisions

### An explicit `is_tailored` flag, not a cleverer query

**Chosen:** `cvs.is_tailored boolean NOT NULL DEFAULT false`, backfilled `true WHERE job_id IS NOT
NULL`. The flag records what the row was created as, which is exactly the fact `ON DELETE SET NULL`
destroys.

**Alternative rejected:** keep inferring, but exclude orphans by their traces — an orphan usually
carries `agent_session_id` or `autopilot_report`. It fails on the case that matters: a tailored CV
that was never opened in the workspace carries neither, and is indistinguishable from a base. An
inference that is right most of the time is what this change exists to remove.

**Alternative rejected:** a `kind text` enum (`'base' | 'tailored'`). Same information, more
surface: a CHECK constraint, a Go enum, and a decode path, to express one bit.

### The flag marks tailored-ness, NOT base-ness — corrected mid-implementation

The first draft of this design added `is_base` plus a partial unique index
`(user_id) WHERE is_base`, on the theory that "one base CV per user" was an invariant the schema
should hold. **That was wrong, and an existing integration test proved it**
(`TestBaseCVAndTailoredCopy`, which creates two plain CVs and asserts the newest is the base): the
index made the second `POST /me/cvs` fail with a uniqueness violation. `cv-builder` requires that a
user may "create, list, read, update, and delete multiple CVs".

"The base" is a *derived* choice — the most recently edited non-tailored CV — not an identity, so it
carries no uniqueness. Marking tailored-ness instead keeps that derivation exactly as it was and
removes only the orphan from the pool. It is also the smaller change: no constraint, no data repair,
nothing for a second base to collide with.

### The deploy window is closed by a reconciliation, not by code

`release.sh` applies migrations **before** the new colour starts, and the old colour keeps serving
until nginx flips — roughly two and a half minutes on the last release. In that window the old code
inserts tailored copies without setting `is_tailored`, so the column defaults to `false` and those
copies join the pool the base is chosen from — the very defect this change removes, for rows created
in those two minutes.

**Chosen:** a post-flip reconciliation statement, run once as part of the release:

```sql
UPDATE cvs SET is_tailored = true WHERE job_id IS NOT NULL AND NOT is_tailored;
```

Idempotent and safe to run at any time. It cannot mislabel anything: a row still holding a vacancy
link was created as a tailored copy by definition.

**Alternative rejected:** a transitional fallback inside `GetBaseCVByUser` (`NOT is_tailored AND
job_id IS NULL`). It works, but it leaves the exact inference this change removes sitting in the
code, plus a follow-up change to delete it — and the window it protects is two minutes on a service
with 68 CVs in total.

### The delta names which no-comparison case it hit

`GetCVATSDelta` currently refuses anything with `JobID == 0` as "not a tailored CV". After this
change that sentence is wrong for an orphan, which *is* a tailored CV. The refusal splits: the base
CV gets "this is your base CV"; a tailored copy whose vacancy is gone gets "the vacancy for this CV
no longer exists". Same 409, an honest reason — a caller cannot act on a message that describes the
wrong situation.

## Risks / Trade-offs

- **A row orphaned BEFORE this migration cannot be recovered** → The backfill reads `job_id`, which
  an already-pruned copy no longer has, so such a row stays flagged non-tailored. Production carries
  none today, which is why this change is cheap now and would need a heuristic repair later.
- **Migration number collision** → `migrations/` on main already carries two `0056_*` and two
  `0057_*` files from parallel branches. `cmd/migrate` keys on the filename, so duplicates coexist,
  but the next number is genuinely ambiguous. Take `0058_` and, if another branch takes it first,
  renumber before merge rather than after — a migration already applied under one name cannot be
  renamed.
- **The reconciliation is a manual release step** → If it is forgotten, the only rows affected are
  tailored copies created inside the flip window, and they stay wrong only until someone runs the
  statement — which is safe at any time. Recorded in tasks so it travels with the change.
- **Every CV-creating path must state the kind** → There are two (`CreateCV`, `CreateTailoredCV`),
  and the flag is written in SQL rather than passed from Go, so a new path cannot forget to thread a
  parameter — it either uses one of these queries or writes its own INSERT, which review would catch.

## Migration Plan

1. `0058_cvs_is_tailored.sql`: add the column and backfill `true WHERE job_id IS NOT NULL`.
   Additive — the running old colour is unaffected, since sqlc enumerates the columns it knows.
2. `make sqlc` after the query changes; deploy through `release.sh`, which applies the migration
   before flipping.
3. Run the reconciliation statement once after the flip.

Rollback: the code change reverts cleanly and the column can stay — it is inert to the old
predicate. Nothing needs undoing in the data.
