## Context

`/my/buddies` renders `BuddiesView` (tabs Similar / Feed / Chats) inside the `/my` shell, whose
own sidebar takes `lg:w-56` of a `max-w-6xl` container. The approved mockup is three columns:
a buddies navigation, the content, and a right rail. They do not fit beside the cabinet
sidebar.

## Decisions

**A top-level `/buddies` space, not a section of `/my`.** It reuses the `/my` gate (a
`+layout.server.ts` that redirects a signed-out visitor to sign-in) and sets `noindex` in its
own layout. The buddies navigation is part of the space's layout, so every page in it shares
it. The account nav keeps an "Interview buddies" entry, now pointing at `/buddies`.

**Redirects, not copies.** `/my/buddies/+page.server.ts` and `/my/buddies/[handle]/+page.server.ts`
answer 308 to the new address with the query string kept: `?c=<id>` goes to
`/buddies/chats?c=<id>`, anything else to `/buddies`. Emails already sent carry the old
address, so the redirect stays.

**Handles cannot collide with the fixed pages.** Handles are `adj-noun-NN` (`handle.go`), so
`network` and `chats` are never a handle; the static routes win in SvelteKit anyway.

**Membership is read once, by the layout.** The space layout calls `GET /me/buddies` and shows
the join screen in place of any page for a non-member, so each page does not repeat it.

**Generated avatar.** `buddyAvatar(handle)` returns initials (first letter of the first two
words) and a hue (a stable string hash of the handle, mod 360). It is pure, so the feed, the
lists, chat rows and the dock draw the same member the same way. The previous rule — always the
silhouette, never a photo — existed so a buddy could not be linked to their Talent Network
card. A picture derived from the anonymous handle alone keeps that property; the photo ban
stays.

**Year in numbers from the activity read.** `ListBuddyMemberActivity` already applies the
feed's visibility predicate; it gains three `count(*) FILTER` columns by event type. The
client sums the days. `count` stays the sum, so `TestMemberActivityMatchesMemberFeed` keeps
guarding the predicate.

**Country on the profile.** The card projection has no country. `Profile` builds the same
`CardSummary` the similar list uses, from the member's inputs, and returns it beside the card.

## Risks

- Old `/my/buddies` bookmarks: covered by the redirects.
- The dock's hide rule moves to `/buddies/chats`; the member page keeps the dock.
