## Why

`/buddies/chats` is the old tab: a bordered list beside a bordered pane of plain boxes with
full timestamps. The approved social redesign (see `buddies-social-space`) makes it read as
a messenger and gives each conversation its context — what the two people have in common.

## What Changes

- Messages as bubbles (the viewer's own in the brand fill, on the right), a short time
  under each, and a day separator between days. A link to one of our job pages in a message
  is previewed under it as a job card (at most two per message), read from the public job
  endpoint. The composer and the Report / Block controls are quieter. This is the shared
  `ConversationPane`, so the messaging dock gets the same.
- Above the first message of a buddy chat: "You both applied to …" with the companies'
  logos, from the similar list's shared companies (`ConversationPane` gains an `intro`
  snippet).
- On 2xl screens, a rail beside the chat: the other member's card with View profile, the
  companies both applied to, and their latest three feed events.
- The conversation list is a compact column of rounded rows.
- No API change.

## Capabilities

### Modified Capabilities
- `buddies-social-space`: the chats page.

## Impact

- **Web:** `ConversationPane`, `ConversationRow`, `BuddyChatView`, new `BuddyChatRail`,
  `ChatJobPreview`; `messaging.ts` gains `messageJobSlugs` and `messageDays`.
