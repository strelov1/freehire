## Why

`users.language` (migrations/0102_user_language.sql) was added specifically
"ahead of interface i18n" (freehire#1836), and the frontend already lets a
signed-in user pick a language via `AccountLanguage.svelte` — but today that
preference only steers LLM output (assistant, voice interview, match
analysis). No UI string anywhere in the app is actually translated. The
authenticated account section (`/my/**`) has no SEO constraint (unlike public
job/company pages), so it is the natural place to make that preference
actually change what the user reads, starting with English + Russian.

## What Changes

- Add a locale-resolution layer that turns the existing `users.language`
  preference into a per-request locale for `/my/**` only: a `hire_lang`
  cookie synced from the root layout's existing session load, read
  server-side by a new `hooks.server.ts` handle that is path-gated to
  `/my/**` and fills a `%lang%` placeholder in `app.html`'s `<html lang>`.
  Every other route always renders `en`.
- Add a hand-rolled message-catalog mechanism (`web/src/lib/i18n/t.ts`): a
  `defineMessages(en, ru)` helper with per-key fallback to English, and a
  `locale()` reader that mirrors the existing `currentUser()` pattern (reads
  `page.data`, not a module-level store) so SSR stays per-request-safe. No
  new npm dependency.
- Migrate `/my/security` end-to-end as the reference page: every literal
  moves into a colocated `messages.ts` (`en`/`ru`), rendered through the new
  `t()` helper. Fix the now-stale "once interface translations ship" caveat
  in `AccountLanguage.svelte`'s copy. Includes the "Danger zone" delete-account
  section merged from `main` mid-task (#1996) — its own `DeleteAccountButton`
  component gets a colocated catalog too, since it renders exclusively there.
- Translate the shared account-section chrome (`my/+layout.svelte`,
  `AccountNavRail.svelte`, `accountNav.ts` labels) through the same
  mechanism, since every other `/my/**` page depends on it.
- Language switching (already-shipped `updateLanguage()` → `invalidateAll()`)
  now also flips the translated UI instantly, client-side, no reload.

Explicitly not included in this change: migrating the remaining `/my/**`
pages beyond security and the shared chrome, a Russian-copy review pass,
locale-prefixed URLs, public-page i18n, translation-management tooling,
`Accept-Language` auto-detection, and a lint/CI guard against new hardcoded
strings — all are noted as deliberate follow-ups, not built now.

## Capabilities

### New Capabilities
- `account-interface-i18n`: locale resolution (cookie sync, path-gated
  SSR/CSR locale, `<html lang>`), the message-catalog + `t()` rendering
  mechanism, and the scope boundary rule (only `/my/**` and its exclusive
  chrome are translatable; shared-with-public components stay English),
  demonstrated end-to-end on the `/my/security` page and the account-section
  shell/nav.

### Modified Capabilities
(none — `account-navigation`'s structural/gating requirements are unchanged;
this change only adds locale-aware rendering of its existing labels, which is
new capability behavior, not a change to what that shell already guarantees)

## Impact

- Affected code: `web/src/hooks.server.ts`, `web/src/routes/+layout.server.ts`,
  `web/src/app.html`, new `web/src/lib/locale.ts` and `web/src/lib/i18n/`,
  `web/src/routes/my/security/+page.svelte` (+ new `messages.ts`),
  `web/src/lib/components/AccountLanguage.svelte`,
  `web/src/routes/my/+layout.svelte`, `web/src/lib/components/AccountNavRail.svelte`.
- No backend/API/DB changes — `users.language`, `PATCH /me/language`, and the
  CHECK constraint's locale set are already correct and untouched.
- No new dependency.
