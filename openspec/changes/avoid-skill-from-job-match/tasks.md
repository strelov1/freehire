## 1. The skill-set rules

- [x] 1.1 Add failing tests in `web/src/lib/profileSkills.test.ts` for `withAvoidedSkill`
      (appends to `excluded_skills`, no duplicate whatever the case, removes the skill from
      `skills`) and `withoutAvoidedSkill` (removes only that skill from `excluded_skills`, leaves
      `skills` untouched). Both pure, neither mutating its input.
- [x] 1.2 Implement the pair in `web/src/lib/profileSkills.ts` beside `withSkill`/`withoutSkill`,
      so the "never in both lists" invariant has one home.

## 2. The profile writes

- [x] 2.1 Add `avoidSkill`/`unavoidSkill` to `ProfileStore`, routed through the same
      `#queue`/`#writeSkills` path as the claim so a claim and an avoid issued together cannot
      clobber each other.

## 3. The row's second action

- [x] 3.1 Replace the row's question with the skill's name plus two pills — **I have it** and
      **Avoid** — in `web/src/lib/components/JobMatch.svelte`, keeping the existing claim wiring
      behind the first.
- [x] 3.2 Render an avoided chip distinctly (struck through, muted) with an accessible name saying
      the skill is avoided, derived from `profileStore.profile.excluded_skills` so it holds on
      every job without a request.
- [x] 3.3 Swap the second pill to **Stop avoiding** when the chip is already avoided, wired to
      `unavoidSkill`.
- [x] 3.4 Confirm an avoid with its own wording ("added to skills you avoid") and its own undo,
      distinct from the claim's; roll back and report a failed write, and issue no match refetch on
      either avoid path.

## 4. Verification

- [x] 4.1 `pnpm --dir web test`, `pnpm --dir web run check`, and lint on the touched files pass.
- [x] 4.2 Drive the block in a browser at the sidebar's width: avoid a skill (coverage unchanged,
      chip struck), reload onto another job asking for it (still struck), stop avoiding, and claim
      a previously-avoided skill (moves to You have, leaves the excluded set).
