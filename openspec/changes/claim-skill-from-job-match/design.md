## Context

The sidebar match block (`web/src/lib/components/JobMatch.svelte`) renders the result of
`GET /api/v1/jobs/:slug/match`, which the backend computes as a pure set operation:
`jobmatch.Compute(job.skills, profile.skills)` plus the curated adjacency dictionary. Both sides
speak the same canonical `skilltag` slugs, and the chips display those slugs verbatim.

Three existing facts decide most of this design:

- **The chip's text is already a valid profile skill.** No dictionary lookup, no free-text entry, no
  new validation — the token under the cursor is exactly what the profile stores.
- **`PUT /api/v1/me/profile` replaces the whole row** (`specializations`, `skills`,
  `excluded_skills`, `location_preferences`) and normalises what it is given: lowercase, trim,
  deduplicate. So "add one skill" is really "save the profile again with one more element", which is
  idempotent but not concurrency-safe from the client's side.
- **The design system has no popover primitive** (`dialog`, `tooltip`, `chip`, and eleven others —
  no floating layer). `ReminderChip.svelte` already solves the same shape by expanding an inline row
  under the chip instead.

No backend work is required. The next `Compute` call reclassifies the claimed skill on its own.

## Goals / Non-Goals

**Goals:**

- Claim a Missing or Close skill into the profile without leaving the job page.
- Feedback is immediate: the chip moves and the percentage changes before the network settles.
- The block never ends up disagreeing with the server about the match.
- Two fast claims both survive.

**Non-Goals:**

- Writing to the CV, the structured résumé, or the experience bank. A profile skill is a filtering
  and matching token, not a claim about employment history — the experience bank's provenance rule
  exists precisely to keep those separate, and nothing here needs it.
- Removing a held skill from the block (You have chips stay inert). Un-claiming lives on
  `/my/profile`, and undo covers the misclick this feature introduces.
- The same affordance on job cards (`JobRow`) or the drawer's teaser. Those chips are a seeded
  teaser, not a match.
- A reusable popover primitive in the design system. One inline disclosure does not establish the
  need for one.

## Decisions

### An inline disclosure row, not a floating popover

Pressing a Missing or Close chip expands a single row directly beneath that chip group:

```
● Missing
[bash] [chatgpt] [confluence] [entra-id] …
┌────────────────────────────────────────┐
│ Do you have entra-id?  [Add to profile]│
└────────────────────────────────────────┘
```

Only one row is open at a time and it names the skill in its own text, so the association with the
pressed chip survives without anchoring; the pressed chip additionally carries a selected ring and
`aria-expanded`.

*Alternative considered:* a floating popover anchored to the chip. It would need a portal, collision
handling against a ~285px sidebar column, outside-click and focus management — a new primitive built
for one call site, against the "note the seam, don't build the infrastructure" rule. The inline row
is what `ReminderChip` already does and what a narrow column can actually fit.

### The optimistic view is a pure overlay in `jobMatch.ts`

`claimSkill(match, skill)` returns a new `JobMatch` with the skill moved out of `missing`/`adjacent`
into `matched`, with `exact_count`, `adjacent_count` and `coverage_percent` recomputed by the
server's own formula — `round((exact + 0.5 × adjacent) / total × 100)`. It is pure and lives beside
`matchBarSegments` and `computeClientMatch`, so it is unit-testable under vitest with no DOM, which
is how the rest of this block's logic is already tested.

The component holds `optimistic: JobMatch | null` and renders `optimistic ?? match`. Rolling back a
failed write is dropping the overlay, not undoing an edit.

*Alternative considered:* re-render only after the refetch. It costs two sequential round-trips
(`PUT`, then `GET`) of dead time on the one interaction whose entire appeal is that it is instant.

### Reconcile against the server after the write lands

The overlay is a strict under-estimate: the client cannot know that claiming `aws` also promotes
`azure` from Missing to Close, because the adjacency dictionary lives in `internal/skilladjacency`.
Leaving it unreconciled means the block shows one match and a reload shows a different one. So on a
successful write the block refetches `GET /jobs/:slug/match` and swaps the overlay for the response.

A failed refetch keeps the overlay: the write succeeded, and reverting would misreport the profile.

Note that the fetch `$effect` tracks only `job.public_slug` and the block state, and the block state
stays `'ready'` across a claim — so the refetch has to be issued explicitly. That is the intent: a
profile write should not be able to accidentally re-trigger the block's initial load.

### The store owns serialisation

`addSkill(skill)` and `removeSkill(skill)` land on `ProfileStore`, not in the component. The store is
the single owner of the profile row, and it is the only place that can guarantee the second write is
built from the first one's result — an internal promise chain (`#queue = #queue.then(...)`) means
each `PUT` reads `#profile` at send time, after its predecessor has applied. Left in the component,
two fast claims each read the pre-claim skills and the second silently drops the first.

`addSkill` also drops the skill from `excluded_skills`: a profile that both claims and avoids the
same token is incoherent, and the user resolved the contradiction by pressing the button. `removeSkill`
subtracts only that one skill, so undoing an earlier claim leaves a later one alone; it does not
restore the exclusion, which would re-create the contradiction the claim resolved.

### Undo is a second write, not a snapshot

Undo calls `removeSkill` and drops the block back to the pre-claim match. Restoring a whole
pre-claim profile snapshot would be wrong the moment a second claim intervened — the snapshot would
silently roll that one back too.

## Risks / Trade-offs

- **A claim is a bare assertion — nothing verifies it.** → It is the same assertion the profile
  editor already accepts by typing, and it only affects the viewer's own matching and filtering. It
  writes nothing that can reach a CV, which is where an unverified claim would actually cost the
  candidate something.
- **The optimistic percentage can be lower than the server's for a moment** (an unlocked adjacency).
  → It resolves on the refetch a moment later, and it errs downward: the number never overstates the
  match and then falls back.
- **Widening the profile makes the viewer's whole feed less selective** — profile skills also feed
  "Apply my profile" in the filter modal and the subscription digests. → It is the same effect as
  editing the profile page, which is the honest outcome of the user saying they have the skill; the
  confirmation names what was added and offers undo.
- **A `PUT` that replaces the row can clobber a profile edited in another tab.** → Pre-existing to
  this endpoint, not introduced here; the claim path reads the store's freshly-loaded copy and adds
  one element to it.
- **Every red chip is now a button, on a block that is mostly red chips.** → The claim row is one
  press away and one press back; nothing is written without the explicit second press on "Add to
  profile".
