## 1. Branch and migrations

- [x] 1.1 Rebase `interview-buddies` onto `origin/main`; renumber the buddy migrations after 0201 as 0202_messaging, 0203_buddies, 0204_application_events_feed_idx (design D8)
- [x] 1.2 Write `0202_messaging.sql` (conversations, conversation_participants, messages, user_blocks, conversation_reports) and strip the old chat tables from `0203_buddies.sql`; `pnpm check:sql` clean
- [x] 1.3 Rewrite the chat queries in `internal/platform/db/queries/` over participants (new `messaging.sql`), `make sqlc`

## 2. Messaging core

- [x] 2.1 Create `internal/engage/messaging` (types, `Kind` interface, Repository, Service) and add it to `internal/platform/arch/layering/blocks.go`
- [x] 2.2 `Start` (blocks, pair key, reopen closed, body length) with integration tests
- [x] 2.3 Accept / Decline / Send / Messages / MarkRead / List with the outsider-404 test across every operation
- [x] 2.4 Account-wide Block and Report, moderator ListReports / ResolveReport
- [x] 2.5 Notifier over per-participant `notified_at` with kind-supplied wording; release on a non-cancellable context; tests moved from buddy
- [x] 2.6 Account deletion removes participations, conversations, messages, reports, blocks (integration test)

## 3. Buddy on the core

- [x] 3.1 Buddy kind: `Counterparts` (handle, card summary, reachable) and `Notice`; register it
- [x] 3.2 `buddy.Service.Invite` checks membership and the daily limit, then calls `messaging.Start`; delete buddy chat/moderation/notify code now in the core
- [x] 3.3 Feed, similar list and profile read `user_blocks`; existing buddy tests green

## 4. API and worker

- [x] 4.1 Handlers for `/me/conversations/...`, `POST /me/buddies/invite`, `/moderation/conversation-reports`; move the handler integration tests
- [x] 4.2 `cmd/buddy-notify` → `cmd/message-notify` (`MESSAGE_NOTIFY_MAX_PER_RUN`, startup validation kept); update AGENTS.md worker list and `web/static/openapi.yaml` if it lists the routes

## 5. Web

- [x] 5.1 API client and types: conversation routes, discriminated `counterpart`
- [x] 5.2 Rename and generalize dock, pane, row, unread badge, reports queue; buddy counterpart snippet; svelte-check, vitest, knip clean

## 6. Ship

- [ ] 6.1 Full checks: gofmt, go vet (+integration tags), go test, tagged suite for messaging/buddy/handler, web lint/tests
- [ ] 6.2 Update `freehire-ops#119` to the `freehire-message-notify` unit
- [ ] 6.3 Push, CI green, CodeRabbit addressed; then merge, release, enable the timer, verify on prod
