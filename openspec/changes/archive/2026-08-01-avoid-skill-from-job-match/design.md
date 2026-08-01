## Context

The claim affordance shipped in #1441 (archived change `claim-skill-from-job-match`): pressing a
Missing or Close chip expands a row under its group, and confirming writes the skill into
`user_profiles.skills` through `ProfileStore`, which serialises writes because `PUT /me/profile`
replaces the whole row.

This adds the opposite answer. It needs no new machinery — the row, the store queue, the pure
skill-set rules and the confirmation line all exist. What it needs is a second action in the row,
a second pair of pure rules, and a rendering state the block has never had: a chip that is missing
*and* deliberately so.

Two facts from the existing code decide the shape:

- **`excluded_skills` already travels with the profile.** `GET /me/profile` returns it and the
  block already holds that profile (it reads `profile.skills` to resolve its own state). So the
  avoided marking is a client-side derivation, not a fetch.
- **`jobmatch.Compute` never reads it.** The match is `job.skills` against `profile.skills` plus the
  adjacency dictionary. So an avoid cannot change the match, and nothing may be re-fetched or
  re-scored for one.

## Goals / Non-Goals

**Goals:**

- Record "I don't want this" at the moment the red chip prompts it.
- Make the record visible — on this job and on every other job that asks for the skill.
- Keep the two answers reversible from the same place.

**Non-Goals:**

- Filtering or hiding jobs from the block. `excluded_skills` feeds the *jobs filter* when the user
  applies their profile; it is not a per-job verdict, and this change does not make it one.
- An avoid affordance on **You have** chips. Those stay inert, as they were.
- Any change to the coverage formula. An avoided skill is still a skill the candidate lacks.

## Decisions

### The row names the skill instead of asking a question

"Do you have `bash`?" fitted a single answer. With two, the question form misleads — **Avoid** does
not answer it. The row becomes the skill's name followed by two pills:

```
bash   [✓ I have it]   [⊘ Avoid]
```

and, when the skill is already avoided:

```
bash   [✓ I have it]   [↺ Stop avoiding]
```

*Alternative considered:* keeping the question and putting the avoid elsewhere (a second row, a
long-press, the chip's context). Two rows in a ~285px column for one skill is more chrome than the
block spends on the match itself, and any hidden gesture is undiscoverable.

### The reverse action lives in the row, not only on the profile page

Without it, avoiding is one-way from this surface: the chip goes grey and the only path back is
`/my/profile`. Undo covers the misclick in the moment; **Stop avoiding** covers the change of mind a
week later. It is one more pill on a row that already exists.

### The avoided marking is derived, never stored per job

The chip's avoided state is `profileStore.profile.excluded_skills` ∋ skill, computed where the chip
renders. Consequences worth stating: it needs no request, it updates every open surface the moment
the write applies, and it is correct on a job the viewer has never opened before.

### The mutual-exclusion rule stays in the pure module

`withSkill` already drops the skill from `excluded_skills`. `withAvoidedSkill` mirrors it by
dropping the skill from `skills`. Both live in `profileSkills.ts`, so "a skill is never in both
lists" is one rule with one test, rather than a convention each caller re-implements.

This matters beyond tidiness: `filtersFromProfile` resolves an overlap by letting the include win
and dropping the exclude **silently**. A profile that held a skill in both lists would produce a
filter that quietly ignores the avoid, with nothing in the UI to explain why.

### Nothing is optimistic and nothing is reconciled

The claim path needed an overlay (the chip moves, the percent changes) and a refetch (an adjacency
the client cannot compute). An avoid needs neither: the only thing that changes is the profile's
excluded set, and the block reads that from the store, which the write updates on success. So the
avoid path is the simple one — write, then render what the store now says.

## Risks / Trade-offs

- **A struck-through chip could read as "this requirement is waived".** → The wording of the
  confirmation and the accessible name both say *avoid*, and the chip stays in the Missing group
  under its red heading, where the percentage it feeds is unchanged.
- **Avoiding widens the profile's exclusions, which narrows the feed when the profile is applied.**
  → That is the point of the action, it is reversible in place, and the filter modal shows the
  excluded set before it is applied.
- **Two writes now share one row, and a mis-press writes the wrong one.** → Both are single pills
  with distinct icons and tone, both are confirmed by name, and both are undoable from the line the
  write produces.
