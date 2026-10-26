> **Superseded in part by `openspec/changes/messaging-core`.** The chat tables, blocks,
> reports, chat routes and the unread worker described below moved into the shared
> messaging core before release: tables `conversations`/`conversation_participants`/
> `messages`/`user_blocks`/`conversation_reports` (migration `0202_messaging.sql`), routes
> under `/me/conversations` and `/moderation/conversation-reports`, the invitation at
> `POST /me/buddies/invite`, and the worker `cmd/message-notify`. Buddy-only tables are in
> `0203_buddies.sql`, the feed index in `0204_application_events_feed_idx.sql`.

## Context

freehire already holds, for every signed-in user, a structured CV (`users.resume_structured`), CV-derived geography (`users.resume_countries/regions`), search preferences (`user_profiles`), and an append-only ledger of what happened to each application (`application_events`, migration 0062: `applied`, `stage_set` with a stage signal, dated by `occurred_at`, company denormalized into `company_slug`). It also has a dictionary-only anonymous card for the public Talent Network (`talentnetwork.ProjectCard`), a pseudonymous community persona, and three delivery paths (email, Telegram, the in-app notification center).

Nothing lets two job seekers who chase the same roles find and talk to each other. This change adds that as a members-only, anonymous peer network with an in-app chat.

Constraints:
- Layering: the package sits in `engage` (layer 7), which may import `candidate`, `application`, `job`, `identity`, `dict`, `platform`.
- The anonymity rule of `internal/candidate/talentnetwork` (only dictionary values, numbers, dates, and role titles reach another person) must hold for buddy cards too.
- Deploy: blue/green behind nginx, one API process per colour; cron workers are run-once `Type=oneshot` units configured in `freehire-ops`.

## Goals / Non-Goals

**Goals:**
- Opt-in membership with its own anonymous handle, separate from the Talent Network.
- A ranked "similar members" list and a feed of applications and interview/offer moves.
- Invitation-as-first-message, an in-app chat, blocks, reports, unread notifications.
- No member data reaches anyone who has not joined, nor crawlers.

**Non-Goals:**
- Live push to the open page (websocket/SSE). Polling is enough at this scale.
- Attachments, group chats, message editing, typing indicators, read receipts shown to the other side.
- Contact fields. People swap contacts in the chat if they want.
- A paid tier or plan-limit integration. The 10-per-day invitation cap is a spam bound, not a product limit.
- Exposing buddy data over MCP, OJCP or the public API.

## Decisions

### D1. One package, `internal/engage/buddy`
Membership, ranking, feed, conversations and the notifier live in one package with a `Service` over a `Repository` interface, the shape `internal/engage/referral` and `community` use. The HTTP layer is `internal/api/handler/buddies.go` (`newBuddyHandlers(buddy.New(...))`, registered in `handler.go` beside referrals).
*Alternative:* split out a generic `engage/chat` package for future reuse by referrals or mentorship. Rejected: no second consumer exists today. The seam is the `conversations`/`messages` tables, which can be generalized later without a wire change.

### D2. A separate handle, minted like the community persona
`buddy_members.handle` is generated with the same shape as `community.GenerateHandle` (adjective-noun-NN) and is unique. It is not derived from the CV category, because the Talent Network handle already is, and two handles built from the same input are easier to link. Leaving sets `left_at` and keeps the row, so rejoining restores the handle.
*Alternative:* reuse the community persona. Rejected: it is shown beside public discussion posts and company reviews, so linking it to a list of applications would let anyone join a person's posts to their job search.

### D3. Card projected on read; ranking keys stored
The card is projected from `resume_structured` on every read with `talentnetwork.ProjectCard`, exactly as `talentnetwork.ByHandle` does, so a CV change shows up at once and nothing has to keep a copy fresh.
Ranking needs the category and geography in SQL, so `buddy_members` stores `category text`, `countries text[]`, `regions text[]`:
- `category` = `classify.Parse(talentnetwork.PrimaryTitle(cv)).Category`, falling back to the first `user_profiles.specializations` entry.
- `countries`/`regions` = `users.resume_countries/regions`, falling back to the profile's base country.
These are recomputed on join and whenever the member loads their buddy status (`GET /me/buddies`, the call the web client makes on entering the section; `Service.Touch`), which is cheap and keeps active members current. No other route touches: the chat polls every few seconds and a write per poll would be wrong. A member who never visits keeps older keys; they are also the member least affected.
*Alternative:* compute everything in Go over all members. Rejected: it scans the whole membership on every request.

