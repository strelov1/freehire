## 1. Schema
- [x] 1.1 Migration 0196: `users.agents_may_apply`, `auto_apply_queue.submit_uncertain_at`,
      `auto_apply_events` table + index, transition trigger. Integration test: each
      transition writes the expected event.
- [x] 1.2 Queries: `DeleteAutoApplyEntry` records `submitted`; `MarkAutoApplySubmitUncertain`;
      `RetryAutoApplyEntry`; `CancelAutoApplyEntry` (records `cancelled`);
      `ListAutoApplyEntries`; `ListAutoApplyEvents`; agent-permission get/set. `make sqlc`.

## 2. Worker
- [x] 2.1 `autoapply.Store.DeadLetterUncertain`; `deadLetterImmediately` uses it; dbStore impl.

## 3. Use cases + HTTP
- [x] 3.1 Extract enqueue into a transport-free method; route stays behaviour-identical.
- [x] 3.2 Extract review likewise.
- [x] 3.3 Retry + cancel methods and routes (`POST /me/auto-apply/:queueId/retry`,
      `DELETE /me/auto-apply/:queueId`), with integration tests.
- [x] 3.4 `GET/PUT /me/agent-permissions` (PUT cookie-only).

## 4. MCP tools
- [x] 4.1 Auto-apply tools (start, list, get, review, retry, cancel) gated by the switch.
- [x] 4.2 Answer bank + screening tools; register all on the account server; update
      instructions. In-memory SDK test.

## 5. Web
- [x] 5.1 Connected devices: agents-may-apply switch.
- [x] 5.2 JobDrawer: Retry / Cancel for blocked/failed/pending attempts.

## 6. Docs + ship
- [x] 6.1 AGENTS.md: fix stale "not yet built" line, describe events/retry/cancel/switch.
- [ ] 6.2 Full verification, PR, CI, merge, deploy, prod check.
