## Why

The match block now lets a viewer claim a missing skill they actually hold. The other honest
answer to a red chip is the opposite one: *I don't have this and I don't want it.* That answer is
already a first-class part of the profile — `excluded_skills` seeds the jobs filter's
`skills_exclude` set through "Apply my profile" — but the only way to record it is the profile
page, reached by leaving the job that prompted the thought.

Both answers belong at the same moment, in the same row. One raises the match; the other stops the
catalogue offering more of the same.

## What Changes

- The claim row gains a second action. Its prompt drops the question form, which only fitted one
  answer: the row now names the skill and offers **I have it** and **Avoid**.
- **Avoid** writes the skill into the profile's `excluded_skills` and removes it from `skills`,
  mirroring the existing rule that claiming a skill drops it from `excluded_skills`. A skill is
  never in both lists.
- An avoided skill's chip stays in **Missing** but renders as struck through and muted, on every
  job that asks for it — the avoided set is already in the profile store, so this costs no request.
- Pressing an avoided chip opens the row with **I have it** and **Stop avoiding**, so the mark can
  be lifted where it was made rather than only on the profile page.
- The coverage percentage, the bar and the chip groups do NOT move: the server computes the match
  from `skills` alone, and avoiding a skill changes nothing about whether the candidate has it.
- Confirmed by the same line the claim uses, naming what happened, with **Undo**.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-profile-match`: the claim affordance gains the avoid action, the avoided marking, and the
  reverse action that lifts it.

## Impact

- `web/src/lib/components/JobMatch.svelte` — the row's two actions, the avoided chip styling, the
  reverse action, and a confirmation that distinguishes the two writes.
- `web/src/lib/profileSkills.ts` — `withAvoidedSkill` / `withoutAvoidedSkill` beside the existing
  pair, holding the "never in both lists" invariant in one place.
- `web/src/lib/profile.svelte.ts` — `avoidSkill` / `unavoidSkill`, through the same serial queue.
- No backend, database, or contract change. `jobmatch.Compute` never reads `excluded_skills`, and
  the profile endpoint already normalises both lists.
