## Why

The interview-buddies chat (`buddy_conversations`, `buddy_messages`, buddy blocks, reports and
the unread notifier) is built for exactly two pseudonymous members and nothing else. The next
uses already on the table — a company writing to a candidate, and operator announcements landing
in the inbox — would each need a second chat stack, or a schema rework after the buddy tables
hold real data. Neither the tables nor the feature have reached production yet, so this is the
cheapest moment to put one messaging core under the buddy chat.

## What Changes

- New `internal/engage/messaging` package owning conversations, participants, messages, read
  cursors, account-wide blocks, reports and the unread-notice worker. A conversation carries a
  `kind`; today the only kind is `buddy`.
- The buddy package keeps what is buddy-only (membership, card, feed, similar, activity grid,
  the invitation limit) and starts its conversations through the core.
- **BREAKING (unreleased API):** chat routes move from `/me/buddies/conversations/...` to
  `/me/conversations/...`; the buddy invitation becomes `POST /me/buddies/invite`; moderation
  moves from `/moderation/buddy-reports` to `/moderation/conversation-reports`. Nothing outside this
  branch calls the old routes.
- Each conversation in the API carries a `counterpart` describing how to draw the other side,
  supplied by its kind.
- `cmd/buddy-notify` becomes `cmd/message-notify`; its unit in `freehire-ops` is renamed.
- Web: the dock, conversation pane and conversation row become kind-agnostic components.
- The unreleased buddy migrations are rewritten and renumbered after `main`'s latest.

Company↔candidate chat and inbox broadcasts are NOT built here; they arrive later as new kinds.

## Capabilities

### New Capabilities
- `messaging`: conversations of any kind between accounts — request/accept, messages, read
  state, account-wide blocks, reports to moderators, unread notices, deletion.

### Modified Capabilities
None in `openspec/specs/`. The unarchived `interview-buddies` change's spec is edited in place:
its chat, block/report and notification requirements move here, and its invitation
requirement narrows to the buddy-only rules.

## Impact

- Migrations: buddy migration rewritten; new messaging tables.
- Go: new `internal/engage/messaging`; `internal/engage/buddy` slimmed; handlers in
  `internal/api/handler`; layering table in `internal/platform/arch/layering/blocks.go`;
  `cmd/buddy-notify` → `cmd/message-notify`.
- Web: buddy chat components generalized; API client and types.
- Ops: `freehire-ops#119` renames the worker unit.
