## Why

A board card cannot be picked up. `svelte-dnd-action` refuses a drag whose event
target carries a `value` property — a guard written for `<input>`, which a
`<button>` also satisfies, its `value` being `""` rather than `undefined`. #1312
turned the card into a `<div>` carrying a button with `after:inset-0`, a
pseudo-element stretched over the whole card; a pseudo-element is not its own
event target, so every `mousedown` on the card reaches the guard as the button.

Measured, not argued — a spike drove the library's real bundle in a browser:

| grab point | event target | `value` | drag |
|---|---|---|---|
| card body, button overlay | `<button>` | `""` | **refused** |
| badge row (`z-10`) | `<span>` | `undefined` | starts |
| card with no button at all | `<span>` | `undefined` | starts |

The middle row rules out "the library is broken" and puts the cause in our
layout. The third says what fixes it.

Around that repair sit three gaps the same surface has: the board is the only way
to read the applications, there is no way to find one by employer or role, and
the opened application — which has room — offers none of the actions the card is
crowded with.

## What Changes

- **The board card carries no controls.** It drags, and clicking it opens the
  application. The open overlay, the follow-up button and the rehearse button are
  removed; the card becomes one `role="button"` element. This alone repairs the
  drag. The badges stay — they are indicators, not controls.
- **The opened application gains an action row**: Rehearse, Follow up, Analyze,
  Tailor CV. Rehearse and Follow up move here off the card. Rehearse loses its
  stage gate; Follow up keeps its own. Analyze switches to the drawer's existing
  Job Match tab rather than navigating away.
- **A list view** at `/my/tracking/list`, a third tab beside Board and Pipeline.
  One row per application, ordered by last activity, opening the same panel.
- **A search field** over both views, synchronised to `?q=`, filtering the loaded
  rows by employer and role.
- **Write routes addressed by the row the listing served** —
  `PATCH|DELETE /api/v1/me/applications/:id` and
  `DELETE /api/v1/me/applications/:id/stage`. Today the board sends the row id to
  a slug-addressed route, so an application whose posting `cmd/prune` removed
  cannot be moved at all: its id is `a<n>`, and `PATCH /jobs/a<n>/track` is a 404.
- `/my/tracking/[slug]` is renamed `[id]`: it addresses a row, not a posting, and
  has done since #1359.

Not a breaking API change — the existing slug-addressed routes stay for the
`freehire-cli` and `freehire-mcp`, which address postings and have no row ids.

## Capabilities

### New Capabilities

- `tracking-list-view`: reading and searching the tracked applications as a list
  rather than a board — the routed third view, its row, its ordering, and the
  employer/role search that serves both views.

### Modified Capabilities

- `user-job-tracking`: the board card carries no controls and is dragged from
  anywhere on it; the opened application carries the actions; stage, notes and
  removal are writable by the row id the listing served, so an application with
  no posting can be moved.
- `application-silence-signal`: a silent card no longer offers the follow-up
  draft itself — the silence marker reverts to an indicator and the chase is
  offered in the opened application.
- `interview-rehearsal`: the rehearsal is offered in the opened application at
  any stage, not on the card at `screening`/`interview`.

## Impact

- `web/src/lib/components/`: `BoardCard`, `BoardColumn`, `JobBoard`, `JobDrawer`.
- `web/src/routes/my/tracking/`: `+layout.svelte`, new `list/`, `[slug]` → `[id]`.
- `web/src/lib/`: `board.ts` (the search predicate), `rehearsal.ts` (gate
  removed), `api.ts` (the new client calls).
- `internal/handler/user_jobs.go`: the three new routes and the id decoder.
- No migration; no schema change. `ListUserJobs` and the row id's composition are
  deliberately untouched — task 5b.2b of the in-flight `applications-outlive-jobs`
  change records that composition as a decision, and that change is still open.
