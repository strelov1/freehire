## 1. Reshape the catalog API for N locales

- [x] 1.1 Change `web/src/lib/i18n/t.ts`: `defineMessages<T>(en: T, translations: Partial<Record<Exclude<Locale, 'en'>, DeepPartial<T>>>)` returning `{ en: T } & Partial<Record<Locale, T>>` — one deep-merged copy per locale actually present in `translations`, none for the rest. `t(catalog, locale)` returns `catalog[locale] ?? catalog.en`. Keep `deepMerge`, `isMessages` and the `string[]`-is-a-leaf rule exactly as they are.
- [x] 1.2 Extend `web/src/lib/i18n/t.test.ts`: a catalog with a third locale resolves that locale; a locale absent from the map resolves to the English source; a key omitted from a present locale falls back per key (the existing assertion, re-expressed against the new signature); a `string[]` leaf is still replaced whole. (Also asserts no copy is built for an untranslated locale — the property the `??` fallback exists to buy.)
- [x] 1.3 Migrate the four existing call sites to the new signature — `web/src/lib/i18n/shell.ts`, `web/src/routes/my/security/messages.ts`, `web/src/routes/my/profile/messages.ts`, `web/src/lib/components/DeleteAccountButton.messages.ts`. Text unchanged in this task; the diff is `{ ru: … }` wrapping only.
- [x] 1.4 (added — not foreseen) Confirm a call site left on the old signature is a **compile** error, not a silent no-op: `defineMessages(en, { navItems: … })` would otherwise register a locale named `navItems` and quietly leave Russian unresolved. Verified by running `pnpm check` against an un-migrated `shell.ts` — `"Object literal may only specify known properties, and 'navItems' does not exist in type 'Partial<Record<TranslatedLocale, …>>'"`. The excess-property check on an object literal is what makes this safe; no runtime guard is needed.

## 2. Guard the navigation catalog

- [x] 2.1 Add `web/src/lib/i18n/shell.test.ts`: for every href in `accountNav`, assert an entry exists in the `en` navItems map and that its `ru` label is not still the English one; assert every navItems key is an href `accountNav` still lists. Each assertion collects **every** offender before failing, so one run names them all — the drift arrived three at a time. Failure messages name the hrefs.
  - Note: asserting "the `ru` map has this key" is vacuous — `t(messages, 'ru')` is the translation deep-merged over English, so every English key is present regardless. Equality with the English label is the only signal a merged catalog can offer. `SAME_IN_RUSSIAN` is the escape hatch for a section legitimately identical in both, keeping that a reviewed statement rather than something the check cannot distinguish from an omission. Empty today.
- [x] 2.2 Confirm the new test fails on the pre-fix catalog before fixing it — evidence the guard actually guards. Observed: `navigation sections with no entry in the label catalog: expected [ '/my/lists', '/my/integrations', '/my/webhook' ] to deeply equal []` — all three named in one run.
- [x] 2.3 (added — not foreseen) Add a `$lib` alias to `web/vitest.config.ts`. That config deliberately omits the SvelteKit plugin, so it also omits the alias that plugin provides: `shell.ts` imports `defineMessages` as a *value* through `$lib/i18n/t` and could not be loaded from a test at all. `t.ts` was testable only by accident — its `$lib/locale` import is `import type` and erases before the resolver runs. Whether a module happens to import a type or a value is no basis for whether it can have a test.

## 3. Fix the nav-label defects

- [x] 3.1 Add the sections missing from both navItems maps in `web/src/lib/i18n/shell.ts`: `/my/integrations` ("Интеграции"), `/my/lists` ("Списки вакансий"), `/my/webhook` ("Вебхук"). Three, not the one the issue named — `accountNav` has grown from 14 entries to 17 since the catalog was written. Task 2.1's test goes green.
- [x] 3.2 Correct `'/my/submissions'`: `'Мои публикации'` → `'Мои вакансии'`. The section lists jobs the user submitted for review (`MySubmissionsView.svelte`: "Jobs you submitted for review"), not published posts.
- [x] 3.3 Correct `'/my/contributions'`: `'Вклад'` → `'Добавить борд'`. The section is "Contribute a board" — paste a job link so we crawl a board we don't have.
- [x] 3.4 Settle the wording on `/my/security`: `subtitle` says "сеансы", the section heading below it says "Сессии". Pick "сессии" in both (`web/src/routes/my/security/messages.ts`).
- [ ] 3.5 Manual verification: with language set to Russian, open any `/my/**` page and confirm the sidebar reads "Интеграции", "Списки вакансий", "Вебхук", "Мои вакансии" and "Добавить борд", and that no English label remains.

