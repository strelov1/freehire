## Why

Signed-in users browsing the jobs feed have no way to say "not this one — stop
showing it to me." Irrelevant postings (wrong role, a company they've ruled out,
a repeat they've already judged) keep occupying the feed. The swipe deck already
lets a user dismiss a job, and the whole backend for it — the `dismissed_at`
mark, the `POST/DELETE /jobs/:slug/dismiss` endpoints — already exists. This
change brings that same "hide" gesture to the ordinary browse list and gives the
user a place to review and undo it.

## What Changes

- Add a **hide control** to the job card (`JobRow`) for authenticated users: an
  eye-off icon revealed on hover that dismisses the job via the existing
  `POST /jobs/:slug/dismiss` endpoint.
- **Hidden jobs drop out of the browse feed** client-side, mirroring the
  existing viewed/saved slug cross-reference (no server-side Meili filtering).
- Hiding a card makes it **vanish immediately with a transient "Job hidden —
  Undo" toast**; Undo restores it via `DELETE /jobs/:slug/dismiss`.
- Add a **dismissed-slugs endpoint** (`GET /me/tracking/dismissed`) so the SPA
  knows which feed cards to exclude, mirroring `GET /me/tracking/saved`.
- Add a **"Hidden" tab** to the Activity surface listing dismissed jobs, each
  with an un-hide action, backed by a new `dismissed` filter on the tracking
  list.

## Capabilities

### New Capabilities

- `hidden-jobs`: Let an authenticated user hide a job from the browse feed and
  manage the set of hidden jobs. Covers the card hide control, the client-side
  feed exclusion with undo, the dismissed-slugs cross-reference endpoint, and the
  Activity "Hidden" tab with un-hide. Reuses the existing dismiss interaction
  (`user_jobs.dismissed_at`, `POST/DELETE /jobs/:slug/dismiss`) owned by
  `job-swipe`; that endpoint's contract is unchanged.

### Modified Capabilities

<!-- None: the dismiss endpoint's contract is unchanged; this change only adds
     new read surfaces and UI on top of the existing dismiss mark. -->

## Impact

- **Backend**: new `ListDismissedJobSlugs` query + `DismissedSlugs` tracking
  method + `GET /me/tracking/dismissed` handler and route; new `dismissed`
  filter branch in `ListUserJobs`/`CountUserJobs` and the `MyJobsFilter` domain
  vocabulary.
- **Frontend**: `JobRow.svelte` (hide control), new `dismissedJobs.svelte.ts`
  store, `JobsView.svelte` (feed exclusion + undo toast), new
  `my/activity/hidden/+page.svelte` + `Hidden.svelte`, a fourth tab in
  `my/activity/+layout.svelte`, and `api.ts`/`types.ts` (`listDismissedSlugs`,
  `dismissed` filter value).
- **Reused unchanged**: `dismissed_at` column, `DismissJob`/`UndismissJob`
  queries, `POST/DELETE /jobs/:slug/dismiss` endpoints, `api.dismissJob`/
  `api.undismissJob`.
