## Context

The interview-buddies change (unarchived, unreleased, PR #3623) built a two-member chat inside
`internal/engage/buddy`: `buddy_conversations` with `user_low`/`user_high`, per-side read
cursors and notice stamps as columns; `buddy_messages`; `buddy_blocks`; `buddy_reports`; and
`cmd/buddy-notify`. The owner wants the same inbox to carry company↔candidate threads and
operator announcements later. None of the tables exist in production, and `main` has since
taken migrations 0199–0201, so the buddy migrations must be renumbered anyway.

## Goals / Non-Goals

**Goals:**
- One messaging core that any feature uses by declaring a kind.
- The buddy chat runs on it with no behaviour change visible to members.
- Adding company↔candidate threads or announcements later needs a new kind, not a new schema.

**Non-Goals:**
- Company↔candidate threads, mentor conversations, announcements, group conversations,
  attachments, realtime push. Mentor chat is the next kind on this core, with its own short
  design (who may write first, whether a request step applies).
- Migrating data: the old tables never shipped.

## Decisions

### D1. Package `internal/engage/messaging`, imported by kinds, importing none of them
The core lives in the `engage` block. `buddy` imports `messaging`; `messaging` never imports
`buddy`. A later company kind lives in `ingest` (employer accounts), which shares layer 7 with
`engage` and therefore cannot import it — that thread will be orchestrated in `internal/api`,
which may import both. Alternative considered: putting the core in a lower block (`identity`)
so `ingest` could import it. Rejected: messaging is not identity, and the api layer already
composes cross-block flows.

### D2. Participants are rows, not columns
Tables:
- `conversations (id, kind, pair_key NULL, status, starter_id, created_at, accepted_at,
  closed_at)` with `UNIQUE (kind, pair_key)` and a `kind` CHECK listing known kinds.
- `conversation_participants (conversation_id, user_id, persona, last_read_message_id,
  notified_at, PRIMARY KEY (conversation_id, user_id))`, index on `user_id`. `persona` is
  the identity the account speaks as in this conversation (see D3a); CHECK lists the known
  personas, today only `buddy`.
- `messages (id, conversation_id, sender_id, body, created_at)`.
- `user_blocks (blocker_id, blocked_id, created_at)`.
- `conversation_reports (id, conversation_id, reporter_id, reason, note, created_at,
  resolved_at, resolved_by, UNIQUE (conversation_id, reporter_id))`.

Alternative considered: keep `user_low`/`user_high` and add `kind`. Rejected by the owner:
announcements and a recruiter's team are not pairs.

The buddy kind sets `pair_key = '<low>:<high>'` so one buddy conversation exists per pair.
Every query that used `user_low`/`user_high` is rewritten over participants; the
"other participant" is the participant row whose `user_id` differs from the caller.

### D3. Kinds plug in through an interface registered at the composition root
```go
type Kind interface {
    Name() string
    // Counterparts describes each other participant as viewerID may see them, keyed by
    // account: what the UI draws and whether the other side may be shown at all.
    Counterparts(ctx context.Context, viewerID int64, otherIDs []int64) (map[int64]Counterpart, error)
    // Names is what a moderator reads each account as: the name the others saw.
    Names(ctx context.Context, userIDs []int64) (map[int64]string, error)
    // Notice is the wording and link of an unread notice.
    Notice(conversationID int64, pending bool) Notice
}
```
`Counterpart` is `{Reachable bool; Detail any}` — `Detail` is the kind's own value (for buddy:
handle and card summary), and the core blanks it whenever `Reachable` is false. Starting a conversation is NOT on the interface: the kind's own
service decides who may start (buddy: active members, the 10-per-24h limit) and then calls
`messaging.Service.Start(ctx, kind, starterID, recipientID, pairKey, firstBody)`. The core
enforces only what is universal: blocks, the pair key, body length. When the pair key already names a
conversation that is `closed` (not `declined`, not blocked), `Start` reopens it as pending with
the new starter — the buddy "re-invite after leaving" rule — keeping earlier messages as read
for both and the new message unread for the recipient.

### D3a. Participants speak as a persona; personas are never joined
One account has several identities: the member (real account), a buddy (anonymous handle),
later a mentor and an employer. Each participant row records the persona it speaks as, and the
counterpart a viewer sees is resolved from the OTHER participant's persona — not from the
conversation's kind. So the plug-in seam is split: a `Persona` (`Name()`, batch
`Counterparts(ctx, viewerID, rows)`) draws a participant; a `Kind` keeps the notice wording,
link and pair-key rule. The buddy package registers both the `buddy` kind and the `buddy`
persona; a buddy conversation's two participants are both `buddy`.

