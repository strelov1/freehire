# Interview buddies

## Scope
The opt-in peer network (openspec/changes/interview-buddies): membership under a generated
handle, similar members, a feed of other members' search progress, and the invitation that
starts a one-to-one chat. The chat itself — conversations, messages, blocks, reports, the
unread notices — is the messaging core's ([internal/engage/messaging](../messaging/AGENTS.md)),
kind `buddy`, both participants speaking as the persona `buddy`. This package registers
both: `Persona` (a member drawn by handle and card, named by handle to a moderator, never
by account) and `Kind` (the notice wording and link). Layer 7 (`engage`); tables are
`buddy_members` and `buddy_hidden_applications` (migration 0203), queries in
`internal/platform/db/queries/buddies.sql`. HTTP lives in `internal/api/handler`, not here.

## Always true
- **Anonymity.** A member is a handle. Nothing they typed into their CV reaches another
  member, except role titles through `talentnetwork.ProjectCard`. The buddy handle is
  minted separately from the Talent Network and community handles so that no two of them
  can be joined. Notifications carry no message text and no account detail.
- **Feed vocabulary.** The feed shows `applied`, and `stage_set` with signal `interview` or
  `offer`, at day precision, and nothing else. `accepted`, `rejected`, `withdrawn` and
  `expired` never appear: "took the offer" is the most identifying fact in the ledger. An
  event is dropped when it is retracted, hidden by its owner (`buddy_hidden_applications`), or the
  owner has left or is blocked in either direction.
- **The member page's year grid counts the feed, nothing else.** `ListBuddyMemberActivity`
  restates `ListBuddyFeed`'s visibility predicate (sqlc cannot share a WHERE clause); change
  one and change the other. `TestMemberActivityMatchesMemberFeed` seeds every exclusion and
  holds the grid's total to the member feed's length. Never reuse the tracker's activity
  query for another person: it counts follow-ups and every stage move.
- **One conversation per pair**: the messaging pair key `"<low>:<high>"`. Leaving closes every
  pending or open buddy conversation the member is in, in the same statement as the leave
  (`LeaveBuddyMember` touches the core's `conversations` for exactly that); the row and
  handle survive, so rejoining restores the handle and a new invitation reopens the closed
  conversation.
- **Membership is checked by a read before the start, not inside it.** A block is checked in
  the core's insert; a leave committing between `Invite`'s read and the start can leave a
  pending invitation to someone who left, which stays pending until they rejoin. The core
  cannot know a kind's membership, so this race is accepted, not guarded.
- **A counterpart is drawn only while both sides are active members and unblocked**
  (`ListBuddyCounterparts`); otherwise the conversation lists as history with nothing about
  the other side.
- **Account deletion** is the foreign keys: every reference to `users` cascades, and a
  new one must be indexed on the referencing side (`TestEveryUserForeignKeyIsIndexed`).
- **Only entering the section touches.** `GET /me/buddies` runs `Touch`, which rewrites the
  member's ranking keys; no other route may call it.
- **A privately pasted posting (`jobs.is_private`) never reaches another member**: the feed and
  the shared-company count both skip events whose job is private.
