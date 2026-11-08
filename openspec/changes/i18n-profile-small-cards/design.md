## Context

The catalog mechanism (`defineMessages`/`t`/`locale()`) and the scope-boundary
rule (translate a component only if it is exclusively reachable from
`/my/**`, or safely reads `page.data.locale`) are both already established —
see `PlanView.messages.ts`/`PlanView.svelte` and `ContributeView.messages.ts`
for reference shape. This change applies that mechanism to seven components;
it introduces no new infrastructure. See `proposal.md` for why this
particular slice of `/my/profile` goes first.

## Goals / Non-Goals

**Goals:**

- Every literal, reader-facing string these seven components own renders in
  the resolved locale.
- Follow the existing catalog conventions exactly (one `*.messages.ts` per
  component that has its own strings; a stable wire/dictionary token stays
  untranslated with a comment saying so, matching `viaPrefix` in
  `ContributeView.messages.ts`).

**Non-Goals:**

- Translating `/my/profile`'s four remaining tab views
  (`ExperienceBankView`, `ProfileForm`, `ScreeningAnswersForm`,
  `EducationCard`) — separate, larger changes.
- Localizing any facet dictionary (work mode, region, country, skills) or
  IANA timezone identifiers. These are data the backend/dictionary layer
  owns, not this component's own prose; a dictionary already localized would
  make its OWN component's job trivial, but building that mechanism is out
  of scope here.
- Adding `es`/`pt`/`de`/`fr` translations — item 2 of freehire#2005, blocked
  on the en/ru fan-out stabilizing first (the issue's own stated order).

## Decisions

### `AccountPreferences.svelte` gets no catalog

It renders only `<AccountTimezone />` and `<AccountLanguage />` with no
literal string of its own (confirmed by reading the file — a `<section>`
wrapper and nothing else). Adding an empty or near-empty catalog for a
component with nothing to translate would be process for its own sake.

### `AccountLanguage`'s six language names are translated, not left as English proper nouns

Alternative considered: leave `LANGUAGES`' `label` values (`'English'`,
`'Russian'`, …) untranslated, treating them like a wire token (a language
CODE is a wire token; the display NAME the reader sees in a translated
combobox is not). Rejected — unlike `viaPrefix`'s surface token or a
timezone identifier, a language's display name is prose a reader reads
directly in the picker, and leaving six English words in an otherwise
Russian settings row is exactly the half-translated seam this whole
initiative exists to close. Each name is added to
`AccountLanguage.messages.ts` as its own key.

### `LocationCard`/`SkillsCard` and the child component they delegate to are separate catalogs

Alternative considered: one shared catalog per wrapper+child pair (the way
`activity.messages.ts` deliberately became one catalog for five tiny views).
Rejected here — that precedent applied because those five views carried two
or three strings each and were reachable from nowhere else; here the child
components (`LocationPreferencesFields`, `SkillsPicker`) are substantial on
their own (318 and 59 lines) and are the more natural unit to colocate a
catalog with. `LocationCard.messages.ts` ends up holding exactly one key
(its own error string) — small, but consistent with "one catalog per
component with its own strings" rather than introducing a second exception
to the established rule in the same change that is supposed to be proving
the rule holds.

### Scope boundary: `RemoteSearchSelect`/`SearchSelect`/`$lib/ui` primitives are untouched

Both are reachable from public search filters, not just `/my/**` — the same
"exclusively reachable from `/my/**`" test the archived fan-out design.md
already applies. Any placeholder or empty-state text those shared components
render internally (as opposed to a placeholder STRING this change's own
components pass INTO them, e.g. `SkillsPicker`'s `placeholder="Search
skills"`) is out of scope here.

## Risks / Trade-offs

- **`LocationPreferencesFields` is used by both `LocationCard` (autosave)
  and `ProfileForm` (batched save, not yet translated)** — translating its
  catalog now means `ProfileForm`'s later translation change reuses this
  catalog rather than creating a second one, which is a benefit, not a risk,
  but worth flagging so that future change reads this one's catalog first
  rather than duplicating it.
- **Language-name translation is a judgment call** (see Decisions above) —
  revisit if it reads oddly once translated and reviewed.

## Migration Plan

No data migration. Frontend-only change, one PR. Rollback is a normal
revert.
