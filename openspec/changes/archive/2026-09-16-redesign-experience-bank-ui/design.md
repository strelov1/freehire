## Context

`ExperienceBankView.svelte` (995 lines) currently renders everything inline as Svelte
`{#snippet}` blocks: `employmentSection` (card: logo, header, summary, stack, then the full
achievement `<ul>`) and `achievement` (one `<li>` per bullet, in three modes: display / editing
/ promote-to-project). All state — `bank`, `editing`, `selected`, the add/edit form drafts —
lives in this one component. See proposal.md for the motivation and specs/experience-bank/spec.md
for the new observable-behavior requirements (collapse, cross-employment selection, banner
link).

Relevant design-system primitives already in `$lib/ui`: `Chip` (pill, `variant="brand"`
matches the current hand-rolled skill-tag style exactly), `FormField`+`Input` (labelled field
wrapper), `Card` (bordered container — not used here, see Decisions). There is no
Accordion/Disclosure or Checkbox primitive in the design system.

## Goals / Non-Goals

**Goals:**

- Collapse achievement lists behind an in-place expand, without breaking cross-employment
  selection for Merge / Tailor with assistant.
- Remove borders and hand-rolled tag markup; adopt `Chip` where it already fits.
- Rework the add/edit forms onto `FormField`/`Input`.
- Keep every existing capability (merge, promote-to-project, confirm, edit, delete, the
  interviewer entry points) working exactly as today — this is a presentation and disclosure
  change, not a feature change.

**Non-Goals:**

- No new design-system primitive (no Accordion, no Checkbox) — one call site does not justify
  adding one; the expand/collapse is local component state, and the selection checkbox stays a
  styled native `<input type="checkbox">` as it is today.
- No API, database, `freehire-cli`, or assistant tool-registry changes.
- No change to the interviewer/assistant panel docking or its entry points (already covered by
  existing `experience-bank` requirements, untouched here).

## Decisions

**Componentization.** Split `ExperienceBankView.svelte` into:

- `ExperienceBankView.svelte` — retains all state and mutation logic (load, save, delete,
  merge, confirm, selection), renders the page shell, the unconfirmed banner, and the two
  sections (jobs, projects) plus unplaced achievements.
- `ExperienceEmploymentCard.svelte` — one employment: logo/header/summary/stack, collapsed
  achievement count, expand toggle, its own edit-in-place form. Takes the employment, its
  atoms, and callbacks as props; holds its own `expanded` boolean as local `$state`.
- `ExperienceAchievementRow.svelte` — one achievement: display / editing / promote-to-project
  modes, restyled, with its edit and promote drafts (`isEditing`/`isPromoting` and their form
  fields) now LOCAL `$state` rather than the single bank-wide `editing`/`promotingAtomId` the
  original file held. Takes the atom and callbacks as props; selection state (`selected`) stays
  lifted in the parent so it survives any card's expand/collapse.
This mirrors the reasoning in `frontend-design`/`svelte-core-bestpractices`: keep state where
it is actually shared (selection, the bank, mutations) and push everything else down into
props-driven, independently readable components. Rejected alternative: keep one file and just
add local `$state` per employment for `expanded` — still workable at 995 lines, but the file
already mixes three concerns (data, employment card markup, achievement row markup) and adding
a fourth (expand state per employment, threaded through the same snippets) makes the diff to
review far larger than three small files.

**Accepted behavior change: edit mode is no longer bank-wide exclusive.** The original file's
single `editing`/`editingEmploymentId`/`promotingAtomId` fields meant starting to edit one
achievement silently reverted any other in-flight edit elsewhere in the bank back to view mode
— an artifact of those drafts being shared component-level variables, not a deliberate
constraint anyone asked for. Moving them into each row/card's own local `$state` (the state
split above) removes that side effect: any number of rows can now be mid-edit at once, each
saving independently through its own callback. Nothing shares mutable state across rows, so
nothing can corrupt; this is called out explicitly here, rather than left as an undocumented
side effect of the componentization, because the Goals section above says "keep every existing
capability working exactly as today" and this one narrow interaction does not.

