# Design — add-job-tracker

## Context

`user_jobs (user_id, job_id, viewed_at, applied_at, PK (user_id, job_id))` is the
established one-row-per-(user,job) interaction model. Writes go through
idempotent upserts (`RecordJobView`, `MarkJobApplied`) addressed by the public
`:slug` and resolved via the slim `GetJobIDBySlug`. The public wire shape of a
job is `jobview.Job`; lists use `{"data": ..., "meta": {...}}` with limit/offset.

## Decisions

### `saved_at` column, not a new table

A bookmark is just another facet of the same (user, job) interaction, exactly
like `applied_at`. `ALTER TABLE user_jobs ADD saved_at timestamptz` keeps the
invariant "at most one interaction per (user, job)" and means the my-jobs
listing is a single join — no UNION over a separate favorites table.

- `SaveJob`: upsert setting `saved_at = now()` (insert path gets `viewed_at`
  via its default, same precedent as `MarkJobApplied`).
- `UnsaveJob`: `UPDATE ... SET saved_at = NULL` — the row stays, so view
  history and an `applied_at` are never lost by unsaving. No row → no-op
  (idempotent DELETE semantics, still 200 with the absent state).

### Save/unsave are `POST`/`DELETE` on the same path

`POST /api/v1/jobs/:slug/save` and `DELETE /api/v1/jobs/:slug/save` mirror the
existing `/view` and `/apply` interaction endpoints: behind `RequireAuth`,
slug-addressed via `GetJobIDBySlug`, returning `{"data": interaction}`. The
DELETE returns the updated interaction too (or a zero-state if no row exists),
so the SPA toggle never needs a second read.

### One listing endpoint with a `filter` enum, counts in `meta`

`GET /api/v1/me/jobs?filter=all|viewed|saved|applied` (default `all`). `/me/...`
is the natural home for user-scoped reads (consistent with `GET
/api/v1/auth/me`). Filters map to predicates on the one table:

- `all` — every interaction row
- `viewed` — view-only rows (`saved_at IS NULL AND applied_at IS NULL`): the
  passive history without the jobs already acted on (a "viewed = every row"
  reading would just duplicate `all`)
- `saved` — `saved_at IS NOT NULL`
- `applied` — `applied_at IS NOT NULL`

The query joins `jobs` and returns rows ordered by
`GREATEST(viewed_at, saved_at, applied_at) DESC` — "most recently touched
first". Closed jobs are NOT filtered out: a user's application history must not
silently shrink when a posting closes; the job view carries `closed_at` and the
SPA renders the closed state.

Response item shape: `{"job": <jobview>, "viewed_at", "saved_at", "applied_at"}`
— the job stays in the shared `jobview.Job` shape (no internal id leaks), the
interaction fields ride alongside rather than being flattened into it.

`meta` carries the standard `total/limit/offset` for the active filter plus
`counts: {all, viewed, saved, applied}` so the SPA renders tab badges without
extra requests. Counts come from one aggregate query
(`COUNT(*) FILTER (WHERE ...)`), not three round-trips.

An unknown `filter` value is a `400` (explicit input contract, matching the
fail-fast style elsewhere).

### Interaction record gains `saved_at`

`interactionResponse` adds `saved_at` (nullable). The SPA already records a
view when a signed-in user opens a job; that response now tells it whether the
job is saved, so the job page Save toggle needs no extra fetch.

### SPA: one page, tabs client-side over the filter param

Route `/my/jobs` (guarded client-side: signed-out users are prompted to sign
in). Tabs All / Saved / Applied drive the `filter` query param; rows reuse
`JobRow` with the interaction timestamps as secondary text and an Applied/Saved
badge. The page is reachable from `UserMenu`. The Save toggle on the job detail
page calls `saveJob`/`unsaveJob` and flips on the returned interaction.

## Risks / Trade-offs

- **Migration on an existing volume**: initdb-only migrations mean dev DBs need
  `docker compose down -v`; prod needs a manual `ALTER`. Accepted — same as the
  `close-stale-jobs` rollout; the migration-runner seam stays open.
- **`all` includes silently recorded views**, which can surprise ("I never
  opened that"… they did, briefly). Accepted: that is literally the view
  history, and the screenshot-style product treats it the same way.
