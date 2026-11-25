## Context

`CompanyPicker.svelte` already exists and is used by `ReferralsView.svelte` and `MentorProfileEditor.svelte`. In both current uses, the component owns its typed text as private state and only ever exposes a *resolved* catalogue company (`{slug, name}` or `null`) via a required `onSelect` callback — neither consumer needs the free-text value itself, since both ultimately submit a `company_slug`.

The experience bank's job forms are different: `jobs.company` (via `createExperienceEmployment`/`updateExperienceEmployment`) is a free-text field with no slug concept, and must keep accepting a company that is not in the catalogue at all (a candidate's past employer is frequently not one we crawl). See proposal.md for the full motivation.

## Goals / Non-Goals

**Goals:**
- Reuse `CompanyPicker` for the experience bank's Company field without changing its behavior for its existing two consumers.
- Keep free-text entry fully functional (typing a company that never appears in suggestions must still save correctly).
- Expose the existing `current` field through the add-job and edit-job forms only, since it does not apply to `project`-kind entries.

**Non-Goals:**
- No backend or API changes — `current` and `company` are already accepted end to end.
- No change to `ReferralsView.svelte` or `MentorProfileEditor.svelte` behavior.
- No dedicated `Checkbox` design-system component — out of scope for this change; follow the existing raw-`<input type="checkbox">` convention (e.g. `MentorProfileEditor.svelte:337`).

## Decisions

**`CompanyPicker`'s typed text becomes an optional `$bindable` prop, not a second component.**
Rather than forking a second "free-text company picker" component, `CompanyPicker`'s internal `query` state becomes a `value` prop declared with Svelte 5's `$bindable('')`, and `onSelect` becomes optional (`onSelect?.(...)`). A caller that only binds `value` (the experience bank) gets live text on every keystroke, including an unresolved pick; a caller that only uses `onSelect` (Referrals, Mentor) sees no behavior change, since an unbound `$bindable` prop simply keeps its local default and the component works exactly as it does today. This keeps one component for one job (search-and-suggest over the company catalogue) instead of two components that would drift apart.

Alternative considered: leave `CompanyPicker` untouched and wrap it in a new component that mirrors its input. Rejected — it would duplicate the debounce/race-guard/rendering logic the reviewed report already found in `CompanyPicker`, for no behavioral difference.

**Selecting a suggestion writes the canonical name into the same free-text field, never a slug.**
Because `jobs.company` has no slug column, `pick()`'s existing behavior of setting the visible text to `c.name` is exactly what's wanted — no new field, no new payload shape.

**The edit form's shared `empName` state is branched by `employment.kind`, not duplicated.**
`ExperienceEmploymentCard.svelte` already uses one `empName` state/input for both "Company" (job) and "Project name" (project), switching the label by kind. The same branch now also switches the control: `CompanyPicker` for `kind === 'job'`, the existing plain `Input` for `kind === 'project'`. No new state variable.

**"Current" hides the End date input rather than disabling it.**
`PeriodDateInput` has no `disabled` prop today. Conditionally rendering it (`{#if !jobCurrent}` / `{#if !empCurrent}`) avoids adding one for a single caller, and matches the mutual-exclusivity in the spec (a current employment has no end date to show).

## Risks / Trade-offs

- [Risk] A candidate could type a company name that happens to match a catalogue entry by coincidence but pick nothing, then save close-but-not-identical text (e.g. differing case) → Mitigation: none needed — this is the existing, accepted behavior of a free-text field; the picker is a convenience, not a validator.
- [Risk] Widening `CompanyPicker`'s public prop surface could be missed by a future edit to its two existing call sites if someone assumes `value` is required → Mitigation: `value` stays optional with a safe default; existing call sites are unchanged in this PR and continue to compile and behave identically.
