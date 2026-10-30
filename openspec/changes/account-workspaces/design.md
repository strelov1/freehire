## Context

A throwaway spike (2026-10-30, `/tmp/spike-cabinet`) rendered the grouped sidebar and the
switcher against the live API and confirmed the shape works without a backend change.

## Decisions

**The workspace comes from the URL, not from stored state.** A shared link, a reload and the
back button all land in the right workspace with nothing to keep in sync. The mapping is a
property of each nav item (`workspace`), so `workspaceFor(path)` is "the workspace of the
most specific item whose route contains the path", defaulting to Job search.

**Most specific match wins.** `/my/mentorship/schedule` sits under `/my/mentorship`; the old
prefix rule would light up both. The active item is the longest matching href.

**One flat list stays the source of truth.** `accountNav` keeps every item, now carrying
`workspace` and `group`; the sidebar, the drawer and the rail derive their view from it. The
icon record and the label catalog stay keyed by href, so a new item without an icon or label
still fails to compile / fails its test.

**The switcher is not a second copy of the sections.** The profile menu used to repeat the
sidebar item by item, which is why it was long. It now lists workspaces; the sections live in
the sidebar only.

**Mentor and Employer stay visible as today.** `/me` carries no "is a mentor" or "has a
claimed company" flag, so hiding a workspace until its role is active needs a payload change.
Noted as the seam for a later change rather than built now.

## Risks

- A phone user outside `/my` reaches Tracking in two taps (menu → Job search → drawer)
  instead of one. Accepted: the menu had grown to seventeen account rows above Log out.
