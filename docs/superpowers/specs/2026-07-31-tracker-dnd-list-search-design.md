# Tracker: repair the drag, add a list view and a search, move the actions into the application

Date: 2026-07-31
Status: approved, ready for an implementation plan

## Why

Four complaints about `/my/tracking`, which turn out to be one repair and three
additions:

1. A board card cannot be picked up at all.
2. The interview-rehearsal button lives on the card, where it competes for room
   with the follow-up button and the badges, and is absent from the opened
   application, where there is space for it.
3. There is no way to read the board as a plain list.
4. There is no way to find an application by employer or role.

## The defect

`svelte-dnd-action` refuses to start a drag whose event target looks like a form
field (`web/node_modules/svelte-dnd-action/dist/index.js:1920`):

```js
if (e.target !== e.currentTarget && (e.target.value !== undefined || e.target.isContentEditable)) return;
```

`e.currentTarget` is the wrapper `<div>` the column puts around each card
(`web/src/lib/components/BoardColumn.svelte:52`). `e.target` is the card's inner
`<button>`. A `<button>` has a `value` IDL attribute — the empty string, not
`undefined` — so the guard, written for `<input>`, catches it.

What makes the guard fire everywhere is the layout. That button carries
`after:absolute after:inset-0` (`web/src/lib/components/BoardCard.svelte:51`), a
pseudo-element stretched across the whole card so a click anywhere opens the
application. A pseudo-element is not its own event target: events over it target
the button. So a `mousedown` anywhere on the card reaches the guard and the drag
is refused.

Introduced by #1312, which turned the card from a single `<button>` into a
`<div>` carrying a stretched button. Before it, `e.target` was a `<span>` — no
`value` — and the drag started.

Falsifiable prediction, and the cheapest confirmation: the card can still be
dragged by its badge row, which sits on `relative z-10`, above the overlay, and
whose `<span>` passes the guard.

**Confirmed by spike**, against the real `dist/index.mjs` driven in a browser —
`dndzone` is a plain action, so the whole experiment was one HTML page with no
build and no database:

| grab point | event target | `value` | drag |
|---|---|---|---|
| card body, button overlay | `<button>` | `""` | **refused** |
| badge row (`z-10`) | `<span>` | `undefined` | **starts** |
| card with no button at all | `<span>` | `undefined` | **starts** |

The middle row is the one that matters: it rules out "the library is broken" and
pins the cause on the element under the pointer.

## Design

### 1. Writes addressed by the row the listing served

`TrackedJob.ID` is not one thing. `internal/jobtracking/repository.go:209` sets
it to the posting's public slug for an ordinary row;
`internal/jobtracking/repository.go:243` sets it to `"a" + applications.id` for
an application whose posting `cmd/prune` removed. The board sends that value to
`api.trackJob`, which is `PATCH /api/v1/jobs/:slug/track` — so a posting-less
application's stage change 404s and the card reverts.

New routes, addressed by the row id exactly as the listing served it:

```
PATCH  /api/v1/me/applications/:id          {stage?, notes?}
DELETE /api/v1/me/applications/:id          remove from the board
DELETE /api/v1/me/applications/:id/stage    drop progress, keep the bookmark
```

A single documented decoder resolves the two forms: `a<digits>` is an
`applications.id`; anything else is a posting slug resolved the way the existing
handlers resolve it. A malformed id answers 404 with the body a missing one
produces — the same rule the opaque-id swap set for CVs.

Its own namespace rather than `/me/tracking/:id`, because
`internal/handler/gmail.go:101` already mounts `GET /me/tracking/:slug` addressed
by a posting slug. One path segment meaning different things depending on the
method is a trap for the next reader.

**Not** changing the composition of `TrackedJob.ID`. Uniform application ids
would remove the decoder, but task 5b.2b of the in-flight OpenSpec change
`applications-outlive-jobs` records the current composition as a decision, and
that change is unfinished (groups 6–10 open) with an active worktree against it.
The decoder is the smaller, local cost.

