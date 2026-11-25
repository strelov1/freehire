## Why

`/buddies/network` still shows the old Similar tab: a column of rows. The approved social
redesign (see `buddies-social-space`) makes it a network page: people like you as a grid of
cards, and the companies the viewer shares with them — who else applied where you did.

## What Changes

- People like you as a grid of cards (large generated avatar, heading, flag, handle, the
  shared companies' logos and count, Message), with an All / Same companies filter and
  offset paging ("Show more", 24 a page).
- A new section, "At the companies you applied to": every company the viewer shares with
  the members loaded, busiest first, with their faces and a count, linking to the company.
  Drawn client-side from the similar list's own shared companies (`buddiesByCompany`).
- `BuddyIdentity` gains `avatar={false}` for a host that draws the avatar itself.
- No API change.

## Capabilities

### Modified Capabilities
- `buddies-social-space`: the network page.

## Impact

- **Web:** `BuddySimilarList`, `buddyNetwork.ts`, `BuddyIdentity`, `routes/buddies/network`.
