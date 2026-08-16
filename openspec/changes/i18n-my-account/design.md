## Context

See proposal.md - Why. Relevant existing plumbing this design builds on
(all unchanged by this work):

- `internal/accounts` / `migrations/0102_user_language.sql`: `users.language`,
  NOT NULL DEFAULT `'en'`, CHECK-constrained to `en`/`ru`/`es`/`pt`/`de`/`fr`.
- `web/src/lib/api.ts` `updateLanguage()` → `PATCH /me/language`.
- `web/src/lib/auth.svelte.ts` `updateLanguage()`: calls the API, then
  `invalidateAll()` so `page.data.user` re-resolves — the same mechanism
  already used for `currentUser()`/role changes.
- `web/src/routes/+layout.server.ts`: resolves the signed-in user server-side
  once per full load (forwarding the session cookie), returns `{ user }` (or
  `{ user: null }`, never an error) — deliberately per-request, not a
  module-level singleton, "so a module-level `$state` singleton would leak
  one request's user into another's SSR" (existing comment).
- `web/src/hooks.server.ts`: currently sequences only Sentry's handle and a
  `cacheControl` handle (sets `Cache-Control`/`Vary: Cookie`, private for
  session-tied HTML).
- `web/src/app.html`: static `<html lang="en">` today, never templated.
- Stack: SvelteKit ^2.70 (Svelte 5 runes), Vite ^8, `@sveltejs/adapter-node`
  (full SSR). CSP has no `unsafe-inline` script-src.

## Goals / Non-Goals

**Goals:**
- Resolve a per-request locale for `/my/**` from the existing `users.language`
  value, correct on the first SSR byte, with no new network round trip.
- Provide a small, typed, synchronous message-lookup mechanism usable from
  Svelte components without an async loading state.
- Keep every non-`/my/**` route's behavior byte-for-byte unchanged.

**Non-Goals:**
- Migrating any `/my/**` page beyond `/my/security` and the shared shell/nav
  (separate follow-up change).
- Reviewing/refining Russian copy quality (separate follow-up).
- Locale-prefixed URLs, `Accept-Language` detection, translation-management
  tooling, or a hardcoded-string lint guard (see proposal.md).

## Decisions

### Hand-rolled catalog instead of a library (svelte-i18n / Paraglide)

The scope is one route subtree (~20 files today, 3 in this change), so a
compile-time compiler's tree-shaking/bundling payoff (Paraglide) doesn't pay
for its added build step and generated-output surface. svelte-i18n's core
mechanism is a module-level reactive store initialized asynchronously — the
same failure shape (`page.data`-vs-module-singleton) the codebase already
documents avoiding for `currentUser()`, and it requires a flash-of-
untranslated-content guard this design doesn't need. A ~100-line
`defineMessages`/`t()`/`locale()` module, following the existing
`page.data`-reading idiom, is proportionate and still typed and
fallback-safe — not a shortcut.

Alternative considered: do nothing until more pages are ready and batch the
infra with the first real page. Rejected — Phase 0 infra has zero user-visible
effect on its own and is cheap to land and verify (`<html lang>` always
resolving to `en` pre-catalog) before building the first real page on top of
it.

### Locale carried via a dedicated `hire_lang` cookie, not re-deriving from the session on every request

`hooks.server.ts` runs before any load function, so it cannot itself call
`.me()` (that would double the session round trip already performed by the
root layout, on every request site-wide). Instead: the root
`+layout.server.ts`'s existing `.me()` call additionally computes `locale`
from `user.language` and syncs a dedicated, non-httpOnly `hire_lang` cookie
(`event.cookies.set`, not the auth session cookie), for the *next* request.
`hooks.server.ts` reads that cookie synchronously (no DB/network) to seed
`event.locals.locale` with a best-effort guess *for this request*.

