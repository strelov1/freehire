## 1. Locale resolution primitives

- [x] 1.1 Add `web/src/lib/locale.ts`: `SUPPORTED_LOCALES = ['en','ru','es','pt','de','fr'] as const`, `type Locale`, `isLocale(v): v is Locale`, `LOCALE_COOKIE = 'hire_lang'`. Unit test `isLocale()` for valid/invalid/undefined/null input.
- [x] 1.2 Change `web/src/routes/+layout.server.ts`: when `user` resolves non-null, compute `locale = isLocale(user.language) ? user.language : 'en'`, sync it via `cookies.set(LOCALE_COOKIE, locale, { path: '/', httpOnly: false, sameSite: 'lax', maxAge: 60*60*24*365 })`, and return `{ user, locale }` (anonymous stays `{ user: null, locale: 'en' }`, no cookie write).
- [x] 1.3 Change `web/src/hooks.server.ts`: add a `locale` handle, sequenced before `cacheControl`, that reads the `hire_lang` cookie, validates with `isLocale`, falls back to `'en'`, and only keeps a non-`en` value when `event.url.pathname === '/my' || event.url.pathname.startsWith('/my/')` (forced to `'en'` on every other path). Set `event.locals.locale`. Call `resolve(event, { transformPageChunk: ({ html }) => html.replace('%lang%', locale) })`.
- [x] 1.4 Change `web/src/app.html`: `<html lang="en">` → `<html lang="%lang%">`.
- [x] 1.5 Verify Phase 0 has no visible behavior change: `<html lang="en">` still holds on both a public page and `/my/**` when no catalog exists yet (manual check via the `run` skill, or an SSR-response assertion in an existing route test if one already loads `+layout.server.ts`).

## 2. Message catalog + `t()` helper

- [x] 2.1 Add `web/src/lib/i18n/t.ts`: `defineMessages(en, ru)` merging `ru` over a full copy of `en` at module-eval time (per-key fallback, not per-locale); `t(catalog, locale)` returning the resolved strings object; `locale()` reading `page.data.locale` reactively (mirrors `currentUser()`'s `page.data` pattern — no module-level store).
- [x] 2.2 Unit test `defineMessages`/`t()`: a key present only in `en` falls back to English when `ru` is requested; a key present in both returns the `ru` value when `ru` is requested; `locale === 'en'` always returns `en`.

## 3. Reference migration — `/my/security`

- [x] 3.1 Add `web/src/routes/my/security/messages.ts` with `en`/`ru` covering every literal in `web/src/routes/my/security/+page.svelte`: page title/subtitle, password section (heading, subheading, no-password notice, field labels, saving/save/changed states), the three `messageFor()` error strings, sessions section (heading, subheading, signing-out/sign-out), and the `<svelte:head>` title.
- [x] 3.2 Migrate `+page.svelte` to render through `messages.ts` + `t()`/`locale()`: template reads `{s.title}` etc., `messageFor(e)` returns catalog values instead of literals. No change to control flow, API calls, or markup structure.
- [x] 3.3 Fix `web/src/lib/components/AccountLanguage.svelte`'s stale copy: drop the "once interface translations ship" caveat now that a real page is translated.
- [x] 3.4 Manual verification: sign in, set language to Russian via `/my/profile`'s picker, open `/my/security`, confirm every string is Russian and `<html lang="ru">`; switch back to English and confirm it flips without a full reload; visit `/jobs` in the same session and confirm it stays English with `<html lang="en">`.
- [x] 3.5 Manual verification: set language to `es` (or any of `pt`/`de`/`fr`) via the picker, open `/my/security`, confirm it renders fully in English (no blank/missing text).

## 4. Shared account-section chrome

- [ ] 4.1 Add `web/src/lib/i18n/shell.ts` with `en`/`ru` catalogs for `web/src/routes/my/+layout.svelte`'s own chrome strings and `web/src/lib/accountNav.ts`'s navigation item labels (keyed by `item.href`, `accountNav.ts` itself stays free of any i18n import).
- [ ] 4.2 Migrate `my/+layout.svelte` to render its own chrome strings through `shell.ts` + `t()`/`locale()`.
- [ ] 4.3 Migrate `web/src/lib/components/AccountNavRail.svelte` to look up each nav item's translated label from `shell.ts` by `item.href`, falling back to `item.label` if a key is missing.
- [ ] 4.4 Manual verification: with language set to Russian, confirm every `/my/**` page's section navigation renders Russian labels (spot-check two or three sections), and that the shell's own chrome text (if any) is translated too.

## 5. Wrap-up

- [ ] 5.1 Run `pnpm --filter freehire-web check`, `pnpm --filter freehire-web lint`, `pnpm --filter freehire-web test` and fix any failures introduced by this change.
- [ ] 5.2 Re-read `design.md` Risks/Trade-offs and confirm none require a code change before finishing (cookie self-heal timing, missing-`ru`-key fallback, single shared cookie across tabs/accounts) — all are accepted trade-offs, no action expected.
