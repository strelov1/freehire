> **Superseded in part by `openspec/changes/messaging-core`.** The chat tables, blocks,
> reports, chat routes and the unread worker described below moved into the shared
> messaging core before release: tables `conversations`/`conversation_participants`/
> `messages`/`user_blocks`/`conversation_reports` (migration `0202_messaging.sql`), routes
> under `/me/conversations` and `/moderation/conversation-reports`, the invitation at
> `POST /me/buddies/invite`, and the worker `cmd/message-notify`. Buddy-only tables are in
> `0203_buddies.sql`, the feed index in `0204_application_events_feed_idx.sql`.

## Why

People searching for the same kind of role in the same place, often at the same companies, have no way to find each other on freehire. Each of them goes through the same interview loops alone, while the person who just reached the onsite at a company is exactly who the next applicant wants to ask. An opt-in, anonymous peer network ("interview buddies") turns the tracking data we already hold into that connection without exposing anyone to their current employer or to recruiters.

## What Changes

- New opt-in **buddy membership** with its own anonymous profile and handle. The card uses the Talent Network's dictionary-only projection, but it is a separate identity: visible only to signed-in members, never to crawlers, recruiters or the public Talent Network catalogue.
- New **similar members** list. It ranks every other member by shared category, shared country and region, and companies both applied to in the last 90 days.
- New **buddy feed** built from the application event ledger. It shows a member's applications and their moves into the interview and offer stages, dated to the day. Rejections, withdrawals and expiries never appear. The feed has a "similar" and an "all" view.
- A per-application **"show in buddy feed"** switch in the tracker (default on), stored apart from the tracker's own tables.
- **Invitations as the first chat message.** One pending or open conversation per pair. Body up to 500 characters. At most 10 new invitations per member per day. Declining is silent.
- **In-app chat** for accepted conversations. The open page polls for new messages. Members exchange outside contacts themselves if they want to, and there are no contact fields.
- **Block and report** in chat. A block closes the conversation for good and hides both members from each other everywhere. Reports reach a new moderation tab.
- New cron worker **`cmd/buddy-notify`**. It sends one "you have unread messages" notification per conversation, once a message has stayed unread for 5 minutes, over the member's existing notification channels.
- A **member page** at `/my/buddies/<handle>`: the anonymous card (rendered by the same `CandidateCardView` as the Talent Network page, never with a photo), a "Message" button, the member's own feed, and a **year activity grid** that counts exactly the feed's events.
- A **messaging dock** on every page for members: the conversation list and up to two chat windows in the bottom-right corner, a full-width sheet on mobile.
- Leaving the network removes the member from lists and the feed and closes their conversations while they are away. After rejoining, former partners see their handle on the old history again and either side may invite again, re-opening the same conversation. Only a block ends contact for good. Deleting the account deletes their conversations and messages.
- Free for every signed-in account. No plan gate.
- `/privacy` gains a section on stored private messages.

## Capabilities

### New Capabilities
- `interview-buddies`: buddy membership and anonymous profile, similarity ranking, the buddy feed and its per-application visibility, invitations, in-app chat, blocks and reports, and unread-message notifications.

### Modified Capabilities
<!-- None: the tracker gains a control, but its own requirements (stages, events) do not change; the switch is owned by interview-buddies. -->

## Impact

- **Backend:** new package `internal/engage/buddy` (layer 7, registered in `internal/platform/arch/layering/blocks.go`). It reads `internal/candidate/talentnetwork` (card projection), `application_events`, and `jobs`/companies.
- **Database:** one new migration adding `buddy_members`, `buddy_hidden_applications`, `buddy_conversations`, `buddy_messages`, `buddy_blocks`, `buddy_reports`. New sqlc queries in `internal/platform/db/queries/buddies.sql`.
- **API:** new authenticated routes under `/api/v1/me/buddies/...` (membership, similar, feed including one member's feed via `?handle=`, a member's profile and activity grid, conversations, messages, block, report, hidden jobs) and a moderation endpoint for buddy reports.
- **Worker:** `cmd/buddy-notify` plus a systemd timer in the private `freehire-ops` repository, every 5 minutes.
- **Web:** new `/my/buddies` page with three tabs (similar, feed, chats), a member page `/my/buddies/<handle>`, a messaging dock mounted by the root layout, a chat view shared by the Chats tab and the dock, a feed switch on tracker cards, a moderation tab, and the `/privacy` text. The Talent Network page `/talent/<handle>` now renders through the shared `CandidateCardView`, with no visible change.
- **No change** to the public Talent Network, its catalogue or its handle.
