## Why

The buddy feed shows other members' search progress but gives no way to answer it short of a
private message. Reactions are the lightest answer a social feed offers, and the approved
redesign (`buddies-social-space`) has them — chosen for a job search rather than a generic like.

## What Changes

- **Reactions** to a feed event: congrats, luck, rooting, same (same boat), insight. One per
  member per event; choosing another replaces it, choosing it again removes it.
- A member can react only to an event their own feed shows. The check reads the feed query
  positioned at the event, so every exclusion it keeps (own events, blocks, hidden
  applications, private postings, members who left) holds with no second predicate.
- Feed entries carry `reactions: {counts, mine}`. A reaction from a member who left, or from
  one on either side of a block with the viewer, does not count.
- **API:** `PUT /me/buddies/feed/:id/reaction` `{kind}` and `DELETE` the same; both answer
  the event's reactions. **Migration 0220** `buddy_reactions`.
- **Web:** under every post, one click gives the reaction that fits the news (congrats on an
  offer, luck on an interview, rooting on an application); hover or the arrow opens all five;
  beside it the most chosen kinds and the total. A folded applications post reacts on its
  first application.

Notifying the author is a follow-up.

## Capabilities

### Modified Capabilities
- `buddies-social-space`: reactions on feed posts.

## Impact

- **Go:** `internal/engage/buddy` (`reaction.go`, feed entries), `buddies.sql`, handler routes.
- **Web:** `buddyReactions.ts`, `BuddyReactionBar`, `BuddyPost`, `api.ts`, `types.ts`.