## 3b. Act on the phase-1 code review

- [x] 3b.1 **The locale resolvers were hard-wired to `'ru'`**, so a catalog translated into `es`/`pt`/`de`/`fr` could never render — the reshaped API gave those locales somewhere to go in the *type* and nowhere at *runtime*, and `t.test.ts` did not notice because it hands `'es'` straight to `t()`, bypassing the only code that decides a locale. Naively widening both to `isLocale` would have shipped a regression instead: `<html lang="es">` over English text, announced by a screen reader with Spanish phonemes, for as long as the copy was missing. Added `TRANSLATED_LOCALES` + `isTranslatedLocale` to `web/src/lib/locale.ts` — the locales the section is actually WRITTEN in, a subset of what `users.language` accepts — and read it from both `web/src/hooks.server.ts` and `web/src/routes/+layout.server.ts`. Behaviour today is unchanged (`['en', 'ru']`); translating a locale is now that one array plus its catalogs.
- [x] 3b.2 Correct the two comments that argued the old `'ru'`-only rule was safe *because* it matched `t()`'s fallback — true before the reshape, false after, and they would have convinced the next reader the resolver was already right.
- [x] 3b.3 Extend `web/src/lib/locale.test.ts` (it already covered `isLocale` and the cookie name): both predicates over both lists, a supported-but-untranslated locale is rejected, `TRANSLATED_LOCALES ⊆ SUPPORTED_LOCALES`, and English is always translated. This is the test whose absence let a green suite sit on top of a feature that could not work.
- [x] 3b.4 **The nav guard could not catch a rename.** `navLabel` returns `navItems[href] ?? item.label`, and task 3.1 completed the catalog to all 17 hrefs — so `accountNav[].label` is now unreachable and renaming a section there changes nothing on screen and fails nothing. Added a fourth assertion comparing each catalog English label to `accountNav`'s own.
- [x] 3b.5 Correct the spec delta: the Spanish scenario as written was unsatisfiable. Added the requirement that `<html lang>` never names a language the page is not written in, with the untranslated-preference and corrupt-preference scenarios.

## 4. Fan-out — `/my/submissions`

- [x] 4.1 Add `web/src/lib/components/MySubmissionsView.messages.ts` (en/ru) covering the heading, the "Jobs you submitted for review" line with its inline "Submit another" link text, the signed-out prompt, the error and empty states, and each review-status label plus the "Reason:" prefix. The status labels matter: the view printed the API's `Submission['status']` enum verbatim as the pill text, so the wire token was the display string.
- [x] 4.2 Migrate `MySubmissionsView.svelte` to render through the catalog. Move the `<title>` in `web/src/routes/my/submissions/+page.svelte` into a `headTitle` key and render it from the same catalog. (The `{#each submissions as s}` loop variable was renamed to `sub` — `s` is the resolved-messages binding the established pattern uses.)
- [ ] 4.3 Manual verification in a browser under `language = ru`: heading, description, both link texts, the empty state, and the document title render in Russian.

## 5. Fan-out — `/my/api-keys`

- [x] 5.1 Add `web/src/lib/components/ApiKeysView.messages.ts` (en/ru) covering every literal in `ApiKeysView.svelte`: the intro paragraph split around its two links and the header code sample, the one-time key-reveal box, the create form (including the four expiry option labels, which were a module constant), every `formError` string, the provider re-auth block, the list's relative-time line, and the revoke dialog. The `Authorization: Bearer <key>` sample and the CLI command stay English in every locale — they are protocol, not prose.
- [x] 5.2 Migrate `ApiKeysView.svelte` and the `<title>` in `web/src/routes/my/api-keys/+page.svelte`. `expiryOptions` became `$derived` so the labels follow the locale; `days` is still what the form binds to.
- [ ] 5.3 Manual verification under `language = ru`, including the create-key flow's reveal step and a revoke confirmation.

## 6. Fan-out — `/my/contributions`

