## Why

freehire#2005 (item 1) asks for the remaining `/my/**` pages to be fanned out
into the en/ru pattern `/my/security` and part of `/my/profile` already
established (#1998, #2477). `/my/profile`'s tab **views** are explicitly the
largest still-untranslated surface — only the tab labels around them were
translated. This change starts that fan-out with the smallest, self-contained
slice of `/my/profile`: the account-settings and location/skills/contacts
cards, so the pattern is proven again on a small footprint before tackling
the page's larger tabs (`ExperienceBankView`, `ProfileForm`,
`ScreeningAnswersForm`, `EducationCard`) in later changes.

## What Changes

- Seven components gain a colocated `*.messages.ts` catalog (en source + ru
  translation) and are migrated to render through `t(messages, locale())`,
  the established pattern from `$lib/i18n/t.ts`:
  - `web/src/lib/components/profile/LocationCard.svelte` (its one error
    string)
  - `web/src/lib/components/profile/LocationPreferencesFields.svelte` (the
    actual "where & how I want to work" UI `LocationCard` renders — labels,
    placeholders, section headings, empty-state hint)
  - `web/src/lib/components/profile/SkillsCard.svelte` (its two strings)
  - `web/src/lib/components/profile/SkillsPicker.svelte` (the "Skills"
    heading and search placeholder `SkillsCard` renders)
  - `web/src/lib/components/AccountTimezone.svelte`
  - `web/src/lib/components/AccountLanguage.svelte` (including the six
    language names in its own picker — translated, not left as English
    proper nouns, since they are prose a reader sees on a translated page)
  - `web/src/lib/components/CandidateContactsEditor.svelte`
- `web/src/lib/components/AccountPreferences.svelte` itself needs **no**
  catalog — it is a pure layout wrapper around `AccountTimezone`/
  `AccountLanguage` with zero literal strings of its own.
- **Out of scope, deliberately**: facet-dictionary and IANA-timezone values
  rendered by these components (`WORK_MODE_OPTIONS`/`REGION_OPTIONS`/
  `COUNTRY_OPTIONS` labels from `$lib/facets`, the skill-dictionary entries
  `SkillsPicker` searches, and the raw timezone identifiers `AccountTimezone`
  lists) — these are dictionary/wire tokens, not this component's own prose,
  the same boundary `ContributeView.messages.ts`'s `viaPrefix` comment
  already draws for a wire token. Localizing a facet dictionary is a
  separate, much larger initiative.
- **Out of scope, deferred to later changes**: `/my/profile`'s remaining
  four tab views (`ExperienceBankView`, `ProfileForm`,
  `ScreeningAnswersForm`, `EducationCard`) and every other still-untranslated
  `/my/**` page freehire#2005 lists.
- Shared components these cards delegate to for generic UI chrome
  (`RemoteSearchSelect.svelte`, `SearchSelect.svelte`, `$lib/ui`'s `Input`/
  `Button`) are **not** modified — they are reachable from public pages too,
  outside this change's scope boundary (the same rule the archived
  `i18n-my-account-fanout` change's design.md states: translate a component
  only if it is exclusively reachable from `/my/**`, or already reads
  `page.data.locale` safely).

## Capabilities

### New Capabilities

- `account-interface-i18n`: `/my/**` renders in the account's resolved
  locale (English or Russian today). No spec file exists yet under
  `openspec/specs/` for this capability — it has only ever been proposed as
  a delta inside two earlier, still-unarchived changes (`i18n-my-account`,
  `i18n-my-account-fanout`), both already shipped to production. This
  change adds one more delta on top, following the same shape; reconciling
  the three deltas is a pre-existing archival gap, out of scope here.

### Modified Capabilities

(none)

## Impact

- 7 component files plus 7 new colocated `*.messages.ts` catalogs.
- No backend, API, or database changes.
- No changes to `$lib/i18n/t.ts`, `$lib/locale.ts`, or the locale-resolution
  mechanism — this change only adds catalogs and migrates call sites,
  exactly as the archived fan-out changes did.
