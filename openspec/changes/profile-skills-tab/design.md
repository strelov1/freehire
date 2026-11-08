## Context

`/my/profile`'s Settings tab renders `ProfileForm.svelte`, a two-tab form
("Skills & role" / "Location & work") sharing one "Save changes" button that PUTs
the whole profile row. Skills and Skills to avoid currently sit in the "Skills &
role" tab next to Role/specializations, CV upload, and headshot.

`profileStore` (`web/src/lib/profile.svelte.ts`) already exposes single-skill
autosave mutators — `addSkill`/`removeSkill`/`avoidSkill`/`unavoidSkill` — each
issuing its own `PUT /me/profile` through a serial queue (so two edits made a
moment apart don't clobber each other, since the endpoint replaces the whole
row). These are used today by `JobMatch.svelte`'s "I have it" / "Avoid" chips on
a job posting — proof the autosave pattern already works end-to-end, just not
yet exposed as a standalone profile-editing surface.

Top-level page tabs on `/my/profile` (`web/src/routes/my/profile/+page.svelte`)
only render once a profile exists; before that, the bare `ProfileForm` is the
entire page (no tab strip). The server also requires at least one skill to
create a profile (`skills` must be non-empty on `PUT /me/profile`).

## Goals / Non-Goals

**Goals:**

- Give Skills / Skills to avoid their own top-level tab, edited with immediate
  per-toggle autosave (no shared Save button).
- Keep first-time profile creation working unchanged (Role + Skills entered
  together, one Save, since no profile — and no tab strip — exists yet).
- Keep CV-driven skill extraction working for an existing profile without a
  Settings-tab skills field to merge into.

**Non-Goals:**

- No change to the `PUT /api/v1/me/profile` contract or `user_profiles` schema.
- No change to Role/specializations, location preferences, headshot, or CV
  storage/readiness behavior.
- No new "review before applying" UI for CV-extracted skills (out of scope per
  the resolved design question — extraction autosaves directly).

## Decisions

**Skills tab reads/writes the store directly; no local buffer.**
`SkillsView.svelte` renders straight off `profileStore.profile?.skills` /
`?.excluded_skills` and calls `addSkill`/`removeSkill`/`avoidSkill`/`unavoidSkill`
on each chip toggle — mirroring `JobMatch.svelte`'s existing `avoided` derived-set
pattern. Alternative considered: give the new tab its own buffered
skills/excludedSkills state plus a local Save button, matching `ProfileForm`'s
existing shape. Rejected — it would duplicate the buffer-and-PUT machinery
`ProfileForm` already has, when the per-skill mutators already do exactly this
job and are proven in production by `JobMatch`.

**Skills stays inside `ProfileForm` during creation, leaves it once editing.**
`ProfileForm` keeps its skills/excluded-skills fields when `profile === null`
(`!editing`) because the create flow has no tab strip yet and the server won't
accept a skill-less profile. Once a profile exists, those fields disappear from
the form; the "Skills & role" tab is relabeled "Role". Alternative considered:
always show a placeholder/link in Settings pointing at the new tab. Rejected as
unnecessary indirection — the new top-level tab is one click away in the same
tab strip.

**CV upload against an existing profile autosaves extracted skills AND any
specialization the same extraction resolved, together, in one write — not two,
and not N sequential single-skill calls.**
`profileStore.mergeResumeExtraction(newSkills, specializations)` folds
`withSkills` over the extracted skills and saves them alongside the caller's
already-locally-merged specializations, queued through the same `#queue` as the
other mutators. Two candidates were rejected:

- Looping the single-skill `addSkill`: one full-row `PUT` per extracted skill
  (5-15 typically), all serialized, for no benefit over one bulk write.