### D4. Similarity in one SQL query
`ListSimilarBuddies(viewer, limit, offset)` scores each active, unblocked member:
`3·(same category) + 2·(countries overlap) + 1·(regions overlap) + 2·(shared companies, capped at 5)`,
where shared companies come from `application_events` (`kind='applied'`, `occurred_at > now()-90 days`, not hidden) of both sides, grouped by `company_slug`. Ties break on `joined_at DESC`. The shared company slugs are returned as an array (max 5) for the card. The weights are named constants in Go, passed as parameters, and covered by a unit test of the ordering rule. They are a first guess and say so in a comment.

### D5. Feed straight from `application_events`
`ListBuddyFeed(viewer, mode, before_cursor, limit)` reads events of active members, excluding the viewer, blocked pairs and hidden applications:
- `kind='applied'` → type `applied`
- `kind='stage_set' AND signal IN ('interview','offer')` → type `interview` / `offer`
- everything else, including `rejected`, `withdrawn`, `expired`, `accepted`, mail-derived `employer_reply` and `interview_scheduled`, is excluded. `accepted` is excluded on purpose: "took the offer" says the person changed jobs, which is the most identifying fact in the ledger.
- `retracted_at IS NULL` only.
The response carries `occurred_at::date` only. Job title comes from a `LEFT JOIN jobs` (it can be null after prune); the company name comes from `companies` by slug.
Mode `all` orders by `occurred_at DESC, id DESC` with a keyset cursor. Mode `similar` keeps the same keyset order but restricts to the top-N most similar members (N=200), so it stays a cursor-paged feed instead of a re-sorted one.
A new partial index `application_events (occurred_at DESC, id DESC) WHERE kind IN ('applied','stage_set') AND retracted_at IS NULL` serves the keyset read. It is built `CONCURRENTLY` in its own no-transaction migration file, because the table is large and `ALTER` on hot tables hits the 5s lock timeout.

### D6. Hidden applications are buddy-owned
`buddy_hidden_applications (user_id, application_id)` with a primary key on both and no foreign key on `application_id`. The tracker shows the switch, but the tracker's own tables and the `user-job-tracking` requirements do not change. Hidden is keyed on the application, the id every ledger event carries in `application_events.application_id`. `job_id` would be wrong: `cmd/prune` clears it on events (SET NULL) while the event stays in the feed, so a job-keyed mark would be lost or unmatched. A mark left behind by a deleted application is harmless, since its events cascade away. The missing FK also avoids a lock on `applications` during the migration.

### D7. Conversations and messages
- `buddy_conversations (id, user_low, user_high, initiator_id, status, created_at, accepted_at, closed_at, last_read_low, last_read_high, notified_low_at, notified_high_at)`.
  - `user_low < user_high` with a unique index on the pair gives "one conversation per pair" without ordering bugs.
  - `status` is `pending | open | declined | closed`, enforced by a CHECK.
  - The read cursors (`last_read_*` = last read message id) and notified stamps live on the row, one per side. There are only two members, so a side table would add a join and nothing else.
- `buddy_messages (id bigint identity, conversation_id, sender_id, body, created_at)`, with an index on `(conversation_id, id)`. Body length is checked in Go and by a CHECK (1..4000).
- An invitation inserts the conversation and its first message in one transaction. The 24-hour cap counts `buddy_conversations WHERE initiator_id=$1 AND created_at > now()-24h`, the same SQL-count pattern as `referral.DefaultDailyRequestCap`.
- A declined invitation stays `declined`, so the pair cannot be re-invited. The sender's view maps `declined` to "unanswered", the same as `pending`.
- A conversation closed WITHOUT a block between the pair (leaving closes it) is re-opened by a new invitation from either member: the invitation UPDATEs the existing row to `pending` (new `initiator_id`, `created_at = now()` so the cap counts it, `accepted_at`, `closed_at` and the notified stamps cleared) and inserts the new first message in the same statement, under the same guards as the insert (both active, no block). History stays, and both read cursors move to the newest old message so only the new one is unread. A block keeps the row closed for good and the refusal is the ordinary "recipient unavailable", which does not reveal it; `declined` stays terminal. Without this, two members who once talked could never talk again after one of them left and came back.
- All foreign keys to `users` are `ON DELETE CASCADE`, which covers account deletion.

