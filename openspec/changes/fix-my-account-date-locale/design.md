## Context

`web/src/lib/utils.ts` exports four date-formatting helpers — `formatDate`,
`formatDateTime`, `timeAgo`, `formatDateOrAgo` — all built on
`Intl.DateTimeFormat`/`Intl.RelativeTimeFormat` called with `undefined` as
the locale argument, which resolves to the browser/runtime default. See
`proposal.md` for why that is wrong in both directions (translated `/my/**`
pages, mirror-image public pages).

The page's own resolved locale already exists and is already threaded
everywhere a component needs it: `locale()` in
`web/src/lib/i18n/currentLocale.svelte.ts` reads `page.data.locale`
(`Locale`, typed in `app.d.ts`), set once per request by
`web/src/routes/+layout.server.ts` — `'en'`/`'ru'` on `/my/**` per the
account's `language` (gated through `isTranslatedLocale`/`TRANSLATED_LOCALES`),
`'en'` unconditionally everywhere else. This change reuses that value as-is;
it introduces no new locale-resolution logic.

## Goals / Non-Goals

**Goals:**

- Every call site of the four `utils.ts` helpers passes the page's resolved
  locale explicitly.
- The compiler catches any call site this change misses or any new call site
  added later without a locale.
- `timeAgo`'s formatter cache stops being invalid once locale can vary within
  a session.

**Non-Goals:**

- Extending `TRANSLATED_LOCALES` or formatting dates in `es`/`pt`/`de`/`fr`
  for accounts set to those languages. `page.data.locale` collapses those to
  `'en'` today (per `isTranslatedLocale`), same as UI text; giving dates a
  wider locale set than the UI they sit in — formatting a date in Spanish
  next to English prose — is a real product call for item 2 of freehire#2005
  (the actual es/pt/de/fr copy), not this change.
- Translating any new page. This change only touches how already-rendered
  dates are formatted; it adds no `messages.ts` catalogs and migrates no new
  component to `t()`.
- Reworking `NotificationCard`'s always-english-outside-`/my/**` behavior.
  It renders on every route via `TopBar`, and one could argue it should
  format in the *account's* language regardless of path, since it is always
  "my" content. But every other route already renders `NotificationCard`'s
  surrounding chrome in English by the same path gate, so following
  `locale()` like every other call site keeps it *consistent* with its own
  chrome rather than introducing a second, path-independent locale signal.
  Worth reopening only if `NotificationCard` itself gets translated UI text
  outside `/my/**`, which is not proposed here.

## Decisions

### `locale` becomes a required parameter, not an optional one with a fallback

Alternative considered: `locale: Locale = 'en'` (or reading `undefined` as
"use the runtime default," preserving current behavior for anyone who
forgets to pass it). Rejected — a silent fallback is exactly how the account
nav's three-sections-in-three-months drift happened (see the merged
`i18n-my-account-fanout` change's retro in freehire#2005): the per-key
fallback worked as designed and that is *why* it took three cycles to
notice. Making `locale` required turns every one of the ~20 call sites this
change must touch into a TypeScript error until fixed, so `tsc`/`svelte-check`
— which `pnpm --dir web check` already runs — is the completeness gate
instead of a reviewer's eyes.

### Call sites pass `locale()` directly; no new "date locale" concept

Alternative considered: a wider, path-independent "formatting locale" derived
straight from `user.language` (validated against `SUPPORTED_LOCALES`, not
`TRANSLATED_LOCALES`), on the theory that `Intl.DateTimeFormat` can format a
correct Spanish date without any Spanish UI text existing yet. Rejected for
*this* change: it would fix dates for `es`/`pt`/`de`/`fr` accounts ahead of
their UI translation — a genuine improvement, but a separate product
decision (a date in the account's language next to otherwise-English prose)
that changes the shape of item 2 in freehire#2005 rather than being a pure
bug fix. Reusing the existing `locale()` value keeps this change mechanical:
"the date now agrees with the text already on the page," never "the date
started disagreeing with the text in a new way." Revisit as an explicit
follow-up once the es/pt/de/fr copy work starts.

### `timeAgo`'s formatter cache keys on `${locale}:${style}`

The existing cache (`relativeTime: Partial<Record<TimeAgoStyle,
Intl.RelativeTimeFormat>>`) is keyed only by `style`, and its comment states
the invariant this change breaks: "the locale is the runtime default, which
cannot change within a process." Once `locale` is a caller-supplied
parameter, two calls in the same session with different locales (e.g. the
header notification bell on an `/my/**` page in `'ru'`, then the same
component's cached formatter reused on a `'en'`-locale public page after a
client-side navigation) must not collide. The cache becomes
`Partial<Record<\`${Locale}:${TimeAgoStyle}\`, Intl.RelativeTimeFormat>>`,
same construct-once-per-key rationale as before, just keyed on the pair.

### `formatDate`/`formatDateTime`/`formatDateOrAgo` do not cache a formatter

They already construct a fresh `Intl.DateTimeFormat` per call (via
`toLocaleDateString`/`toLocaleString`, not a module-level instance), so
adding a `locale` argument to them is a pure signature change with no cache
to rekey.

## Risks / Trade-offs

- **A missed call site is a build failure, not a silent bug** — the intended
  outcome of making `locale` required, but it does mean this change cannot
  land partially; every call site enumerated in `proposal.md`'s Impact
  section must move together, in one PR.
- **`ModerationView` and children are internal-only tooling** (moderator-role
  gated, `noindex`) formatted in English regardless of locale, same as
  today — this change does not add translation there, only makes the
  existing English-only behavior explicit (`locale()` resolves `'en'`
  outside `/my/**` including `/moderation`) instead of accidental
  (`undefined` happening to read close to English for most staff browsers).
- **No visual regression expected** — for any call site whose page already
  resolves to `'en'` (every public/moderation page, and any `/my/**` page for
  an English-language account), the rendered string is unchanged; the
  behavior change is scoped to `/my/**` pages under a translated
  non-English locale, which today is Russian only.

## Migration Plan

No data migration. Deploy as a single frontend PR — `locale` being a
required parameter means there is no intermediate state where some call
sites pass it and others don't compile. Rollback is a normal revert.
