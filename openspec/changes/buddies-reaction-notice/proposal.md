## Why

Reactions (`buddies-reactions`) reach nobody: the author of a post is never told that
another member congratulated them or wished them luck, so the feature is invisible to the
one person it is for.

## What Changes

- A reaction puts a notice in the author's notification center — "A buddy reacted
  “Congrats” to your offer from Stripe." — once per (event, reacting member): changing the
  reaction, or taking it back and giving it again, adds nothing; another member adds one.
  The notice never names who reacted. In-app only: no email or push, which per reaction
  would be noise. Best-effort: the reaction stands if the notice fails.
- The notification kind `buddy_reaction` opens `/buddies`. The messaging core's
  `buddy_message` and `buddy_invitation`, which were missing from the web's kind list and
  rendered with no icon, are registered too and open `/buddies/chats`.
- No migration: `user_notifications.dedup_key` already makes the notice once-only.

## Capabilities

### Modified Capabilities
- `buddies-social-space`: the author hears about a reaction.

## Impact

- **Go:** `internal/engage/buddy` (`reaction.go`, repository), `BuddyEventAuthor` query.
- **Web:** `types.ts` (`NotificationKind`), `notificationTarget.ts`, `NotificationCard.svelte`.