### D8. Polling, not a live channel
The chat view polls `GET /me/buddies/conversations/:id/messages?after=<id>` every 5 seconds while the tab is visible, and refreshes on `visibilitychange`, the same shape as `MentorBookingView.svelte`. The conversation list polls every 30 seconds for unread counts.
*Alternative:* websocket. Rejected for now: blue/green colours and the nginx path would need sticky routing and reconnection handling, all for typing-speed latency nobody asked for. Moving later only changes the client transport.

### D9. Notifications: worker, one per conversation
`cmd/buddy-notify` (run-once, every 5 minutes) calls `buddy.NotifyUnread`:
1. It selects conversations where some side has a message from the other side with `id > last_read_side`, the oldest unread is older than 5 minutes, and `notified_side_at` is null or older than that oldest unread message. Status must be `pending` (for the recipient) or `open`.
2. It sets `notified_side_at = now()` first (claim), then sends. On a delivery failure it clears the claim, so the next run retries. This is the claim-then-send order `mentorship-remind` uses.
3. Delivery copies the referral pinger: email via `emailnotify` (honouring `emailprefs` for the activity group) and Telegram when linked. It also records an in-app notification via `RecordNotification` with a dedup key per conversation and unread message. The text says "You have a new message from a buddy" (or "a new buddy invitation") and links to `/my/buddies?c=<id>`. It carries neither the body nor any account detail.
Reading a conversation advances `last_read_side`, so the next unread message makes it eligible again.

### D10. Block and report
- `buddy_blocks (blocker_id, blocked_id, created_at)`. Every list, feed, invite and send query excludes a pair blocked in either direction. Blocking also closes the conversation.
- `buddy_reports (id, conversation_id, reporter_id, reason, note, created_at, resolved_at)`. `reason` is a closed list (`spam`, `harassment`, `scam`, `other`) enforced by a CHECK, plus a unique `(conversation_id, reporter_id)`. This is the lighter `company_feedback_reports` pattern.
- Moderator routes: `GET /moderation/buddy-reports` (report + messages) and `POST /moderation/buddy-reports/:id/resolve`. The moderator role gate is the same one the existing feedback queue uses. A new `buddies` tab in `ModerationView.svelte`.
- Moderators reading messages of a reported conversation is stated in `/privacy`.

### D11. Access control
- Every `/me/buddies/*` route is `RequireAuth` (cookie only). A scoped API key cannot reach it. Buddy data is not offered to agents or the CLI in this change.
- Every read except join and membership status first checks for an active membership, otherwise 403 (`buddy.ErrNotMember`).
- A conversation the caller is not part of returns 404, not 403, so its existence does not leak.
- The `/my/` layout is already noindex and absent from sitemaps.

