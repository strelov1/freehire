## Why

Auto-apply already has a working backend for tailoring, candidate review (approve/decline),
and unattended submission (`auto-apply-worker`, `auto-apply-tailored-resume`,
`auto-apply-submit-trigger`, `auto-apply-inngest-orchestration`), but no surface in the SPA
ever calls the review endpoint or shows the candidate where an attempt stands. The only
visible state today is a single button on the job page (`hidden/queued/applied/failed`). A
candidate who starts an auto-apply attempt has no way to approve or decline the tailored CV,
no way to see why an attempt got stuck (a required question the profile could not answer),
and no place to see what auto-apply has recently submitted on their behalf.

## What Changes

- Add `GET /me/auto-apply`, returning the caller's live `auto_apply_queue` entries (with a
  richer status than the job-detail overlay: `tailoring` / `pending_review` / `approved` /
  `blocked` / `declined` / `failed`) plus a capped list of recently completed auto-apply
  submissions, read from the `application_events` ledger.
- A blocked entry's response carries its structured `unmapped` question list (candidate-facing
  by design); it never carries `last_error` (an internal, non-candidate-facing diagnostic).
- Add a new SPA page, `/my/auto-apply`, linked from the account navigation, that lists these
  entries grouped by status, lets the candidate Approve/Decline a `pending_review` entry inline
  (calling the existing `POST /me/auto-apply/:queueId/review`), and shows the blocked question
  list and the recent-submissions section.
- No new database columns and no change to `cmd/auto-apply`, the tailoring/review write paths,
  or the existing job-detail auto-apply button's own 4-state contract.

## Capabilities

### New Capabilities
- `auto-apply-status-list`: the read model behind `GET /me/auto-apply` — the richer,
  list-shaped status derivation over `auto_apply_queue` (distinguishing tailoring, pending
  review, approved-queued, blocked, declined, failed) and the recently-completed view sourced
  from `application_events`.

### Modified Capabilities
<!-- none: the enqueue/tailoring/review/submission behaviors are unchanged, only newly
     surfaced for reading -->

## Impact

- **Backend**: new sqlc queries in `internal/platform/db/queries/auto_apply_queue.sql`
  (list-for-user) and `application_events.sql` (recent auto-applied-for-user); a new use case
  in `internal/application/autoapply`; a new handler file
  `internal/api/handler/auto_apply_list.go` mounting `GET /me/auto-apply` behind `mw.cookie`.
- **Frontend**: new route `web/src/routes/my/auto-apply/+page.svelte`; a new `api.ts` method
  pair (list + review); a new nav entry in `HeaderMenu.svelte`'s `accountLinks`.
- **No impact** to `cmd/auto-apply`, `internal/api/atsapply`, the existing
  `PostAutoApplyTailor`/`PostAutoApplyReview` write paths, or the job-detail auto-apply button.
