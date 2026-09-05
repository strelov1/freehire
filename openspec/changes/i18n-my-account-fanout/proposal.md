## Why

`i18n-my-account` deliberately translated one page (`/my/security`), the shared
shell/nav, and the `/my/profile` tab-strip labels, and named the rest a
follow-up (freehire#2005). Two things have gone stale in the fortnight since.

First, the mechanism is shaped for exactly two locales. `defineMessages(en, ru)`
takes the Russian catalog as its second **positional** argument, so `es`, `pt`,
`de` and `fr` — which `users.language` already accepts and the account language
picker already offers — have nowhere to go. Reshaping that signature costs four
files today. After the fan-out it costs every catalog in the account section.

Second, the nav catalog has already drifted, three sections deep.
`i18n-my-account` landed on 2026-08-16 with all 14 nav labels translated;
`accountNav` now lists 17. `/my/integrations` arrived two days later (#2138,
`e01ce59a`), `/my/lists` and `/my/webhook` since — none of them with a label.
A Russian user reads "Integrations", "Job lists" and "Webhook" in an otherwise
Russian sidebar. Nothing failed, because the per-href fallback is working
exactly as designed — which is precisely why the drift was invisible, and
precisely why it happened three times rather than once. Two more labels are
simply wrong: "My submissions" (jobs the user submitted for review) reads
"Мои публикации", and "Contributions" (paste a job link so we crawl a new
board) reads "Вклад".

## What Changes

- Reshape `defineMessages` from a two-locale positional signature to
  `defineMessages(en, { ru, es, pt, de, fr })` — a partial map keyed by locale,
  with `en` staying positional so TypeScript keeps inferring each translation's
  `DeepPartial` shape from it. `t(catalog, locale)` resolves
  `catalog[locale] ?? catalog.en`, so a locale with no entry costs nothing:
  no merged copy is built for it. Migrate the four existing catalogs.
  This changes the API's shape only — no locale gains or loses a translation.
- Add a guard test over the nav catalog: every `accountNav` href SHALL have an
  `en` and a `ru` label, and the catalog SHALL carry no href that `accountNav`
  no longer has. `es`/`pt`/`de`/`fr` are deliberately exempt.
- Fix the nav-label defects: add the three missing sections
  (`/my/integrations`, `/my/lists`, `/my/webhook`), and correct
  "Мои публикации" → "Мои вакансии" and "Вклад" → "Добавить борд".
  Settle one wording inconsistency on `/my/security` ("сеансы" in the subtitle
  vs. "Сессии" as the section heading below it).
- Fan out en/ru to the first batch of account pages — the small,
  self-contained ones, which prove the pattern across list, form and
  empty-state shapes without a 1500-line diff: `/my/submissions`,
  `/my/api-keys`, `/my/contributions`, `/my/plan`, `/my/referrals`,
  `/my/activity`. Each page's `<svelte:head><title>` moves into its catalog
  too — those are hardcoded English today even on the pages already translated.

Explicitly not included: actual `es`/`pt`/`de`/`fr` copy (this change only makes
the API able to hold it — the issue's own sequencing puts the translation pass
after a stable English source); `/my/profile`'s eight tab views; the remaining
larger pages (`/my/tracking`, `/my/inbox`, `/my/market-pulse`, `/my/assistant`,
`/my/cvs`, `/my/notifications`); locale-prefixed URLs; public-page i18n; a
lint rule against new hardcoded strings.

## Capabilities

### Modified Capabilities
- `account-interface-i18n`: the catalog mechanism accepts translations for any
  supported locale rather than exactly one; the account-section navigation
  gains a completeness guarantee (no nav item may ship without a translated
  label); `<svelte:head>` page titles under `/my/**` join the translated
  surface; and the translated page set grows by six sections.

## Impact

- Affected code: `web/src/lib/i18n/t.ts` (signature), its four call sites
  (`web/src/lib/i18n/shell.ts`, `web/src/routes/my/security/messages.ts`,
  `web/src/routes/my/profile/messages.ts`,
  `web/src/lib/components/DeleteAccountButton.messages.ts`), a new
  `web/src/lib/i18n/shell.test.ts`, a `$lib` alias in `web/vitest.config.ts`
  (that config deliberately omits the SvelteKit plugin, so a module importing a
  *value* through `$lib` could not be loaded from a test at all — `i18n/t.ts`
  passed only because its `$lib` import is `import type` and erases), and —
  per page — a new colocated catalog
  plus the render migration in `MySubmissionsView.svelte`,
  `ApiKeysView.svelte`, `ContributeView.svelte`, `PlanView.svelte`,
  `ReferralsView.svelte`, `AnalysesView.svelte`, `Hidden.svelte`,
  `JobHistory.svelte`, `SavedJobs.svelte` and their `+page.svelte` wrappers.
- No backend/API/DB changes. `users.language`, `PATCH /me/language` and the
  CHECK constraint's locale set are already correct and untouched.
- No new dependency.
- Note for whoever archives this: `i18n-my-account` is still in
  `openspec/changes/` (not `changes/archive/`) and its
  `account-interface-i18n` capability was never synced into `openspec/specs/`.
  That predates this change and is left alone here.