### D12. Wire shapes
The response conventions are `{"data": ...}` and `{"data": [...], "meta": {...}}`. Cards use `talentnetwork.CandidateCard` as-is plus `handle`. Feed entries are `{id, handle, card_summary:{category, seniority, country}, company:{slug,name}, job_title|null, type, day}`. Conversations are `{id, other:{handle, card_summary}, former_member, status_for_me, unread, last_message_at}`, where `former_member` is true when the other party left or the pair is blocked in either direction (the conversation stays listed as history, but `other.handle` is empty and the card is blank, so neither a left nor a blocked member's identity keeps being served), where `status_for_me` maps `pending` to `incoming` or `outgoing` and maps `declined` to `outgoing` for the initiator.

### D13. A page per member, rendered by the Talent Network's card view
Each member has a page at `/my/buddies/<handle>` (under `/my/`, so noindex and absent from sitemaps) reading `GET /me/buddies/profiles/:handle`. The card is `talentnetwork.CandidateCard`, so the page and the public `/talent/<handle>` page render it through one component, `CandidateCardView`: heading, experience, education, certifications, skills. What differs is a prop: the catalogue passes place, zone, "open to" and a photo and keeps skills in a left rail (`skillsSide="left"`); the buddy page passes the handle as a subtitle, a "Message" action beside the heading, and lists skills inline at full width (`skillsSide="inline"`). The buddy page never shows a photo, not even the blurred one the Talent Network serves: the same picture on both pages would link the buddy handle to the catalogue card, which is exactly what D2's separate handle prevents. A buddy is always the plain silhouette (`BuddyIdentity`).
*Alternative:* a second, buddy-only card component. Rejected: two renderings of one payload drift, and the anonymity review would have to be done twice.

### D14. One member's feed and their activity grid
The member page lists that member's events through the feed itself, `GET /me/buddies/feed?handle=<handle>` (`Service.MemberFeed`): the same visibility rules, cursor and entry shape, with `mode` not read. A handle the viewer may not see (their own, a left member, either side of a block) answers not found, as the profile does.
Above it sits a year grid, the tracker's activity-grid component fed by `GET /me/buddies/profiles/:handle/activity` (`ListBuddyMemberActivity`, per UTC day over the last year). It must count exactly what the member feed lists, never the tracker's own activity query, which counts follow-ups and every stage move. sqlc cannot share a WHERE clause, so the query restates `ListBuddyFeed`'s predicate; `TestMemberActivityMatchesMemberFeed` seeds every exclusion (rejection, follow-up, retracted, hidden, private posting) and holds the grid's total to the member feed's length, so changing one predicate without the other fails.

### D15. A messaging dock across the site
A member can message from anywhere, not only from the Chats tab: a dock in the bottom-right corner unfolds into the conversation list, with up to two chat windows beside it (one full-width modal sheet at a time below `sm`). It is mounted once by the root layout, lazily and only for a member, so it and its windows survive client-side navigation. Membership is probed through the conversation list (`403` = not a member), never through `GET /me/buddies`, which touches ranking keys (D3). It renders nothing on `/my/buddies`, whose Chats tab is the full-size version of the same list and pane (`BuddyConversationPane`, shared). The dock, the Chats tab and every "Message" button read one module-level store, so unread counts cannot disagree on one screen. `sessionStorage` keeps only which windows are open (conversation ids and minimized state), never a message body; unsent drafts live in memory only and are dropped on sign-out. The open-source support toast steps aside for the dock, and is suppressed on `/my/buddies`, where it would cover the composer's Send button.

## Risks / Trade-offs

- **Re-identification from a rare combination** (niche role, small country, unusual companies) → the feed shows the day only and never `accepted`. Any application can be hidden. The join screen states plainly that other members will see the companies you apply to.
- **A spam or harassment vector** → the 10-per-day invitation cap, one conversation per pair, a permanent decline, block, report and the moderation queue.
- **Empty network at launch**: ranking never filters people out (D4), and the feed "all" mode always has content once a few members join.
- **The feed query cost on a large ledger** → the dedicated partial index (D5) and the keyset cursor. The similar mode restricts to the top 200 member ids before the event scan.
- **Stale ranking keys for inactive members** → accepted. They refresh on their next visit.
- **Polling load**: 5 s per open chat tab is an indexed `id > $after` read. It stops when the tab is hidden.
- **Storing private messages** raises privacy obligations → a `/privacy` section, cascade delete on account deletion, and moderator access only through a report.

## Migration Plan

1. Migrations `0202_messaging.sql` and `0203_buddies.sql`: the six tables, CHECKs and indexes on the new tables (empty, so no lock risk).
2. Migration `0204_application_events_feed_idx.sql` with the `migrate: no-transaction` marker: `CREATE INDEX CONCURRENTLY IF NOT EXISTS ...`. Build it on prod by hand before the merge, as the lock-timeout note requires for large tables. The migration is then a no-op on deploy.
3. Deploy the server and web. The feature is reachable from the account navigation.
4. Add the `freehire-message-notify.{service,timer}` units in `freehire-ops` (every 5 minutes, reading `.env` and `.env.notify`), copy them to the host and run `daemon-reload`.
5. Rollback: remove the nav entry or revert. The tables are additive and can stay. Stopping the timer stops notifications.
