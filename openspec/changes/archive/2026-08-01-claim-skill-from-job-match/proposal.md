## Why

The sidebar match block tells a signed-in viewer which of a job's skills they are missing, and
stops there. The most common reason a skill reads as missing is not that the candidate lacks it —
it is that the profile never learned it: the skill set was seeded from a CV parse or typed once at
sign-up and has not been touched since. Today fixing that means leaving the job, opening
`/my/profile`, finding the skill in a picker, saving, and navigating back — so nobody does it, and
every subsequent match, card bar and filter stays wrong for the same missing token.

The block already names the exact skill and the exact moment the candidate notices the gap. Letting
them claim it in place turns a dead-end red chip into the cheapest profile-completion prompt the
product has.

## What Changes

- A **Missing** or **Close** chip in the sidebar match block becomes an activatable control. Pressing
  it expands an inline claim row directly under its group — "Do you have `<skill>`?" plus an **Add to
  profile** action — following the existing `ReminderChip` disclosure pattern rather than introducing
  a floating popover the design system does not have.
- Confirming writes the skill into the caller's profile skill set (`PUT /api/v1/me/profile`) and
  nothing else: no CV edit, no experience-bank atom.
- The chip moves to **You have** immediately, with the percentage and the two-colour bar recomputed
  client-side, before the request settles. Once the write lands, the block refetches the authoritative
  match so a newly unlocked adjacency (a claim can turn a third skill from missing to close) is not
  left showing a stale classification.
- A confirmation line under the block offers **Undo**, which removes just that skill again.
- A failed write rolls the chip back to where it was and states the failure.
- **You have** chips stay inert — this change adds no "remove a skill" affordance.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-profile-match`: the real-match state gains a requirement for claiming a Missing or Close skill
  into the profile from the block, covering the disclosure, the optimistic reclassification, the
  reconciliation against the server, and undo.

## Impact

- `web/src/lib/components/JobMatch.svelte` — chips in the Missing/Close groups become buttons; new
  inline claim row, confirmation line, and error state.
- `web/src/lib/jobMatch.ts` — a pure overlay that reclassifies a claimed skill as exact and
  recomputes counts and coverage, unit-tested in `jobMatch.test.ts`.
- `web/src/lib/profile.svelte.ts` — the store gains `addSkill`/`removeSkill`, serialised so two quick
  claims cannot each send a stale skill list (`PUT` replaces the whole profile row).
- No backend, database, or contract change: `jobmatch.Compute` already reclassifies on the next call,
  and the profile endpoint already lowercases, trims and deduplicates the skills it is handed.
