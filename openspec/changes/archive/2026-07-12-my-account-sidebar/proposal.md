## Why

The personal `my/*` pages (Profile, Tracking, Recommendations, Saved searches,
API keys, Submissions) are standalone: each owns its width container and
auth-gate, and the only way to move between them is the header dropdown menu.
There is no persistent in-section navigation, so the account area does not read
as one place, and a bare `/my` URL 404s. A centralized sidebar gives the section
a single navigational home and a single source of truth for its chrome.

## What Changes

- Add a `my/+layout.svelte` shell that owns the account area's chrome for every
  `my/*` route: the width container, the `noindex` head tag, a unified auth-gate
  (one sign-in prompt for the whole section), and the section navigation.
- The navigation renders as a **vertical sidebar at `lg` and up** and a
  **horizontal, scrollable tab strip below `lg`** — six items: Profile,
  Tracking, Recommendations, Saved searches & alerts, API keys, My submissions.
  The active item is derived from the current path.
- Tracking keeps its existing Board/Pipeline/History/AI-fit sub-tabs unchanged:
  the sidebar is the top navigation level, the tabs are the sub-view level.
- Add a `/my` index redirect to `/my/tracking` (closing the current bare-`/my`
  404).
- Refactor `my/tracking/+layout.svelte` and the five leaf pages (`profile`,
  `recommendations`, `searches`, `api-keys`, `submissions`) to drop their own
  width container, per-page auth-gate, and per-page `noindex` — now owned by the
  layout. Each page keeps its `<title>`, its `<h1>` header, and its body.
- Header dropdown account links are unchanged (they stay as entry points into
  the section).

## Capabilities

### New Capabilities
- `account-navigation`: the persistent in-section navigation and shared chrome
  (width container, auth-gate, `noindex`) for the signed-in `my/*` account area,
  including its responsive sidebar/tab-strip behavior and the `/my` redirect.

### Modified Capabilities
<!-- None: header-navigation's account links stay unchanged; this is a new,
     separate in-section navigation surface. -->

## Impact

- Frontend only (`web/`). No backend, API, or DB changes.
- New: `web/src/routes/my/+layout.svelte`, `web/src/routes/my/+page.ts`.
- Modified: `web/src/routes/my/tracking/+layout.svelte` and the `+page.svelte`
  of `profile`, `recommendations`, `searches`, `api-keys`, `submissions`.
- Behavior change: the per-page sign-in prompt copy is unified into one gate.
- `notifications` and `jobs/[...path]` (redirect-only routes) are untouched.