**Expand/collapse is local, not a new primitive.** A per-card `expanded = $state(false)`
toggled by clicking the summary row. No route, no store. The banner-link requirement (jump to
the first unconfirmed achievement) needs a way to force one specific card open and scroll to
one specific row from the parent — implemented as two props passed down:
`forceExpanded?: boolean` (parent sets it true for the target employment after computing which
one holds the first unconfirmed atom) and `scrollToAtomId?: string` (the row does its own
`scrollIntoView` in an `$effect` keyed on that id changing, and calls `element.focus()` for
keyboard/screen-reader users, matching how the interviewer entry point already treats
first-open behavior). The card's own auto-expand `$effect` reads BOTH `forceExpanded` and
`scrollToAtomId` (`if (forceExpanded && scrollToAtomId) expanded = true`) rather than the
boolean alone: re-clicking the banner for a different atom under the same still-collapsed
employment changes `scrollToAtomId` without changing `forceExpanded` (already `true`), and a
boolean-only dependency would miss that re-trigger. The one limitation this does NOT close —
re-clicking the banner for the exact same, already-visible target — is accepted below. Rejected
alternative: lift `expanded` for every employment into the parent's own state map — more
central state to keep in sync for no behavioral benefit, since nothing outside one card ever
needs to know another card's expand state except this one banner-jump case, which only needs
to *set* it.

**Selection is unaffected by collapse.** `selected: string[]` already lives in the parent and
is keyed by atom id, not by DOM presence — collapsing an employment only stops rendering its
`<ExperienceAchievementRow>` instances, it does not clear anything from `selected`. This
requirement is satisfied by construction once the state split above is in place; no additional
mechanism is needed.

**Borders removed, not swapped for `Card`.** `Card` (`design-system/src/card.svelte`) always
carries `border border-border ... shadow-sm` with no prop to turn it off, and the ask is
specifically to remove borders — reaching for `Card` and fighting its default with an override
class would be working against the primitive rather than using it. Employment cards and
achievement rows become plain `<div>`/`<li>` with spacing (`gap-*`, `py-*`) and a subtle
`bg-muted/30`-style background only on the states that need to stand out (selected, unconfirmed
— the same two states that use color today), not on every row.

**Tag markup.** The stack/skill hand-rolled `<span class="... rounded-full bg-brand-muted ...">`
becomes `<Chip variant="brand">` (identical visual result, since `chipVariants.brand` is
`border-transparent bg-brand-muted text-brand-strong` — the same classes, minus a border that
was already `border-transparent`). The metrics tags (previously borderless: `bg-muted ...
font-mono`, no border) and the `cluster_id`/`needs_context`/`needs_metrics` flags (previously
`border border-border`) both become `<Chip>` (the `default` variant) — but that variant's own
base class carries a real `border-border`, so both call sites add `class="border-transparent"`
to actually land on borderless, the same technique `brand`/`primary`/`secondary` already use
internally. Reaching for the primitive still avoids inventing a new borderless variant for two
call sites; it just needs the same one-class override those variants bake in.

**Forms.** Each raw `<input>`/`<textarea>` with a `placeholder` becomes a `FormField` wrapping
an `Input` (or a plain `<textarea>` inside `FormField`'s children slot, since there is no
design-system `Textarea`) with a real `<label>`. This is a like-for-like field count — no new
fields, no removed fields — just labelled instead of placeholder-only, which is what
`FormField` is for.

## Risks / Trade-offs

- [Splitting one 995-line file into three changes every diff touching this view] → the split
  follows existing state/props boundaries exactly (data+mutations vs. one employment vs. one
  achievement), so there is one obvious place for any future change; documented here so a
  reviewer isn't surprised by the file count.
- [`scrollIntoView` + collapsed-card auto-expand could fight a user who is mid-edit elsewhere
  on the page] → the banner-jump only ever runs on an explicit click of the banner, never on
  load or on an unrelated action, so it cannot preempt something the candidate is doing.
- [Re-clicking the banner while its current target is already expanded and in view does
  nothing — `forceExpanded`/`scrollToAtomId` haven't changed value, so neither the card's nor
  the row's `$effect` re-fires] → accepted: the achievement is already visible, so the click's
  goal is already met: there's nothing further to jump to. The other collapse-related edge case
  (re-clicking for a DIFFERENT atom under a still-collapsed employment) is not accepted and is
  fixed — see "Expand/collapse is local" above.
- [Removing borders relies on background-color alone to separate rows in a long list] → verified
  live in the browser per the repo's UI-change convention (this file's own AGENTS-adjacent
  convention in `web/`), specifically at a job with 10+ achievements, before calling this done.

## Migration Plan

Frontend-only, feature-flag-free: the new components replace the old snippets in one PR,
`onBankMutated` and the API calls are untouched, so there is nothing to stage or roll out
gradually. Rollback is a plain revert.