That guess would still be wrong on the very first request of a session (no
cookie yet) if it were the only source `transformPageChunk` read — but
`event.locals` is one mutable object for the whole request. The root layout
load runs (as part of `resolve()`) before any HTML streams, so it overwrites
`event.locals.locale` with the fresh, authoritative value it just computed
from `user.language` — gated to `/my/**` the same way the hook itself gates
non-account routes. `transformPageChunk` reads `event.locals.locale` lazily,
after the load has run, not a value captured before it — so the very first
response already carries the correct `<html lang>`, with no extra network
round trip and no lag. (Caught in code review: an earlier version captured
the hook's pre-load guess into a closure, so the first-ever response could
show `<html lang="en">` with already-Russian content — see spec.md's "correct
on first byte" scenario, which this closes exactly.)

Alternative considered: read `page.data.user.language` only, no cookie,
resolving `<html lang>` client-side after hydration. Rejected — it would
render `<html lang="en">` on first byte even for an established Russian
preference, failing the "correct on first byte" requirement and not matching
how a returning user's session already looks stable end to end.

### Locale application is path-gated in the hook, not left to component-level opt-in

`hooks.server.ts`'s new handle only resolves a non-`en` locale when
`event.url.pathname` is `/my` or starts with `/my/`; every other path is
forced to `en` before `transformPageChunk` runs. This makes "public pages are
never translated" a structural property of one hook, not a convention each
new page's author has to remember.

The same gate must be applied a second time in `+layout.server.ts`, for
`page.data.locale` — the value `locale()`/`t()` actually read in every
component (`AccountNavRail`, `DeleteAccountButton`, ...), as opposed to
`event.locals.locale`, which only feeds `<html lang>`. **Caught in code
review**: an earlier version returned the ungated preference as page data, so
a Russian-preference signed-in user would see `AccountNavRail` (rendered on
the public, non-`/my/**` route `/tailor/[slug]`) in Russian, and the root
layout's `<html lang>` `$effect` would flip the public document to `lang="ru"`
client-side after hydration — silently defeating the hook's own gate for
every consumer that reads `page.data.locale` instead of `event.locals.locale`.
Fixed by computing the same `onAccountSection` check in `+layout.server.ts`
and gating the returned `locale` there too, so there is exactly one path-gated
value (mirrored into both `locals` and `page.data`), not two independently
gated ones that can drift apart.

### `Messages` leaves may be a `string[]`, not only a `string`

`main` merged a "Danger zone" / delete-account section onto `/my/security`
mid-task (#1996), whose confirmation dialog renders a bulleted list of what
gets erased via `{#each erased as item}`. A plain string leaf can't represent
a list without an unnatural join/split hack. `Messages` (`t.ts`) now allows
`string | string[] | Messages` per key; `deepMerge` treats an array as a leaf
(`Array.isArray` guards `isMessages`), so a translated list replaces the
English one wholesale rather than merging element-by-element — the same
whole-value-replacement semantics a string leaf already had.

### Root layout syncs `<html lang>` client-side on a live switch, not just at SSR

`hooks.server.ts`'s `transformPageChunk` only fires on an actual SSR response.
A live language switch (`updateLanguage()` → `invalidateAll()`) refreshes
`page.data.locale` without a full reload, so `<html lang>` would otherwise go
stale the instant the translated content flips. The root `+layout.svelte`
mirrors the existing `theme.svelte.ts`/`initTheme()` pattern (a small,
targeted DOM sync keyed off reactive state) with a `$effect` that sets
`document.documentElement.lang = page.data.locale` — Svelte 5 effects run
client-side only, so no `browser` guard is needed. Discovered during Phase 1
manual verification (curl-based SSR check first showed the attribute correct
on a fresh request but silently stale after a same-session client switch).

### `t()` is synchronous and per-key fallback, not per-locale fallback

`defineMessages(en, ru)` merges `ru` over a full copy of `en` at module-eval
time, so a partially-translated catalog (a key present in `en` but not yet in
`ru`) falls back to English for that one key rather than for the whole page.
This lets `/my/security`'s catalog ship even if a string is temporarily
untranslated, and lets `es`/`pt`/`de`/`fr` (no catalog file at all) render
100% English with the exact same code path — no special-casing per locale.

## Risks / Trade-offs

- [Resolved risk] A signed-in user's very first request after registration (or
  after changing language on a different, cookie-less device) could briefly
  show English on `/my/**` before the cookie synced. → No longer a risk: the
  `event.locals` refinement described under "Locale carried via a dedicated
  `hire_lang` cookie" makes the very first response correct too, not just
  "self-correcting within one more load."
- [Risk] Hand-rolled `t()` has no compile-time guarantee that every `en` key
  has a `ru` counterpart (TypeScript's structural typing on `Partial<T>`
  silently accepts a missing key). → Mitigation: acceptable at this scale
  (reviewed by hand in a small PR); explicitly deferred to a future lint
  check once the pattern is proven across more pages (see proposal.md).
- [Trade-off] No locale-prefixed URLs means locale can't be shared/bookmarked
  independent of the account, and two tabs signed into different accounts on
  the same browser share one `hire_lang` cookie value (last-request-wins).
  Acceptable: `/my/**` is single-account by nature (one session cookie), and
  the cookie re-syncs every request from the authoritative `users.language`
  value for whichever session is active.

## Migration Plan

No data migration (backend already ships `users.language`). Deploy is a
normal frontend release: Phase 0 infra lands with zero visible behavior
change (verify `<html lang="en">` still holds everywhere pre-catalog), then
the `/my/security` + shell/nav migration lands in the same or a following
release. Rollback is a normal revert — no persisted state depends on the new
code (`hire_lang` is derived, disposable).
