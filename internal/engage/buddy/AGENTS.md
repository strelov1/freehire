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

Mock interviews ([spec](../../../openspec/specs/buddy-mock-interviews/spec.md)) live in `mock*.go`: a member
publishes when they can host and another books 60 minutes of it. Tables `buddy_mock_*`
(migration 0215), queries in `internal/platform/db/queries/buddy_mocks.sql`; the slot
arithmetic is the shared engine in [internal/engage/slots](../slots/doc.go), the reminders a
second pass of `cmd/mentorship-remind`, HTTP `internal/api/handler/buddy_mocks.go`.

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
  pending or open buddy conversation the member is in, in the same transaction as the leave
  (`CloseBuddyConversationsForLeave` touches the core's `conversations` for exactly that); the row and
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
- **One person, one mock at a time, whatever the role.** `buddy_mock_participants` holds a
  row per person per mock under one `EXCLUDE` (`user_id =`, `[)` range `&&`, `WHERE active`);
  a mentor-style constraint on the host alone could not refuse "host at 19:00 and guest at
  19:00". A lost race and a stale page are the same `ErrSlotUnavailable`. Cancelling sets
  `active = false` in the statement that cancels.
- **A genuine booking race surfaces as a deadlock, not a conflict.** Two bookings that share
  a person each insert one participant row and then wait on the other's, so Postgres aborts
  one with 40P01 rather than answering 23P01. `CreateMock` re-runs the statement (it wrote
  nothing); the retry meets the `EXCLUDE` plainly. A test that merely fires two goroutines
  never reaches that interleaving — `TestConcurrentBookingsOfOneSlotConfirmExactlyOne` holds
  both at a barrier before the insert, and that is how the 40P01 was found.
- **The meeting link reaches the two participants of a confirmed mock and nobody else** —
  never a slot listing, a member page or a notice to anyone else. It is read live from the
  host's settings, so a corrected link reaches guests who already booked.
- **A leave or a block cancels future mocks in its own transaction.** `LeaveBuddyMember`
  cancels in a CTE; a block cancels through an AFTER INSERT trigger this feature puts on
  `user_blocks`, because the block is the messaging core's statement and the core never
  imports a feature. Both record `cancelled_by`; the notice is sent afterwards by
  `MockService.AnnounceCancellations(actor)`, which the api layer calls after the leave and
  after `POST /me/conversations/:id/block`. A new path that writes a block or a leave must
  call it too.
- **Booking, leave and block serialise on `buddy_members` rows.** Without a shared lock a
  booking and a leave or block each read the other's pre-state under READ COMMITTED: the
  booking commits confirmed, the cancel never saw it, and the blocked member keeps the
  host's link. So the three writers lock the lifecycle rows first: a booking (`CreateMock`)
  takes both participants' rows `FOR SHARE` in `user_id` order (`LockBuddyMembersForMock`);
  a leave (`Leave`) takes the member's own row `FOR UPDATE` (`LockBuddyMemberForLeave`);
  the block trigger takes both rows `FOR UPDATE` in `user_id` order. Each lock is a
  statement of its OWN, before the check or cancel: a statement keeps the snapshot it
  started with even after waiting on a row lock, so only a later statement sees what
  committed during the wait. One order everywhere means no lock cycle between them; the
  leave closes its conversations BEFORE locking the member row, because a block locks the
  conversations before its trigger reaches `buddy_members`. A new writer that creates or
  ends a mock relationship joins this protocol. `TestABookingRacing*` and
  `TestABookingWaitsForAnUncommittedBlock` hold it, and fail without the locks.
- **The link is withheld at read too.** `GetBuddyMock`, `ListBuddyMocksForUser`, the
  reminder due-list and the reminder claim return the link, or claim, only while the mock
  is confirmed, both are active members and no block stands either way — the defence
  should a writer ever miss a cancel. The rule is restated in those four queries; change
  them together.
- **Reviews are blind.** A review is shown to its subject and on their page only once the
  subject has reviewed back or seven days have passed since the end. The rule is computed in
  `ListBuddyMocksForUser`, `GetBuddyMemberMockSummary` and `ListBuddyMemberRevealedReviews`;
  change all three together. A member page never names who wrote a review.
- **Mock notices name the other member by handle only**, render the time in the
  recipient's zone (UTC, stated, when it does not resolve), and say only "cancelled" for a
  leave or a block.
