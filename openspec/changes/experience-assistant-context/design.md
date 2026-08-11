## Context

See proposal.md — Why, for the motivation. What shapes the approach here:

- `experience.Retrieve` already answers by loading the owner's atoms and doing a linear
  pass over them, on the stated grounds that for a bank of tens-to-hundreds this is
  cheaper and far more predictable than an embedding pipeline. Any read this change adds
  is working with the same data already in hand.
- Retrieval deliberately **drops zero-scoring matches**, because a weak match reads to an
  agent as evidence and evidence licenses a CV bullet. That rule is right for search and
  fatal for fetch-by-key.
- `provenanceFor` decides an atom's provenance by checking the model's quote against the
  candidate's own messages in the transcript, and the interviewer's opening message **is
  recorded as the candidate's message**. Anything written into that message becomes text
  the model can quote back to earn `stated_in_chat`.
- The account shell (`web/src/routes/my/+layout.svelte`) is a centred `max-w-6xl`
  container holding a collapsible nav — `w-56`, or `w-14` collapsed, persisted in
  `localStorage` — beside a `min-w-0 flex-1` content column. The panel currently sits two
  flex rows inside that content column.
- The repo already has the pattern for state shared between a page and the shell around
  it: `auth-dialog.svelte.ts` exposes `openAuthDialog()` as a module-level rune store, and
  the layout consumes it.

## Goals / Non-Goals

**Goals:**

- One read path that serves every surface handing the agent an id, not just the panel.
- An atom that looks the same to the model whichever tool produced it.
- A docked panel that costs the bank no width, at the viewport widths people actually use.

**Non-Goals:**

- Batching or indexing the bank read. The linear owner-scoped pass is the established
  choice and this change gives no reason to revisit it.
- Making the panel resizable, or persisting its width.
- Any change to how a merge picks the atom it keeps.

## Decisions

### A read tool, not an `ids` filter on search

`experience_search` retrieves by meaning and drops what scores zero. Threading ids through
the same path would either need that rule suspended for id queries — a search that
sometimes returns zero-scoring rows, depending on which argument was set — or would
silently drop the very ids the caller named. Fetch-by-key and search-by-meaning want
opposite behaviour from the same code, so they get separate tools.

*Alternative rejected — hydrate the opening message with claim text client-side.* It needs
no backend change and it is the wrong answer twice over. It fixes only the panel, leaving
`?preset=profile&atoms=…`, the duplicate clusters and a merge's returned id exactly as
blind as they are today; and the model still cannot re-read mid-turn. Worse, the opening
message is recorded as the candidate's own, so putting the bank's text into it would let
the model quote that text back and have `provenanceFor` certify it as `stated_in_chat` —
laundering stored text into a fresh candidate assertion. The honest wall depends on the
candidate's messages containing only what the candidate wrote.

### The read is a filter over the owner's atoms, not N point lookups

The handler resolves ids by loading the owner's atoms — the call `Retrieve` already makes —
and selecting the requested set, then attaching employments from the owner's employment
list the way `experienceSummary` already does.

One query rather than up to eight, ownership falls out of the query being owner-scoped
rather than being re-checked per id, and no new store method is needed. The alternative,
looping `GetAtom` per id, adds an N+1 and a per-id ownership check for no gain at this
size.

### Unresolved ids come back named, and ownership is invisible

The result reports which requested ids produced nothing, beside the ones that resolved.
This mirrors `resolveEmployment`, which already answers a bad `employment_id` with the ids
that would have worked rather than a bare refusal — the refusal carries what the retry
needs, instead of costing a round of the turn's budget.

An atom owned by someone else is reported identically to one that does not exist. That is
the convention the codebase already applies to a session the caller does not own, and it
keeps the tool from becoming an existence oracle for other people's rows.

### One atom shape, shared with search

`searchResult` builds its per-atom entry inline. That entry becomes a named function used
by both tools, so a field added for one appears in the other. Two shapes for one concept
is how a model ends up believing a read atom and a searched atom are different kinds of
thing.

