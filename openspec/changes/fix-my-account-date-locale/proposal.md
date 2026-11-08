## Why

`web/src/lib/utils.ts`'s date-formatting helpers (`formatDate`, `formatDateTime`,
`timeAgo`, `formatDateOrAgo`) all call `Intl.DateTimeFormat`/
`Intl.RelativeTimeFormat` with `undefined` as the locale, so they render in
whatever locale the visitor's browser/OS reports — never the page's own
resolved locale. This produces mixed-language strings in both directions:

- On `/my/**` pages already translated into Russian (issue #2005, PR #2477),
  every date-bearing line reads half-English: «Создан 2 days ago», «истекает
  in a month». It is the first thing a Russian reader notices, on six pages
  today and growing with every further fan-out page.
- On public/moderation pages, whose UI text is always English by design, a
  non-English-locale browser gets the mirror-image bug: an English `/jobs/*`
  page showing "3 дня назад" to a Russian visitor.

Fixing this now, before the remaining `/my/**` fan-out lands, avoids adding
the same bug to every newly translated page and then having to revisit it.

## What Changes

- `formatDate`, `formatDateTime`, `timeAgo` and `formatDateOrAgo` in
  `web/src/lib/utils.ts` take a **required** `locale` parameter (no default).
  Making it required — not optional with a fallback — is deliberate: it turns
  every unmigrated call site into a compile error, the same completeness
  guarantee the account-nav label test already uses, rather than leaving a
  silent gap a reviewer has to notice.
- `timeAgo`'s module-level `Intl.RelativeTimeFormat` cache, currently keyed
  only by `style` (and documented as safe *because* the runtime locale cannot
  change mid-session — an invariant this change removes), is keyed by
  `${locale}:${style}` instead.
- Every call site passes the page's already-resolved locale explicitly, via
  the existing `locale()` accessor (`web/src/lib/i18n/currentLocale.svelte.ts`,
  backed by `page.data.locale`) — no new locale-resolution mechanism. That
  accessor already resolves to `'en'` outside `/my/**` and to `'en'`/`'ru'`
  inside it (`web/src/routes/+layout.server.ts`), so threading it through
  fixes both directions of the bug with one rule instead of two.
- Call sites: 8 `/my/**` components (`BoardList`, `JobDrawer`,
  `WebhookSettingsView`, `ApiKeysView`, `InboxView`, `MySubmissionsView`,
  `ReferralsView`, `ContributeView`), the header `NotificationCard`/
  `NotificationBell` (renders on every route via `TopBar`), and the
  public/moderation call sites (`JobRow`, `JobView`, `lib/ghost.ts`,
  `RecentJobsFeed`, `CompanyFeedbackListDialog`, `StatusBoard`,
  `ModerationView` and its four children).
- **BREAKING** (internal-only — a TypeScript function signature, not a public
  API or wire contract): callers of the four `utils.ts` functions must be
  updated in the same change; there is no deprecation window because the
  compiler itself is the migration mechanism.

## Capabilities

### New Capabilities

- `date-locale-formatting`: date and relative-time strings rendered anywhere
  in the application resolve to the same locale as the surrounding page's UI
  text, never the visitor's browser/runtime default.

### Modified Capabilities

(none — `account-interface-i18n` has no synced spec under `openspec/specs/`
yet; it exists only as an unmerged delta inside the still-unarchived
`i18n-my-account` / `i18n-my-account-fanout` changes, both already shipped to
prod. That bookkeeping gap is pre-existing and out of scope here.)

## Impact

- `web/src/lib/utils.ts` — the four function signatures and the
  `Intl.RelativeTimeFormat` cache.
- `web/src/lib/utils.test.ts` — needs locale-aware coverage (same instant
  formatted under `'en'` vs `'ru'`), since no existing test asserts on a
  locale argument today.
- ~13 component/route files listed above, each updated to pass `locale()`
  explicitly, plus 4 more found only by `pnpm --dir web check` and a
  follow-up repo-wide grep rather than by the initial code-reading pass:
  `lib/components/community/DiscussionThread.svelte`,
  `DiscussionFeed.svelte`, `ReplyNode.svelte`, `DiscussionIndex.svelte`
  (the public discussion-thread routes) — see tasks.md 5.1.
- No backend, API, database, or public-contract changes.
