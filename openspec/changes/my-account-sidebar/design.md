## Context

The `my/*` account pages are standalone SvelteKit routes. Each `+page.svelte`
opens with its own `<div class="mx-auto w-full max-w-{3xl|6xl} px-4 py-6">`
wrapper, its own `!isAuthenticated()` sign-in gate, and its own
`<meta name="robots" content="noindex">`. `my/tracking` additionally has its own
`+layout.svelte` that repeats the container + gate + noindex and adds a
Board/Pipeline/History/AI-fit tab row. Navigation into the section exists only in
the header dropdown (`HeaderMenu.svelte`, "Account" links). There is no `/my`
index route, so a bare `/my` 404s.

The `tracking/+layout.svelte` already demonstrates the in-repo pattern this
change generalizes: a route layout that owns the section chrome and derives an
active tab from `page.url.pathname`.

## Goals / Non-Goals

**Goals:**
- One source of truth for the account area's chrome (container, `noindex`,
  auth-gate) and its navigation.
- Persistent in-section navigation across all `my/*` pages including Profile.
- Responsive: vertical sidebar at `lg`+, horizontal tab strip below `lg`.
- Preserve Tracking's existing sub-tabs unchanged (two navigation levels).
- Close the bare-`/my` 404.

**Non-Goals:**
- No collapsible/icon-only sidebar state or persisted collapse preference.
- No change to the header dropdown account links (they stay as entry points).
- No backend/API/DB changes; frontend only.
- No moving page `<h1>`/headers into the layout — pages keep their own headers
  (several carry subtitles and action buttons).

## Decisions

**Route layout as the shell (over a shared wrapper component or nav-only
layout).** A new `my/+layout.svelte` owns the container, `noindex`, auth-gate,
and navigation; children render only their header + body. SvelteKit nests
layouts, so `my/+layout` → `tracking/+layout` → page composes naturally, giving
the two navigation levels (sidebar → tabs) with no special-casing. Alternatives:
a nav-only layout leaves every page still centering itself and re-declaring its
gate (double-centering, duplicated gates); an imported `<AccountShell>` component
pushes boilerplate into every page versus one idiomatic layout.

**Vertical sidebar at `lg`, horizontal strip below `lg` (not `md`).** Profile's
coverage tab has its own internal `md:block w-72` filter aside; putting the outer
sidebar at `md` would cramp Profile at mid widths. Gating the vertical sidebar at
`lg` leaves Profile room while still giving the sidebar on typical desktop
widths. Below `lg` the navigation is a horizontal `overflow-x-auto` strip above
the content.

**Centralize the auth-gate (accepted behavior change).** The layout renders the
shell only when authenticated and shows a single sign-in prompt otherwise,
replacing the five per-page prompts. This unifies the sign-in copy across the
section — an intentional, user-approved change — and lets each page assume it
renders only when signed in. Active-item logic reuses the existing
`path === href || path.startsWith(href + '/')` idiom from `HeaderMenu`/
`tracking-layout`.

**`/my` redirect via `+page.ts` load.** A `redirect(308, '/my/tracking')` in a
route `load` mirrors the existing `my/notifications` and `my/jobs/[...path]`
redirect-only routes. Redirect-only children short-circuit in `load` before
rendering, so the shell wrapping them is harmless.

**Width handling.** The shell owns `mx-auto w-full max-w-6xl px-4 py-6`. Pages
drop their outer wrapper. Narrow reading pages (searches, api-keys, submissions)
keep an inner `max-w-2xl/3xl` on their body to preserve line length within the
content column; wide pages (recommendations, tracking, profile) fill the column.

## Risks / Trade-offs

- [Unified auth-gate loses per-page sign-in copy] → Approved by the user; the
  single prompt covers the whole section and keeps a Sign-in action.
- [Nested asides on Profile's coverage tab at `lg`] → The outer sidebar is
  `w-56`; the inner filter aside is itself optional (`hidden md:block`) and only
  on the coverage tab. Content stays usable at `lg`, comfortable at `xl`.
- [Refactor touches every leaf page] → Mechanical, per-page edits (remove
  wrapper/gate/noindex), each verified independently; redirect-only routes
  untouched.
- [SSR renders signed-out shell first] → Matches the existing `tracking/+layout`
  pattern (`isAuthenticated()` is a client store); acceptable on `noindex`
  personal pages.

## Migration Plan

Pure frontend, no data migration. Ships as a normal `web/` deploy. Rollback is
reverting the change; no state to unwind.

## Open Questions

None outstanding — scope confirmed during brainstorming (all `my/*` including
Profile; sidebar = top level with Tracking tabs retained; mobile = horizontal
strip; auth-gate unified).
