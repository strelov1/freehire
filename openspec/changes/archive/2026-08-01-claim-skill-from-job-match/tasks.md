## 1. Optimistic reclassification (pure)

- [x] 1.1 In `web/src/lib/jobMatch.test.ts`, add failing tests for a new `claimSkill(match, skill)`:
      a Missing skill moves to `matched` (exact count +1, coverage recomputed by
      `round((exact + 0.5 × adjacent) / total × 100)`); an adjacent skill also leaves `adjacent`
      (adjacent count −1); a skill the match does not carry returns the match unchanged; the input
      match object is not mutated.
- [x] 1.2 Implement `claimSkill` in `web/src/lib/jobMatch.ts` beside `matchBarSegments`, reusing the
      server's weighting so the optimistic figure cannot drift from `jobmatch.Compute`.

## 2. Profile writes

- [x] 2.1 Add failing tests in `web/src/lib/profileSkills.test.ts` for the pure merge rules:
      `withSkill(profile, skill)` appends the skill (no duplicate when already held, case-insensitively)
      and drops it from `excluded_skills`; `withoutSkill(profile, skill)` removes only that skill and
      leaves the rest — including a skill claimed afterwards — untouched.
- [x] 2.2 Implement `web/src/lib/profileSkills.ts` with those two pure functions.
- [x] 2.3 Add `addSkill`/`removeSkill` to `ProfileStore` (`web/src/lib/profile.svelte.ts`): each builds
      its payload from `#profile` at send time and goes through `save()`, chained on an internal
      promise so a second claim issued before the first settles is built from the first one's result.
      The runner cannot host the store itself (`web/vitest.config.ts` runs plain Node with no
      Svelte plugin, so runes never compile), so the ordering guarantee is tested one level down:
      `serialQueue` in `web/src/lib/serialQueue.ts`, with the store holding only the wiring.

## 3. The claim affordance in the match block

- [x] 3.1 Turn Missing and Close chips into `<button>`s in
      `web/src/lib/components/JobMatch.svelte`, carrying `aria-expanded` and a selected ring, with a
      single `claiming: string | null` selection so pressing another chip moves the row and pressing
      the same chip closes it. You-have chips stay plain `<span>`s; the locked teaser is untouched.
- [x] 3.2 Render the inline claim row under the group that owns the selected chip — "Do you have
      `<skill>`?" plus an **Add to profile** button — following `ReminderChip.svelte`'s inline
      disclosure, not a floating layer.
- [x] 3.3 Wire confirmation: apply `claimSkill` to an `optimistic` copy rendered in place of `match`,
      close the row, call `profileStore.addSkill`, then refetch `GET /jobs/:slug/match` and swap the
      overlay for the response. A failed refetch keeps the overlay; a failed write drops it and shows
      the error.
- [x] 3.4 Show the confirmation line with **Undo** after a successful claim; undo calls
      `profileStore.removeSkill`, restores the pre-claim match, and clears the line. Guard the whole
      surface against a stale response after navigating to another job (the existing `slug` re-check).

## 4. Verification

- [x] 4.1 `pnpm --dir web test` and `pnpm --dir web run check` (and lint) pass.
- [x] 4.2 Visually verify the block in a browser at the sidebar's real width: claim a Missing skill and
      a Close skill, confirm the chip moves, the bar and percentage change, undo restores, and the
      row is reachable and dismissable by keyboard.
