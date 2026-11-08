## 1. Location — `LocationCard` + `LocationPreferencesFields`

- [x] 1.1 Write a failing test in a new `LocationPreferencesFields.spec.ts`:
      render the component under a mocked `$app/state` with
      `page.data.locale = 'ru'` (following `ApiKeysView.spec.ts`'s
      `vi.mock('$app/state', ...)` pattern) and assert a handful of its
      Russian strings appear (e.g. the "Work format" and "Where you're
      based" section headings, the "Add a city" placeholder) while the
      dictionary-derived option labels (`WORK_MODE_OPTIONS` etc., mocked or
      left as their real English dictionary values) are unaffected.
- [x] 1.2 Add `LocationPreferencesFields.messages.ts` (en/ru) covering every
      literal string: the "All optional…" hint, the "Work format" and
      "Where you're based" headings, the "City or country" placeholder, the
      "Searching…" loading row, the "Pick a work format above…" hint, the
      "Remote — regions you can work for (empty = worldwide)" heading, "Add
      specific countries" placeholder, "Open to relocation" checkbox label,
      "Where you'd relocate (empty = anywhere)" heading, and "Add a city"
      placeholder. Facet option labels stay as dictionary values, not
      catalog keys.
- [x] 1.3 Migrate `LocationPreferencesFields.svelte` to render through the
      catalog. Confirm the new test (1.1) passes.
- [x] 1.4 Add `LocationCard.messages.ts` (en/ru) covering its one string
      ("Could not update your location. Try again.").
- [x] 1.5 Migrate `LocationCard.svelte` to render through the catalog.
- [ ] 1.6 Manual verification under `language = ru`, including triggering
      the save-error path if feasible. Not done live in a browser this
      session — see 6.3.

## 2. Skills — `SkillsCard` + `SkillsPicker`

- [x] 2.1 Add the `$app/state` mock (`page: { data: {}, url: new
      URL('http://localhost/') }`) to the existing `SkillsCard.spec.ts` so
      it keeps passing once `SkillsCard.svelte` calls `locale()` — do this
      before 2.4, since otherwise the existing suite goes red for a reason
      unrelated to the new catalog (the same failure mode found and fixed
      in `SourceCatalog.spec.ts` during the date-locale change).
- [x] 2.2 Write a failing test (new file or extend `SkillsCard.spec.ts`)
      asserting `SkillsPicker`'s "Skills" heading and "Search skills"
      placeholder, and `SkillsCard`'s two own strings ("You need at least
      one skill…", "Could not update {skill} in your profile. Try again."),
      render in Russian under a mocked `ru` locale.
- [x] 2.3 Add `SkillsPicker.messages.ts` (en/ru: heading + placeholder) and
      `SkillsCard.messages.ts` (en/ru: the two strings, the second with an
      interpolated skill name).
- [x] 2.4 Migrate `SkillsPicker.svelte` and `SkillsCard.svelte` to render
      through their catalogs. Confirm 2.1 and 2.2 pass.
- [ ] 2.5 Manual verification under `language = ru`, including the
      last-skill-blocked and save-failure states if feasible. Not done live
      in a browser this session — see 6.3.

## 3. `AccountTimezone`

- [x] 3.1 Write a failing test in a new `AccountTimezone.spec.ts` (mocking
      `$app/state` for `locale = 'ru'` and `$lib/auth.svelte` the way
      `SkillsCard.spec.ts` mocks `$lib/profile.svelte`) asserting the
      "Timezone" heading, its description, the "Select a timezone" default
      option, and the "Saving…"/"Saved"/error strings render in Russian.
      IANA zone identifiers themselves stay untranslated.
- [x] 3.2 Add `AccountTimezone.messages.ts` (en/ru) covering all five
      strings.
- [x] 3.3 Migrate `AccountTimezone.svelte` to render through the catalog.
      Confirm 3.1 passes.
- [ ] 3.4 Manual verification under `language = ru`, including the saving
      and saved-confirmation states. Not done live in a browser this
      session — see 6.3.

## 4. `AccountLanguage`

- [x] 4.1 Write a failing test in a new `AccountLanguage.spec.ts` asserting
      the "Language" heading, its description, the "Search a language…"
      placeholder, "No matches", the "Saving…"/"Saved"/error strings, AND
      the six language names (English/Russian/Spanish/Portuguese/German/
      French) all render in Russian under a mocked `ru` locale — this is
      the deliberate call from design.md's Decisions section, so the test
      should make that choice explicit rather than leaving it implicit.
- [x] 4.2 Add `AccountLanguage.messages.ts` (en/ru) covering all of the
      above, including the language-name map.
- [x] 4.3 Migrate `AccountLanguage.svelte` to render through the catalog —
      `LANGUAGES`' `label` field becomes a lookup into the catalog's
      language-name map rather than a literal.
- [ ] 4.4 Manual verification under `language = ru`, confirming the language
      picker itself (used to switch OUT of Russian) still reads correctly.
      Not done live in a browser this session — see 6.3.

## 5. `CandidateContactsEditor`

- [x] 5.1 Write a failing test in a new `CandidateContactsEditor.spec.ts`
      asserting the "Your contacts" heading, its description, all four
      field placeholders (Full name/Email/Phone/Location), the "Links (one
      per line)" label, the "<https://…>" placeholder, the "Save contacts"
      button, and the save-success/save-error messages render in Russian
      under a mocked `ru` locale.
