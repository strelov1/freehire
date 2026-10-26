## 1. Schema and queries

- [x] 1.1 Migration `0199_buddies.sql` (since split into `0202_messaging.sql` + `0203_buddies.sql` by messaging-core): `buddy_members`, `buddy_hidden_applications`, `buddy_conversations`, `buddy_messages`, `buddy_blocks`, `buddy_reports` with CHECKs, unique pair index, cascades (design D2, D6, D7, D10); passes `pnpm check:sql`
- [x] 1.2 Migration `0200_application_events_feed_idx.sql` (now `0204_…`) (no-transaction, `CREATE INDEX CONCURRENTLY IF NOT EXISTS`) for the feed keyset read (D5)
- [x] 1.3 `internal/platform/db/queries/buddies.sql` membership queries (upsert join/rejoin keeping handle, leave, get, update ranking keys) + `make sqlc`
- [x] 1.4 Register `buddy` in `internal/platform/arch/layering/blocks.go` under `engage`; package skeleton `internal/engage/buddy` with `AGENTS.md`, linked from the root AGENTS.md table

## 2. Membership and profile

- [x] 2.1 Handle generation (adjective-noun-NN, unique, retry on collision) and `Join`/`Leave`/`Status`; leaving closes every conversation (integration tests: join mints, rejoin keeps handle, leave closes)
- [x] 2.2 Ranking keys from CV/profile (`category` via classify on `PrimaryTitle`, fallback to specializations; countries/regions from resume geography, fallback to base country) and `touch` refresh (unit test of the fallbacks)
- [x] 2.3 `Profile(viewer, handle)` projecting `talentnetwork.ProjectCard`; empty-card flag; non-member → `ErrNotMember` (test: card carries no summary/highlight/employer text; buddy handle ≠ talent handle)

## 3. Similar members

- [x] 3.1 Score weights as named constants + Go unit test of the ordering rule (closer first, zero-score still listed)
- [x] 3.2 `ListSimilarBuddies` SQL (category, countries, regions, shared applied companies in 90 days, cap 5, excludes viewer/left/blocked both ways/hidden) + service method (integration tests per spec scenarios)

## 4. Feed and hidden applications

- [x] 4.1 `ListBuddyFeed` SQL: applied + stage_set interview/offer only, day precision, keyset cursor, `all` and `similar` (top 200) modes, excludes viewer/left/blocked/hidden/retracted (integration tests: rejection and accepted never appear, pre-join events included, left member gone)
- [x] 4.2 Hide/show an application (`buddy_hidden_applications`) + list hidden application ids for the tracker (integration test: hide removes, show restores)

## 5. Conversations and chat

- [x] 5.1 `Invite`: 1..500 chars, one conversation per pair, 10 per 24h, refuses non-member/left/blocked either way; conversation + first message in one tx (integration tests per scenario)
- [x] 5.2 `Accept`/`Decline` by the recipient only; declined shown to the initiator as outgoing/unanswered
- [x] 5.3 `Send` (1..4000, open only; pending refuses the inviter), `Messages(after)`, `MarkRead`, `ListConversations` with unread counts and `status_for_me`; outsider → not found
- [x] 5.4 `Block` (closes conversation, hides pair everywhere) and `Report` (closed reasons, unique per reporter) + moderator `ListReports`/`Resolve`

## 6. HTTP API

- [x] 6.1 `internal/api/handler/buddies.go`: `/api/v1/me/buddies` routes (membership, profile, similar, feed, hidden jobs, conversations, messages, read, accept/decline, block, report), cookie-only `RequireAuth`, error mapping (400/403/404/409/429), response shapes (integration tests incl. 401 without session)
- [x] 6.2 Moderator routes `GET /moderation/buddy-reports`, `POST /moderation/buddy-reports/:id/resolve` behind the existing moderator gate (integration test: non-moderator 403)
- [x] 6.3 Update `web/static/openapi.yaml` only if these routes belong to the documented surface (check how referrals are treated; skip with a note otherwise)

## 7. Notifications worker

- [x] 7.1 `buddy.NotifyUnread`: eligibility query (unread from the other side older than 5 min, pending/open, not yet notified since), claim-then-send, release claim on failure (integration tests: one per conversation, recent waits, re-notify after read + new message, declined/closed skipped)
- [x] 7.2 Delivery: email via `emailnotify` honouring `emailprefs`, Telegram when linked, `RecordNotification` with dedup key; body carries no message text or account detail
- [x] 7.3 `cmd/buddy-notify` on `worker.Main`, no-op without mail config; document it in root AGENTS.md worker list

## 8. Web

- [x] 8.1 `api.ts` client methods + types for every buddy route
- [x] 8.2 `/my/buddies` page: join screen (explains companies are visible to members), tabs Similar / Feed / Chats, leave control; navigation entry
- [x] 8.3 Similar list cards (card summary, shared companies, Invite dialog with 500-char counter and 429 message)
- [x] 8.4 Feed with similar/all switch and cursor paging; entry opens profile
- [x] 8.5 Chats: incoming invitations (accept/decline), conversation view with 5 s polling while visible, composer, block and report actions; list polls unread every 30 s
- [x] 8.6 Tracker card switch "Show in buddy feed" (only for members)
- [x] 8.7 Moderation tab `buddies` in `ModerationView.svelte` with report queue and resolve
- [x] 8.8 `/privacy` section on stored buddy messages, moderator access via reports, deletion; `/terms` if conduct rules live there

## 9. Ship

- [x] 9.1 Full checks: `gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, tagged tests for `internal/engage/buddy` and `internal/api/handler` buddies, `pnpm run check` + web tests, `pnpm check:links`
- [ ] 9.2 Build the 0200 index CONCURRENTLY on prod by hand before merge; PR, CI green, merge, deploy
- [ ] 9.3 `freehire-ops`: `freehire-message-notify.{service,timer}` (was `freehire-buddy-notify`, renamed by messaging-core) (5 min, `.env` + `.env.notify`), copy to host, `daemon-reload`, enable; verify one run in journal
- [ ] 9.4 Verify on prod: join, invite between two test accounts, chat, notification arrives, feed shows an applied event, block hides

## 10. Owner review follow-ups

- [x] 10.1 Leave and block confirm through the design-system `ConfirmDialog`
- [x] 10.2 Member page `/my/buddies/<handle>` on a shared `CandidateCardView` (also used by `/talent/<handle>`), no photo (design D13)
- [x] 10.3 A "Message" button on member cards and the member page that opens the existing conversation or the first-message dialog
- [x] 10.4 Messaging dock in the root layout: list plus up to two windows, mobile sheet, hidden on `/my/buddies`, window ids only in sessionStorage, drafts in memory (D15)
- [x] 10.5 Re-open a conversation closed by a leave with a new invitation; a block keeps it closed (D7)
- [x] 10.6 Restyle buddies like the Talent Network: dictionary heading, silhouette, feed cards
- [x] 10.7 Member's own feed (`?handle=`) and year activity grid, guarded by `TestMemberActivityMatchesMemberFeed` (D14)
- [x] 10.8 UI audit fixes: message sides, drafts, focus return, loading/empty/error states
- [x] 10.9 Skills listed inline on the buddy page instead of a side rail
- [x] 10.10 Support toast suppressed on `/my/buddies`, where it covered the composer
