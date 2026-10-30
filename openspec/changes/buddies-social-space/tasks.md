## 1. API

- [x] 1.1 `ListBuddyMemberActivity` counts applied / interview / offer per day; `ActivityDay` and the activity response carry them; the grid-matches-feed test still holds
- [x] 1.2 `Profile` returns the member's `CardSummary`; the profile response carries `card_summary`
- [x] 1.3 Unread-message notification links to `/buddies/chats?c=<id>`

## 2. Space and addresses

- [x] 2.1 `/buddies` layout: sign-in gate, noindex, membership read once, join screen for non-members, left navigation (Feed, Network, Chats), right-rail slot
- [x] 2.2 Pages `/buddies` (feed), `/buddies/network` (similar), `/buddies/chats` (chats, `?c=`) built from the existing tab contents
- [x] 2.3 `/my/buddies` and `/my/buddies/<handle>` redirect (308, query kept)
- [x] 2.4 Every link and rule pointing at `/my/buddies` moves: account nav + i18n + icons, landing CTA, onboarding step, messaging dock link and hide rule, `BuddyIdentity`

## 3. Avatar and profile

- [x] 3.1 `buddyAvatar(handle)` (initials + hue) and its use in `BuddyIdentity`
- [x] 3.2 Member profile at `/buddies/<handle>`: header, year in numbers, skills, Message, year grid, activity list with logos, similar members in the right rail