### The cap is the tool's, and overflow is reported

One read is bounded for the same reason a search is: the result is replayed into context
every later turn. Rather than coordinating a shared limit between the kickoff builder in
the frontend and the tool in the backend — two constants that would drift — the tool reads
up to its own cap and reports the ids it did not read, so an oversized selection resolves
in a second call instead of silently losing achievements.

### The panel is fixed to the viewport; the shell yields and the nav collapses

The panel becomes `fixed` at the viewport's left edge, below the site header, at the same
offset the selection action bar already pins to. The account shell offsets by the panel's
width while it is open.

The interesting part is where docking is viable at all, because "the bank keeps its width"
is arithmetic, not taste. Closed, at any viewport at or above the cap, the bank gets
`1152 − 224 (nav) − 32 (gap) = 896`. Docked with a 384px panel, the shell has
`viewport − 384` to work with, and the nav is still spending 224 of it. At a 1440 laptop
that leaves 800 — narrower than the 896 it started with, which is the thing this change
exists to stop. Requiring the full width back would push docking out to a ~1536 viewport
and turn the panel into an overlay on most laptops.

So the nav collapses to its icon rail while the panel is docked, reclaiming 168px. At 1440
the bank then gets `1440 − 384 − 56 − 32 = 968` — wider than the 896 it had with the panel
closed. Docking stays viable from roughly a 1400px viewport, and the affordance is one the
layout already has and already persists.

The collapse is an override, not a write: the candidate's stored preference is untouched
and their nav returns to whatever they had chosen when the panel closes.

*Alternative rejected — lift the shell's `max-w-6xl` while docked instead of collapsing the
nav.* It reclaims less (the shell is already at its cap on the viewports in question), and
it makes the account section change width for a reason unrelated to the section.

### The open state travels through a module store

The panel is opened from inside the Experience tab; the offset must be applied by the
account layout, across a route boundary a page cannot pass props through. A small
module-level rune store carries the open state and the panel width, set by the experience
view and read by the layout — the pattern `auth-dialog.svelte.ts` already establishes for a
dialog opened from many places and rendered by the shell.

This is also the seam the account-wide dock would grow from, which is exactly why it stays
a seam: the store holds open state, not a per-page launch contract.

## Risks / Trade-offs

- **A tool that reads ids makes blind merging easier to reach, not harder.** The agent
  could still merge without reading. → The prompt and the opening message both instruct it
  to read first, and the spec's scenario is that a proposed merge states what the two
  achievements say. This is a behavioural requirement covered by prompt-level tests, not a
  mechanical gate; a mechanical one would mean tracking per-atom reads through the turn
  loop, which is disproportionate to the harm of a merge the candidate is asked to confirm
  anyway.
- **Refining blind destroys less than first assumed, but still destroys.** `experience_update`
  loads the atom and overwrites only the fields the call carries, so an unmentioned field
  survives — the hazard is not "everything is lost". It is that `metrics` and `skills` are
  set as whole lists rather than appended to, so an agent adding one newly learned metric
  to an achievement it never read replaces the metrics already recorded with that single
  one. → The prompt instruction is the same either way: read before you write.
- **Below ~1400px the panel becomes an overlay where it used to dock at `xl` (1280).** On
  a 1280–1400 viewport the candidate loses the side-by-side view they have today. → That
  view is the cramped one this change is fixing; at that width there is no arrangement
  where both columns are comfortable. The threshold is a single documented number and can
  be lowered if the overlay proves worse in practice.
- **The panel reaches outside the component that owns it to offset a shell it does not
  own.** That coupling rots if it spreads. → It is confined to one module store with two
  fields, and the layout reads it in exactly one place.
- **Auto-collapsing the nav could look like the product forgetting a preference.** → The
  override never writes to `localStorage`, and the nav restores on close, so the only way
  to notice is the animation itself.

## Open Questions

- Whether the docked panel's width should eventually be draggable. It does not change the
  specs, the approach or the tasks — the width is already one value in the store — so it
  can wait for a complaint.