- [x] 5.2 Add `CandidateContactsEditor.messages.ts` (en/ru) covering all of
      the above.
- [x] 5.3 Migrate `CandidateContactsEditor.svelte` to render through the
      catalog. Confirm 5.1 passes.
- [ ] 5.4 Manual verification under `language = ru`, including a failed
      save. Not done live in a browser this session — see 6.3.

## 6. Wrap-up

- [x] 6.1 Run `pnpm --dir web check`, `pnpm --dir web lint`, and `pnpm --dir
      web test`; fix anything this change introduced. (check: 0 errors;
      lint: exit 0; test: 203/203 files, 2258/2258 tests passing — final
      count after the fixes below.)
      **Independent code review caught task descriptions (1.1, 3.1, 4.1)
      overstating what the delivered tests actually asserted** — the
      Timezone/Language save-state (saving/saved/error) and remaining
      language-name coverage, and the relocation section's "Add a city"
      placeholder, were described but not written. Extended
      `AccountTimezone.spec.ts`, `AccountLanguage.spec.ts`, and
      `LocationPreferencesFields.spec.ts` to actually cover them (the
      catalogs and component wiring were already correct — only the test
      coverage was short of its own description).
      **A second, independent review (`/code-review`) found four more real
      issues, all fixed:**
      1. `AccountTimezone.svelte`, `AccountLanguage.svelte`,
      `CandidateContactsEditor.svelte`, and `LocationCard.svelte` each
      captured a translated fallback error message into `$state` at the
      moment of failure, so an already-shown error froze in whatever
      locale was active then — a later locale change (e.g. via the
      sibling `AccountLanguage` card) left it stale. Fixed by storing a
      flag/the server's own raw message in `$state` and deriving the
      displayed text from `s` (the resolved catalog) reactively, in all
      four components. (A live regression test proving the reactive
      update was attempted and reverted — the shared `$app/state` test
      mock is a plain object, not a genuine Svelte reactive primitive, so
      it cannot exercise a mid-session locale change; the fix is
      verified by code reading instead.)
      2. `ProfileForm.spec.ts` renders `SkillsPicker`/`LocationPreferencesFields`
      (via its create-profile branch) without its own `$app/state` mock;
      no current test exercises that branch, but the shared stub would
      throw the moment one does. Added the same defensive mock
      `SkillsCard.spec.ts` already needed.
      3. `AccountLanguage.svelte`'s six language codes and
      `AccountLanguage.messages.ts`'s `languageNames` catalog keys were
      two independent, hand-written lists with nothing tying them
      together. `LANGUAGE_CODES` now derives from `$lib/locale.ts`'s
      `SUPPORTED_LOCALES`, and `languageNames` is typed
      `satisfies Record<Locale, string>` — adding a locale without a
      matching catalog entry is now a compile error, not a silent
      `tokenLabel` fallback to the raw code.
      4. `SkillsCard`'s save-failure message concatenated
      `saveFailedPrefix`/`saveFailedSuffix` catalog keys around an
      interpolated skill name — a shape that hard-codes English/Russian
      word order and can't express a language whose grammar needs the
      value positioned differently. Added a minimal `format()`
      placeholder-substitution helper to `$lib/i18n/t.ts` (with its own
      tests) and switched `SkillsCard.messages.ts` to a single
      `'Could not update {skill} in your profile. Try again.'` template.
      Also applied the first review's Minor suggestion: `AccountLanguage`'s
      `byCode` dropped its unused `list` parameter (always called with the
      same array).
- [x] 6.2 Re-read design.md's Decisions and Risks and confirm the
      language-name translation reads naturally once rendered — revisit the
      choice if it does not.
- [ ] 6.3 Full manual pass on `/my/profile` under `language = ru`: open the
      account-settings, location, skills, and contacts sections in one
      session and confirm no leftover English string among the ones this
      change owns (facet/dictionary/timezone values excepted). **Not
      performed in this session** — the local dev stack's search/job-lookup
      backend was unreliable in the previous change (`fix-my-account-date-locale`),
      and this change was not re-attempted against a live authenticated
      `language = ru` account. Every translated string is instead covered by
      a component-level test rendering the real `t()`/`defineMessages`
      output for `'ru'` (not mocked), which is the closest verification
      available without a live browser session. Recommend a real
      click-through before or shortly after this ships.