- A skills-only bulk mutator (the first version of this decision, `addSkills`,
  called alone): found in review of task 4 to silently drop any specialization
  the same CV resolved. `ProfileForm` is wrapped in `{#key profile.updated_at}`
  (`+page.svelte`), so ANY write to the profile row remounts it fresh from the
  server — including a write this same component triggers for skills alone. A
  specialization merged into `ProfileForm`'s local (unsaved) state moments
  earlier, in the same `analyzeResume()` call, would still be sitting in that
  local state when the remount discarded it — the user would never get a
  chance to save it via the visible Role field, because the field's own
  component instance is already gone. Committing specializations in the SAME
  write as the skills closes this: nothing is left in local-only state for the
  remount to orphan.

**Shared skill-dictionary loader.**
Both `ProfileForm` (create flow) and `SkillsView` need the same typeahead
universe — `api.facetCounts` → sort by descending job count. Extracted to
`web/src/lib/skillDictionary.ts` (`loadSkillDistribution(): Promise<FacetOption[]>`)
rather than duplicated, since it's now needed in two independent components.

**The Skills tab refuses to remove or avoid a profile's one remaining skill,
client-side, before attempting a write.**
`normalizeSkills` (`internal/userprofile/userprofile.go`) requires a non-empty
skill set on every save, not only at creation — `Save` is the single path both
flows go through. Without a `ProfileForm`-style Save-button gate, an autosaving
Skills tab has no other checkpoint before the request goes out, so removing (or
avoiding, which also un-claims) the last skill would always come back a 400.
Caught in `SkillsView.svelte` before the call: `skills.length === 1 &&
skills.includes(skill)` blocks the toggle and shows a message that a skill is
required, instead of sending a request that cannot succeed and reporting it
through the generic (retry-implying) failed-write copy. Found during review of
task 3 — see the "Removing the one remaining skill is refused client-side"
scenario added to the spec delta.

## Risks / Trade-offs

- **[Risk]** A user toggles a skill on the new tab while `ProfileForm`'s
  Settings tab is also mounted (both subscribe to the same `profileStore`,
  possible only in the create-flow's absence of tabs — not reachable once
  editing, since Settings and Skills are mutually exclusive tab panels). →
  **Mitigation**: not reachable in practice; the two are never both mounted for
  an existing profile since `TabRow` shows one panel at a time.
- **[Risk]** Losing the explicit Save affordance could surprise a user used to
  reviewing before committing. → **Mitigation**: this mirrors the existing,
  shipped `JobMatch` skill-editing UX exactly, so the interaction pattern is
  already validated in this codebase, not a novel one.
- **[Trade-off]** CV-extracted skills for an existing profile now write
  immediately with no review step, vs. the "surface as suggestions" alternative
  considered and declined during brainstorming (adds a review UI for a case that
  behaves acceptably today: false extractions can simply be removed on the
  Skills tab, same as an unwanted manual entry).
- **[Risk]** The same `{#key profile.updated_at}` remount that would have
  discarded an unsaved specialization (see the decision above) may also mean
  `ProfileForm`'s own `resumeNote` confirmation ("Added N skills…") never
  renders for an editing-mode CV upload — by the time `mergeResumeExtraction`'s
  write resolves and the component would show the note, the remount may already
  have replaced it with a fresh instance. → **Mitigation**: the DATA is
  unaffected (this is display-only, unlike the specialization-loss bug), and the
  result is directly visible on the Skills tab regardless. Meant to be verified
  empirically in a browser during task 5 (Verification) rather than guessed at
  from Svelte's effect-flush timing — that verification was skipped (see task
  5's notes), so this remains unconfirmed either way. Not worth a structural
  fix (e.g. lifting the note to the parent) unless a future check shows it
  matters in practice.

## Migration Plan

Frontend-only, single deploy, no data migration, no feature flag. Rollback is a
plain revert (no schema/API change to unwind).

## Open Questions

None outstanding — placement, autosave-vs-Save, and CV-extraction behavior were
resolved during brainstorming (see
`docs/superpowers/specs/2026-08-04-profile-skills-tab-design.md`).
