## Context

The inbox reading pane (`InboxView.svelte`) renders one of three link states from
the selected email's `linked_slug` / `suggested_slug` fields:
- **linked** → "Linked to X ↗" + Unlink
- **suggested** → "Looks like X · Link · Not this"
- **neither** → nothing (the gap this change fills)

All mutations go through `api.linkEmail(id, slug)` / `unlinkEmail` /
`confirmEmailLink` / `rejectEmailLink`, each returning the refreshed `EmailBody`;
`applyLinkUpdate` writes it back into `selected` and the list. The caller's
applications are available via `api.listTracked` (`GET /me/tracking`).

## Goals / Non-Goals

**Goals:**
- Let the user link an unlinked email to one of their tracked applications.
- Make Unlink reversible with a one-click Undo to the same application.
- Reuse the existing endpoint/client; no backend change.

**Non-Goals:**
- Changing the link of an already-linked email in place (Unlink → link again
  covers it).
- Linking to a job the user does not track (picker is scoped to tracked apps).
- Persisting Undo across reloads (it is a transient in-session affordance).

## Decisions

- **Picker is its own component** `ApplicationLinkPicker.svelte`: given the
  tracked applications, it renders a searchable list and emits the chosen slug.
  `InboxView` owns the data fetch (once, lazily) and the `api.linkEmail` call, so
  the picker stays a pure presentational unit with a clear interface (props in,
  `onpick(slug)` out).
- **Undo is client-side state**, no backend change. When `unlink()` runs,
  `InboxView` stashes the just-unlinked `{id, slug, company}` in a
  `lastUnlinked` rune. The neither-state row shows "Unlinked · Undo" while
  `lastUnlinked` matches the selected email; Undo calls
  `api.linkEmail(id, lastUnlinked.slug)` and clears the stash. Selecting another
  email or a fresh link clears it, so Undo never targets the wrong application.
- **Picker vs Undo coexist:** in the neither-state, if `lastUnlinked` matches show
  "Unlinked · Undo" (with the picker still reachable to link elsewhere);
  otherwise show just the "Link to application" picker.
- **Empty tracked list** → the picker shows "No applications yet — track a job
  first" instead of an empty menu.

## Risks / Trade-offs

- Undo is in-memory only: navigating away loses it. Acceptable — re-linking via
  the picker is always available as the durable path.
- `listTracked` is paginated; the picker fetches the first page and filters
  client-side. If a user tracks more than one page of applications the search box
  may not reach the tail — acceptable for now; noted as a future enhancement (log
  nothing silently in UI, the picker's search hints "showing your applications").