A counting id is right here. Applications never cross an ownership boundary —
every read and write is scoped to the calling user — so the UUID rule, which
exists for resources served to somebody else, does not apply. No migration.

### 2. A card carries no buttons

The card is a card, not a control panel: it drags, and clicking it opens the
application. Every action moves into the opened application (§5).

So `BoardCard` loses its `<button>` elements — the stretched open overlay, the
follow-up button and the rehearse button alike — and becomes one
`role="button" tabindex="0"` element with a click and a keyboard handler. The
badges stay: `Interview`, `24d`, the mail count and the notes mark are
indicators, not controls, and none of them is a `<button>`.

That alone repairs the drag, as the spike measured: with no button under the
pointer the event target is a `<span>` or the card element itself, and the
library's guard passes everywhere on the card. **No drag handle is needed** —
an earlier draft of this design added one, and the spike showed it would have
been solving a problem that the button removal already solves.

`role="button"` is an attribute, not an element, so the node has no `value`
property and cannot re-trigger the guard. The library suppresses the click that
ends a real drag, so opening and dragging do not collide.

The silence badge is the one loss to weigh: it is currently the entry point to
the follow-up dialog (#1312 made it "a next step"). It reverts to a plain
indicator, and the chase is reached by opening the application.

### 3. A list view

A third tab at `/my/tracking/list`, beside Board and Pipeline, following the
routed-tab pattern the layout already uses so the view is linkable and survives a
reload.

One row per application: logo, employer, role, a stage select, days silent, mail
count. Ordered by last activity. A row opens the same `JobDrawer` the board
opens. Fed by the same server `load` as the board — no second fetch path.

The dynamic segment `/my/tracking/[slug]` is renamed `[id]`: it addresses a row,
not a posting, and has done since #1359.

### 4. Search

One field above both views, synchronised to `?q=` in the URL. It filters rows
already loaded rather than asking the server: the listing is capped at 500 rows,
which nobody reaches, and a round trip per keystroke buys nothing.

The predicate lives in `web/src/lib/board.ts` beside `columnOf`, and is covered by
vitest. It has to answer for the posting-less row, whose employer is known only as
a slug.

### 5. The application's actions

A row in `JobDrawer`'s header, below the meta pills, above the tabs — so it shows
on every tab and does not fight `View job` for the corner.

- **Rehearse** moves off the card and is offered at any stage. The stage gate in
  `web/src/lib/rehearsal.ts` existed because a card carries few controls; an
  opened application does not have that problem, and the server has never gated a
  rehearsal.
- **Follow up** moves off the card too, and keeps its own gate
  (`canFollowUp` in `web/src/lib/followup.ts`) — chasing an employer who answered
  yesterday is not an offer worth making. It opens the existing `FollowUpDialog`.
- **Analyze** switches to the existing Job Match tab, which already renders
  `JobMatch` + `MatchAnalysisFull`. It does not navigate: leaving the drawer to
  show something the drawer contains would be a strange trade.
- **Tailor CV** navigates to `/tailor/[slug]`.

Rehearse, Analyze and Tailor CV need a posting. With `item.job === null` they are
absent, not disabled — the same treatment `View job` already gets two lines
above. Follow up does not need one: the chase is addressed to the employer, which
the application knows by itself.

## Out of scope

`GetTrackedApplication` is addressed by posting slug, so the drawer's Emails tab
loads nothing for a posting-less application (`JobDrawer.svelte:70` returns
early). Those applications can hold linked mail, so this is a real gap — but it
belongs to the mail cutover in `applications-outlive-jobs` group 6, not here.

## Verification

- Unit: the search predicate, over rows with and without a posting.
- Unit: the id decoder, both forms plus a malformed id.
- Integration: a stage change on a posting-less application persists.
- By hand, in a browser: a card drags from anywhere on it, and a click still
  opens the application. The defect was invisible to every existing test and has
  to be confirmed the way it was found.