- [x] 6.1 Add `web/src/lib/components/ContributeView.messages.ts` (en/ru) covering the "Contribute a board" heading and its explanatory paragraph, the URL field's label, the submit button's idle/"Checking…" states, the malformed-link error, and the contribution-list line (including the "via <surface>" connective; the surface itself is a wire token).
- [x] 6.2 Migrate `ContributeView.svelte` and the `<title>` in `web/src/routes/my/contributions/+page.svelte`.
- [x] 6.3 (added — not foreseen) The outcome sentence, which is the page's whole answer, lives in `$lib/intakeOutcome.ts` and is shared with the public search box. Leaving it English would have put the one line that matters in the wrong language. Moved its five messages into a catalog, gave `intakeOutcomeMessage(resolved, locale = 'en')` an optional locale so the public caller needs no change, and translated `IntakeOutcome.svelte`'s two links via their own catalog. Extended `intakeOutcome.test.ts`: the English default, Russian for all five outcomes with none collapsed onto another, and the fallback for an untranslated locale.
- [ ] 6.4 Manual verification under `language = ru`, including submitting a malformed link to see the error path translated.

## 7. Fan-out — `/my/plan`

- [x] 7.1 Add `web/src/lib/components/PlanView.messages.ts` (en/ru) covering the plan strip, the subscription block (including the billing interval and the five provider statuses, both of which were printed as raw wire tokens), today's allowances and their feature labels, the beta gateway-activity panel, and the history section.
- [x] 7.2 Migrate `PlanView.svelte` and the `<title>` in `web/src/routes/my/plan/+page.svelte`. The three token→label maps became lookups over the catalog, each falling through to the raw token for a value we have no words for — the behaviour the English `?? id` already had.
- [x] 7.3 (added — not foreseen) `{count} model call(s)` cannot be translated by a catalog alone: Russian takes three plural forms and 21 takes the singular while 11 does not. Added `plural()` over `Intl.PluralRules` and `plurals()`, which marks a form set as a catalog LEAF so a translation may carry categories the English source has no word for — see design.md. Merging instead of replacing would leave English's `other` under a Russian noun; both are tested.
- [ ] 7.4 Manual verification under `language = ru` on both a free and a Pro account if one is available; otherwise note which state was not observed.

## 8. Fan-out — `/my/referrals`

- [x] 8.1 Add `web/src/lib/components/ReferralsView.messages.ts` (en/ru) covering the three tab labels, the requests table's four columns and status pills, the offer form's fields/hints/validation, the five `ApiError`-mapped submit errors, the offer statuses (printed raw before), the incoming inbox's actions and the anonymity footnote, and the withdraw dialog.
- [x] 8.2 Migrate `ReferralsView.svelte` and the `<title>`/heading in `web/src/routes/my/referrals/+page.svelte`. The tab strip iterated a literal `[id, label]` pair list; it now iterates the existing `tabs` array and reads its labels from the catalog, which removes the second place a tab was named. Renamed a local `t` in `readTab()` — it shadowed the catalog resolver.
- [ ] 8.3 Manual verification under `language = ru` across both tabs, including at least one empty state.

## 9. Fan-out — `/my/activity`

- [x] 9.1 Add `web/src/lib/components/activity.messages.ts` (en/ru) for the layout's heading and tablist, the four tab labels, all five `<title>`s, and each view's error/empty states. **One catalog, not the five the design anticipated**: the four views carry two or three strings each, are reachable from nowhere else, and their empty states are written to read as a set — splitting them per component would spread eight lines over five files and let the set drift. It lives in `$lib` rather than under the route because a `$lib` component may not import from `routes/`.
- [x] 9.2 Migrate the four sibling views (`AnalysesView`, `Hidden`, `JobHistory`, `SavedJobs`), the layout, and the four child pages. Confirmed each view is reachable only from `/my/activity`, so no path-gating argument is needed. `AnalysesView`'s "Closed"/"Stale" badges and the `Unknown company` logo fallback were literals too.
- [ ] 9.3 Manual verification under `language = ru` across all four tabs, including each tab's empty state.

## 10. Wrap-up

- [ ] 10.1 Run `pnpm --dir web check`, `pnpm --dir web lint` and `pnpm --dir web test`; fix anything this change introduced. (`lint` covers eslint; CI additionally runs oxlint.)
- [ ] 10.2 Re-read `design.md`'s Risks / Trade-offs and confirm none turned into something needing code: the four-call-site migration, the nav guard's deliberate narrowness, the unreviewed Russian copy, and `/my/activity`'s shared components.
- [ ] 10.3 Update freehire#2005 with what shipped and what is still outstanding. Lead with the DATE gap: every translated line that composes a catalog prefix with `timeAgo`/`toLocaleDateString(undefined, …)` now reads half-English — «Создан 2 days ago», «истекает in a month» — on six pages instead of one, and it is the first thing a Russian reader notices. Then the remaining pages, `/my/profile`'s eight tab views, `JobRow`, the server-composed strings, and the `es`/`pt`/`de`/`fr` copy the reshaped API now allows.
