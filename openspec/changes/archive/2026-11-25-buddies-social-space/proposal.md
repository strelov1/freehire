## Why

Interview buddies is a list inside the account cabinet: three tabs, grey silhouettes and plain
rows. The owner approved a social-network redesign (mockup kept outside the repo,
`~/Documents/freehire-mockups/buddies-social-2026-10-30.html`). It needs room the cabinet
cannot give — the `/my` shell already spends a left column on its own navigation — and members
need to tell each other apart at a glance.

This change is the first slice: the space itself and the member profile. The feed, network and
chat redesigns follow as separate changes on top of it.

## What Changes

- A new signed-in space at **`/buddies`**, full width, with its own left navigation (Feed,
  Network, Chats) and a right rail. Pages: `/buddies` (feed), `/buddies/network` (the former
  Similar tab), `/buddies/chats` (the former Chats tab, `?c=<id>` opens a conversation),
  `/buddies/<handle>` (a member). Noindex, behind the same sign-in gate as `/my`.
- **`/my/buddies` and `/my/buddies/<handle>` redirect** to the new addresses, keeping the
  query, so links already sent in emails and notifications keep working. The unread-message
  notification links to `/buddies/chats?c=<id>`.
- A **generated avatar** for every member: two initials from the handle on a colour derived
  from the handle. It replaces the plain silhouette. It is derived from the anonymous handle
  alone, so it links to nothing outside the buddy network; a photo is still never shown.
- A redesigned **member profile**: avatar, heading and country, handle, the member's year in
  numbers (applications, interviews, offers), skills, Message, the year grid, and the activity
  list with company logos. The right rail shows similar members.
- **API, additive:** the profile response gains `card_summary` (category, seniority, country);
  each activity day gains `applied`, `interview` and `offer` beside `count`.
- The feed, network and chat pages keep their current content inside the new space until
  their own changes land.

## Capabilities

### New Capabilities
- `buddies-social-space`: the `/buddies` space, its addresses and redirects, the generated
  avatar, and the member profile's contents.

## Impact

- **Web:** new `web/src/routes/buddies/**`; `web/src/routes/my/buddies/**` becomes redirects;
  `BuddyIdentity` draws the generated avatar; account nav, landing CTA, onboarding step,
  messaging dock and its hide rule point at the new paths.
- **Go:** `internal/engage/buddy` (profile summary, per-type activity counts),
  `internal/platform/db/queries/buddies.sql` (`ListBuddyMemberActivity`),
  `internal/api/handler/buddies.go`; `internal/engage/buddy/kind.go` notification path.
- No migration.
