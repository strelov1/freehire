## Context

Notification-related UI is split across three unconnected `my/*` routes today:
`/my/notifications` (account-level reminder/nudge settings), a link off it to
`/my/notifications/history` (full delivery history), and a separate sidebar
entry `/my/searches` (per-saved-search alert management, unrelated by URL but
conceptually the same "what am I told and how" concern). Each already exists
as a working page/component — this is a pure reorganization into one tabbed
section with real, bookmarkable routes, not a client-side view switch. No
backend, data model, or API change is involved.

Full prior write-up: `docs/superpowers/specs/2026-08-13-notification-center-tabs-design.md`.

## Goals / Non-Goals

**Goals:**
- One section, `/my/notifications`, with three real sub-routes sharing a tab
  strip: history (landing), search alerts, settings.
- Old URLs (`/my/searches`, `/my/notifications/history`) keep working via
  redirect — no broken bookmarks, no lost inbound links.
- Reuse every existing component/page body unchanged in content — only routes,
  the shared layout, and nav entries move.

**Non-Goals:**
- No change to notification frequency, delivery timing, or a profile timezone
  field (separate, already-scoped follow-up).
- No new "profile match" notification kind (separate, already-scoped
  follow-up).
- No change to what `SavedSearchesView`, `ReminderSettings`, or the history
  list actually render or do — only where they live.

## Decisions

- **Route-driven tabs, not local `$state`.** The design system's `TabStrip`
  (`design-system/src/tab-strip.svelte`) is a controlled component — it takes
  `active` + `onSelect` and has no opinion on what drives them. Every existing
  call site (`/my/profile`, `/my/market-pulse`) uses local `$state`, but this
  section's three views are already meant to be independently linkable (a
  "manage alerts" link from a saved-search card, a future email footer link,
  etc.), so `onSelect` calls `goto(resolve(...))` and `active` derives from
  `page.url.pathname` instead. This is a new usage pattern for `TabStrip` but
  not a new component — no design-system change needed.
- **History becomes the landing page (`/my/notifications`), not settings.**
  Settings moves to `/my/notifications/settings`. History is the more
  frequently useful default (what was I sent) versus settings (rarely
  touched after first setup), and it mirrors the email-inbox mental model the
  proposal is explicitly modeling this on.
- **Redirect via `+page.ts`, not a rewritten nav-only change.** Two retired
  routes (`/my/searches`, `/my/notifications/history`) get a 308 redirect
  file, following the existing convention (`web/src/routes/my/+page.ts`).
  Confirmed no other code path assumes GET on the retired path returns page
  content rather than a redirect.
- **`/my/notifications/[id]/jobs` stays outside the tab set.** It is a
  drill-down from a history card, not a fourth tab. Under the shared layout it
  falls back to the `history` tab visually active (closest-matching prefix) —
  simplest correct behavior without inventing a fourth tab id nobody
  navigates to directly.

## Risks / Trade-offs

- [Risk] Anything hardcoding `/my/searches` or `/my/notifications/history` as
  a literal string (not via `resolve()`) elsewhere in the SPA silently 404s
  post-move if missed. → Mitigation: grep the whole `web/src` tree for both
  literal path strings before removing the old page files, not just the
  routes tasks.md lists.
- [Risk] `saved-searches` and `account-navigation` specs hardcode the old
  paths/labels; skipping their delta specs would leave `openspec/specs/`
  contradicting the shipped app. → Mitigation: both are listed as Modified
  Capabilities with delta specs in this change.

## Migration Plan

No data migration. Deploy is a single web-only release: merge the route
moves, the two redirect files, and the nav-list edit together (splitting them
would leave the sidebar linking to a route that no longer renders its old
content, or a duplicate/missing nav entry, for the gap between deploys).

## Open Questions

None — scope and approach were confirmed with the user before this change was
proposed (see the referenced design doc).
