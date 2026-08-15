## 1. Profile type and value mapping

- [x] 1.1 Add the six screening-answer fields to `AutofillProfile` in `extension/lib/freehire.ts`
- [x] 1.2 Map those six fields into `profileToValues()` in `extension/entrypoints/sidepanel/App.svelte`

## 2. Label matching

- [x] 2.1 Add `authorizedCountries`, `visaSponsorshipNeeded`, `desiredSalary`, `noticePeriod`,
      `willingToRelocate`, `age18OrOlder` entries to `FIELD_SYNONYMS` in `extension/lib/form.ts`
      with real-world ATS phrasings for each
- [x] 2.2 Confirm the boolean "are you authorized to work" phrasing is not matched to
      `authorizedCountries` (semantic mismatch: boolean question, list-shaped answer)
- [x] 2.3 Fix (found by review): `authorized_countries` arrives as comma-joined ISO codes
      ("US, DE"), but the checkbox-group questions it answers offer full country names
      ("United States") as options, and `chosenOptions` matches option-by-option — so raw
      codes never matched and the group silently stayed unchecked. Added
      `formatAuthorizedCountries` (`extension/lib/form.ts`, using `countryLabel` from
      `extension/lib/labels.ts`) and wired it into `profileToValues()`.

## 3. Tests and verification

- [x] 3.1 Add `matchFieldKey` test coverage for the six new mappings in `extension/lib/form.test.ts`
- [x] 3.2 Add a regression test asserting the plain work-authorization Yes/No question stays unmatched
- [x] 3.3 Add unit tests for `formatAuthorizedCountries` and an end-to-end `fillByLabel` test
      against a country checkbox group, covering the 2.3 fix
- [x] 3.4 Run `npx vitest run` (230/230 passing)
- [x] 3.5 Run `npm run check` (svelte-check, 0 errors)
- [x] 3.6 Run `npx eslint` on touched files (clean)
