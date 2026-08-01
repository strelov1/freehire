## Context

The tracking board's cards cannot be picked up. The cause was found by reading
`svelte-dnd-action`'s bundle and confirmed by a spike that drove that bundle in a
browser — `dndzone` is a plain Svelte action, so the experiment was one HTML page
with no build and no database.

The library refuses a drag whose event target carries a `value` property
(`dist/index.js:1920`):

```js
if (e.target !== e.currentTarget && (e.target.value !== undefined || e.target.isContentEditable)) return;
```

`e.currentTarget` is the wrapper `<div>` the column puts around each card
(`BoardColumn.svelte:52`). `e.target` is the card's inner `<button>`, whose
`value` is `""` — the guard was written for `<input>` and catches a button too.
The button carries `after:absolute after:inset-0` (`BoardCard.svelte:51`), a
pseudo-element stretched over the whole card so a click anywhere opens the
application; a pseudo-element is not its own event target, so every `mousedown`
lands on the button.

Measured:

| grab point | event target | `value` | drag |
|---|---|---|---|
| card body, button overlay | `<button>` | `""` | **refused** |
| badge row (`z-10`) | `<span>` | `undefined` | starts |
| card with no button at all | `<span>` | `undefined` | starts |

The middle row is the one worth having: it rules out "the library is broken" and
puts the cause in our own layout. Introduced by #1312, which turned the card from
a single `<button>` into a `<div>` carrying a stretched button.

The surface has three other gaps: the board is the only way to read the
applications, there is no search, and the opened application offers none of the
actions the card is crowded with.

## Goals / Non-Goals

**Goals:**

- A card drags from anywhere on it.
- Every action on an application is reachable from the opened application.
- The applications are readable as a list and findable by employer or role.
- An application whose posting `cmd/prune` removed can be moved on the board.

**Non-Goals:**

- Changing the composition of the listing's row id. Task 5b.2b of the in-flight
  `applications-outlive-jobs` change records it as a decision, that change is
  open, and a worktree is active against it.
- Server-side search. The tracking listing is capped at 500 rows, which nobody
  reaches.
- Repairing the Emails tab for a posting-less application. It loads through
  `GetTrackedApplication`, which is slug-addressed, so those applications show no
  linked mail even though they can hold some. Real, and it belongs to
  `applications-outlive-jobs` group 6.
- A migration. Nothing here changes the schema.

## Decisions

### The card carries no controls, instead of gaining a drag handle

An earlier draft added a grip handle and held `dragDisabled` until a `mousedown`
on it. The spike showed that would have been solving a problem that disappears on
its own: with no button under the pointer the event target is a `<span>` or the
card element, and the guard passes everywhere.

So `BoardCard` loses its `<button>` elements — the open overlay, the follow-up
button, the rehearse button — and becomes one `role="button" tabindex="0"`
element with a click and a keyboard handler. `role="button"` is an attribute, not
an element, so the node has no `value` property and cannot re-trigger the guard.
The library suppresses the click that ends a real drag, so opening and dragging do
not collide.

This is also the smaller change, and it matches what a card is for. **Alternative
considered:** keep the overlay and add the handle — rejected as the strictly
larger change that leaves the trap in place for whoever adds the next control.
The library does export `dragHandle`/`dragHandleZone`, so a handle remains cheap
if one is ever wanted.

**Trade-off, stated because it is a real loss:** #1312 deliberately turned the
silence badge into a next step. It reverts to an indicator, and the chase costs
one click more.

### Writes addressed by the row the listing served

`TrackedJob.ID` is not one thing. `internal/jobtracking/repository.go:209` sets
it to the posting's public slug for an ordinary row; `:243` sets it to
`"a" + applications.id` for an application whose posting was pruned. The board
sends that value to `PATCH /api/v1/jobs/:slug/track`, so a posting-less
application's stage change 404s and the card reverts.

New routes under `/api/v1/me/applications/:id`, taking the row id in either form,
resolved by one documented decoder. A malformed id answers `404` with the body a
missing one produces — the rule the opaque-id swap set for CVs.

**Alternatives considered.** *Uniform application ids* would delete the decoder,
and `ListUserJobs` already `LEFT JOIN`s `applications`, so exposing `a.id` is one
line — but it rewrites a decision recorded in an unfinished change with an active
worktree. *Mapping back to `job.public_slug` client-side* is a one-file change but
leaves the posting-less application unwritable, which is the bug.

**Namespace.** `/me/applications` rather than `/me/tracking/:id`, because
`internal/handler/gmail.go:101` already mounts `GET /me/tracking/:slug` addressed
by a posting slug. One path segment meaning different things by method is a trap.

A counting id is right here: applications never cross an ownership boundary —
every read and write is scoped to the calling user — so the UUID rule, which
exists for resources served to somebody else, does not apply.

### Analyze switches tabs rather than navigating

`JobDrawer` already renders `JobMatch` + `MatchAnalysisFull` in its Job Match tab.
Leaving the panel to show something the panel contains would be a strange trade,
and would lose the open application. Tailor CV does navigate, to `/tailor/[slug]`,
because that surface owns its own bootstrap.

### The list view is a route, not a toggle

`/my/tracking/list`, a third tab, following the routed-tab pattern the layout
already uses for Pipeline — so the view is linkable and survives a reload, and
lands in SSR. A `localStorage` toggle would do none of that.

`/my/tracking/[slug]` is renamed `[id]`: it addresses a row, not a posting, and
has done since #1359. The static `list` segment wins over the dynamic one in
SvelteKit, so the rename does not shadow it.

### Search filters loaded rows

One field over both views, synchronised to `?q=`. The predicate lives in
`web/src/lib/board.ts` beside `columnOf` and is unit-tested. It has to answer for
the posting-less row, whose employer is known only as a slug.

## Risks / Trade-offs

- **The repair is invisible to every existing test.** → The spike's page is gone,
  but its measurement is recorded above, and the change is confirmed by hand in a
  browser before it is called done. A unit test cannot see a CSS overlay.
- **Removing the card's follow-up button costs a click on the surface that
  feature was built for.** → Accepted, and recorded in the silence spec rather
  than left implicit. The alternative is keeping a control that makes the board's
  primary gesture impossible.
- **The id decoder is a wart.** → Small, one place, documented, and it dies for
  free whenever `applications-outlive-jobs` closes and the id can be made
  uniform. Chosen over editing that change's recorded decision underneath it.
- **`role="button"` on a div is weaker than a `<button>`.** → The keyboard
  handler (Enter/Space) and `tabindex="0"` are part of the task, not an
  afterthought; a card that cannot be opened from the keyboard is a regression.

## Migration Plan

Deploy as one unit. No schema change, no migration, no worker. The slug-addressed
routes stay registered, so `freehire-cli` and `freehire-mcp` are unaffected and no
client needs to move. Rollback is code-only.

## Open Questions

None blocking. The one deliberately deferred item is the Emails tab for a
posting-less application, which belongs to `applications-outlive-jobs` group 6.
