## Why

On the job page the ghost signal is stated twice — once as a chip under the title, then again as a
bordered panel listing all four criteria, two of which usually read "No data". A reader deciding
whether the posting is worth an hour of unpaid work meets a fourteen-line block where a single line
would do, and half of it says nothing. The panel earns its space only for the reader who wants the
justification; for everybody else it is the loudest thing on the page after the title.

## What Changes

- Replace the always-open panel with a single row: a four-segment gauge, the hedged level, the
  `fired/total` scale, and a `Details` disclosure that expands the existing checklist in place.
- The gauge fills with risk, not health: only fired criteria are coloured, and the tone escalates
  with the count (one amber → four red). Unfilled segments stay neutral grey and mean *not
  observed*, never *checked and clear* — a distinction the served payload cannot make, so the
  interface must not imply it.
- The row takes the chip's position under the title, and the separate bordered section before the
  description is removed. One statement of the signal on the page instead of two.
- List cards keep the existing text chip. In a row of `remote`/`salary` chips a gauge breaks the
  rhythm, and a card reader is scanning, not adjudicating.
- The hedged wording, the explicit "no data" rows, and the "observations about the posting, not the
  employer" caveat all survive — they move behind the disclosure rather than disappearing.

## Capabilities

### New Capabilities

None. This changes how an existing signal is presented, not what the system observes.

### Modified Capabilities

- `ghost-job-signal`: the interface requirement changes from "chip plus a full checklist on the job
  page" to "a gauge row whose checklist is disclosed on demand", and adds the rule that an unfilled
  segment carries no claim about the criterion behind it.

## Impact

- `web/src/lib/components/GhostChecklist.svelte` — becomes the gauge row plus disclosure.
- `web/src/lib/components/GhostBadge.svelte` — unchanged; keeps serving list cards.
- `web/src/lib/components/JobView.svelte` — the ghost row moves to the header slot where the chip
  sits today, and the section before the description goes away.
- `web/src/lib/ghost.ts` — gains the gauge projection (segment count, filled count, tone) beside the
  existing `ghostBadge` / `ghostChecklist`; both keep their current contracts.
- No backend, contract, or database change. `Ghost` is served exactly as it is today.