Invariant: nothing returned for a conversation may identify the account behind a persona —
no user ids, emails or the account's other personas. Someone who chats with a buddy handle
and, later, with the same person as a mentor must not be able to tell it is one account.
Alternative considered: deriving the counterpart from the kind alone. Rejected: a mentor
conversation has a mentor on one side and a mentee on the other, so the two sides differ.

### D4. Blocks are account-wide
A block from any conversation inserts into `user_blocks` and closes every conversation between
the pair. Buddy's feed, similar list and profile read the same table. Reason: someone blocked
as a buddy must not reappear as a recruiter.

### D5. One notice worker for all kinds
`cmd/message-notify` replaces `cmd/buddy-notify` with the same claim/deliver/release logic,
moved to per-participant `notified_at`. The release uses `context.WithoutCancel` plus its own
timeout. Notice wording and link come from `Kind.Notice`; the in-app notification-center kinds
stay `buddy_message` / `buddy_invitation` for the buddy kind. `MESSAGE_NOTIFY_MAX_PER_RUN`
replaces `BUDDY_NOTIFY_MAX_PER_RUN`.

### D6. API
- `GET /me/conversations`, `GET|POST /me/conversations/:id/messages`,
  `POST /me/conversations/:id/{read,accept,decline,block,report}`.
- `POST /me/buddies/invite` starts a buddy conversation (body: `handle`, `body`).
- `GET /moderation/conversation-reports`, `POST /moderation/conversation-reports/:id/resolve`.
- Status codes are unchanged from the buddy routes (404 outsider, 409 wrong state,
  403 blocked/not allowed, 429 invitation limit, 400 bad body), except that the
  conversation list no longer requires buddy membership: it lists every kind, and a
  non-member simply has no buddy conversations in it.
- A conversation entry is `{id, kind, status_for_me, counterpart: {reachable, detail},
  unread, last_message_at, created_at}`; for kind `buddy`, `detail` is
  `{handle, card_summary: {category, seniority, country}}` and is `null` when unreachable.
  A report is `{id, conversation_id, kind, reason, note, created_at, resolved_at, reporter,
  reported, messages: [{sender, body, created_at}]}`, names as the kind draws them.

### D7. Web
`BuddyMessagingDock`, `BuddyConversationPane`, `BuddyConversationRow`, `BuddyUnreadBadge`
become `MessagingDock`, `ConversationPane`, `ConversationRow`, `UnreadBadge`. Each draws the
counterpart through a per-persona snippet; only `buddy` exists. `BuddyReportsQueue` becomes
`ConversationReportsQueue`. The `/my/buddies` Chats tab keeps using the shared pane.
The dock can no longer learn membership from a 403 on the list (D6), and must not call
`GET /me/buddies`; it now appears for an account that has at least one conversation, or
while a buddy first message is being written (its dialog lives in the dock). A member with
no conversation yet therefore sees no dock until the first one exists, and a member who left
keeps their closed conversations in it.

### D8. Migrations
The unreleased buddy migration is split and renumbered after `main`: `0202_messaging.sql`
(core tables), `0203_buddies.sql` (members, hidden applications), and the feed index moves to
`0204_application_events_feed_idx.sql`. That index already exists on production, built by hand
with the same name and `IF NOT EXISTS`, so the migration is a no-op there.

## Risks / Trade-offs

- [Queries over participants are a join where columns were direct] → index
  `conversation_participants (user_id)`, and keep the existing EXPLAIN guard tests on the list
  and unread-due queries.
- [Local dev databases hold the old buddy tables] → the rewritten migration drops nothing; a
  developer resets the local DB (`make down && make up`). Production never had them.
- [Deleting an account deletes whole conversations] → right for two-person kinds; a future
  multi-party kind must revisit it (delete the participant, keep the thread). Done by an
  AFTER DELETE trigger on `conversation_participants`, since a participant row cascading
  away with its account would otherwise leave the other side holding the conversation.
- [The buddy invitation and answer no longer check membership inside the write, as the
  buddy tables did] → a leave committing between the read and the write can leave a
  pending invitation to, or an acceptance by, a member who left. The core cannot know a
  kind's membership; blocks, the guard that matters for safety, stay inside the writes.
- [Generic `Detail any` in Counterpart loses type safety on the wire] → each kind has a typed Go
  struct and a typed TS shape discriminated by `kind`.

## Migration Plan

Ships in PR #3623 with the buddy feature. `release.sh` runs the migrations; the ops PR renames
`freehire-buddy-notify` to `freehire-message-notify` before the timer is enabled. Rollback:
revert the release; the tables are new and unread by anything else.

## Open Questions

None.
