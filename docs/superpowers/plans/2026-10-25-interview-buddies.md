# Interview buddies — execution plan

Source of truth: `openspec/changes/interview-buddies/` — `specs/interview-buddies/spec.md` (requirements and scenarios), `design.md` (decisions D1–D12), `tasks.md` (checkboxes). Every task below names the checkboxes, requirements and decisions it covers. Exact values (limits, weights, lengths, statuses) are in the spec and the design; use them verbatim.

## Global Constraints

- Repo rules: `AGENTS.md` at the repo root (English only, surgical changes, no overengineering, no MVP hacks). Read `internal/engage/AGENTS.md` and `internal/platform/db/AGENTS.md` before touching those areas.
- Package `internal/engage/buddy` (layer 7). It may import `platform`, `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search`, and nothing from `ingest` or `api`.
- `internal/platform/db/` is generated. Edit `internal/platform/db/queries/buddies.sql`, then run `make sqlc`.
- Migrations: `0199_buddies.sql` and `0200_application_events_feed_idx.sql` (renumbered and split by messaging-core: `0202_messaging.sql`, `0203_buddies.sql`, `0204_application_events_feed_idx.sql`) (the second has a no-transaction marker on its own comment line and uses `CREATE INDEX CONCURRENTLY IF NOT EXISTS`). Never edit an applied migration.
- Before every commit with Go: `gofmt -l .` prints nothing, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`. Run the tagged tests of the packages you touched: `go test -tags=integration ./internal/engage/buddy/... ./internal/api/handler/ -run Buddy`. These need Docker.
- Invoke the `simplify` skill on your diff before committing.
- Response shapes: `{"data": ...}` and `{"data": [...], "meta": {...}}`; errors `{"error": msg}`.
- Anonymity rule: nothing a member typed into their CV reaches another member, except role titles (`talentnetwork.ProjectCard`). Notifications carry no message text.
- Feed: `applied`, plus `stage_set` with signal `interview` or `offer`, only. Day precision. `accepted`, `rejected`, `withdrawn` and `expired` never appear.
- Limits: invitation body 1..500 chars, message body 1..4000 chars, 10 invitations per rolling 24h, one conversation per pair, notification after 5 minutes unread, once per conversation until read, similarity window 90 days, shared companies cap 5, similar-feed top 200 members.
- Commit messages: plain English, written to a file and passed with `git commit -F` (backticks in `-m` run as shell). End with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- After a task's checkboxes are done, flip them to `- [x]` in `openspec/changes/interview-buddies/tasks.md` in the same commit.

## Task 1: Schema, queries skeleton, package registration

Covers tasks.md 1.1, 1.2, 1.3, 1.4. Design D2, D5 (index), D6, D7, D10. Spec: every requirement's data needs, plus "Account deletion removes buddy data" (cascades).

- Write both migrations. `buddy_members(user_id PK → users ON DELETE CASCADE, handle text UNIQUE NOT NULL, category text, countries text[] NOT NULL DEFAULT '{}', regions text[] NOT NULL DEFAULT '{}', joined_at, left_at)`. The other five tables as design D6/D7/D10 describe, with CHECKs (status, reason, body lengths, `user_low < user_high`) and the indexes they need.
- `pnpm check:sql` must pass for the two new files.
- Membership queries in `buddies.sql`: insert-or-rejoin (keeps handle, clears `left_at`), leave (sets `left_at` and closes the member's conversations), get by user, get by handle, update ranking keys. Run `make sqlc`.
- Add `buddy` to the engage list in `internal/platform/arch/layering/blocks.go`. Create `internal/engage/buddy/doc.go` (package doc) and `internal/engage/buddy/AGENTS.md` (what it owns, the anonymity rule, the feed vocabulary rule). Add it to `internal/engage/AGENTS.md` and to the module table in the root `AGENTS.md`. `pnpm check:links` must pass.
- An integration test in `internal/platform/db/` (tag `integration`) that deleting a user cascades its membership, conversations and messages.

## Task 2: Membership and profile service

Covers 2.1, 2.2, 2.3. Design D2, D3, D11. Spec: "Opt-in buddy membership", "Anonymous buddy profile".

- `buddy.Service` over a `Repository` interface, built like `internal/engage/referral` (`New(...)`). Errors: `ErrNotMember`, `ErrNotFound` and the others as needed, as exported sentinels.
- The handle has the `community.GenerateHandle` shape (adjective-noun-NN). Write a buddy-local generator, or reuse an exported one if `community` exposes it cleanly. Retry on a unique violation.
- `Join`, `Leave`, `Status`, `touch` (refreshes ranking keys), and `Profile(viewer, handle)`, which projects `talentnetwork.ProjectCard` from `resume_structured`.
- Ranking keys: category = `classify.Parse(talentnetwork.PrimaryTitle(cv)).Category`, falling back to the first `user_profiles.specializations` entry. Countries and regions come from `users.resume_countries/regions`, falling back to the profile's base country (`userprofile.LocationPreferences.Base.Country`).
- Tests: a unit test of the fallbacks, and integration tests for each scenario of the two requirements (join mints, rejoin keeps the handle, non-member gets `ErrNotMember`, leave closes conversations, the card has no summary/highlight/employer text, the buddy handle differs from the talent handle, an empty CV gives an empty card flag).

## Task 3: Similar members

Covers 3.1, 3.2. Design D4. Spec: "Similar members ranking".

- Named weight constants in Go, passed into the SQL as parameters. A comment says they are a first guess.
- A `ListSimilarBuddies` query and a `Service.Similar(viewer, limit, offset)` returning card summary, handle, score and shared company slugs and names (at most 5).
- Tests: a unit test of the ordering rule, and integration tests for every scenario (closer first, zero-overlap still listed after, blocked excluded both ways, applications older than 90 days do not count; also excludes viewer, left members and hidden applications).

## Task 4: Feed and hidden applications

Covers 4.1, 4.2. Design D5, D6. Spec: "Buddy feed of applications and stage moves", "Per-application feed visibility".

- A `ListBuddyFeed` keyset query, `Service.Feed(viewer, mode, cursor, limit)` with modes `all` and `similar` (top 200). Entry shape per design D12.
- `HideApplication`, `ShowApplication`, `HiddenApplications(user)`, keyed on `application_id`.
- Integration tests for every scenario, plus: `accepted` never appears, retracted events never appear, the viewer's own events never appear.

## Task 5: Invitations

Covers 5.1, 5.2. Design D7. Spec: "Invitations".

- `Invite(from, toHandle, body)`: one transaction inserting the conversation and its first message. `Accept`, `Decline` by the recipient only.
- Integration tests for every scenario of the requirement, including the 429 path. The service returns a sentinel and the time the next invitation becomes possible.

## Task 6: Chat

Covers 5.3. Design D7, D12. Spec: "In-app chat".

- `Send`, `Messages(viewer, conversationID, after, limit)`, `MarkRead`, `ListConversations(viewer)` with unread counts and `status_for_me`.
- Integration tests for every scenario.

## Task 7: Block, report, moderation service

Covers 5.4. Design D10. Spec: "Block and report".

- `Block(viewer, conversationID)`, `Report(viewer, conversationID, reason, note)`, `ListReports` (with messages), `ResolveReport`.
- Integration tests: block closes the conversation and hides the pair from similar, feed and invite in both directions; the report is listed with its messages.

## Task 8: HTTP API

Covers 6.1, 6.2, 6.3. Design D11, D12. Spec: the 401/403/404/409/429/400 scenarios across requirements.

- `internal/api/handler/buddies.go` with `newBuddyHandlers`, registered in `handler.go` beside referrals. Routes under `/api/v1/me/buddies/...`, cookie-only auth. Moderator routes behind the same gate the company-feedback reported queue uses.
- Every route calls `touch` for members.
- Integration tests in `internal/api/handler` (`//go:build integration`) covering auth (401), non-member (403), outsider conversation (404), duplicate invite (409), the limit (429), a bad body (400), non-moderator (403), and one happy path per route.
- For 6.3, check whether referral or community routes appear in `web/static/openapi.yaml` or the internal one. Follow the same treatment and state the outcome in the report.

## Task 9: Unread notifications worker

Covers 7.1, 7.2, 7.3. Design D9. Spec: "Unread message notifications".

- `buddy.NotifyUnread(ctx, now)` doing claim, then send, then release on failure. Delivery follows `internal/engage/referral/pinger.go`: email via `emailnotify` honouring `emailprefs`, plus Telegram when linked, plus `RecordNotification` with a dedup key.
- `cmd/buddy-notify` (now `cmd/message-notify`, see messaging-core) on `internal/platform/worker` conventions. It is a no-op that never opens the pool when the mail config is missing. Add an entry to the root `AGENTS.md` worker list.
- Integration tests for every scenario (use fake transports).

## Task 10: Web — client, page, similar, feed

Covers 8.1, 8.2, 8.3, 8.4. Read `web/AGENTS.md` and `design-system/AGENTS.md` first.

- `web/src/lib/api.ts`: methods and types for every buddy route (shapes per design D12 and the handler code from Task 8).
- `/my/buddies`: a thin `+page.svelte` that renders a `BuddiesView` component, following the `ReferralsView.svelte` pattern (`onMount` + `AsyncData`).
  - The join screen says, in plain words, that members will see the companies you apply to and your anonymous card.
  - Three tabs: Similar, Feed, Chats (the Chats tab content comes in Task 11; render a placeholder only if needed to compile).
  - A leave control.
  - An entry in the account navigation.
- Similar cards with shared companies and an Invite dialog (500-char counter; show the 429 message from the API).
- Feed with a similar/all switch, cursor paging and "load more". An entry opens the author's profile.
- No Sparkles icon. Use design-system primitives (`EntityLogo` for company logos).
- Tests: vitest for any non-trivial pure logic. `pnpm run check` and the web lint must pass.

## Task 11: Web — chats, tracker switch, moderation, privacy

Covers 8.5, 8.6, 8.7, 8.8.

- Chats tab: incoming invitations (accept and decline), the outgoing ones shown as unanswered, and a conversation view that polls every 5 s while visible and refreshes on `visibilitychange` (pattern: `MentorBookingView.svelte`). It has a composer with a 4000-char limit and block and report actions. The list polls unread counts every 30 s.
- Tracker card: a "Show in buddy feed" switch for members only, calling hide/show.
- `ModerationView.svelte`: a new `buddies` tab with a queue component (reason, note, messages, resolve).
- `/privacy`: a section on stored buddy messages, moderator access only through a report, and deletion with the account. `/terms`: conduct rules, if conduct rules already live there.
- `pnpm run check`, web lint and tests pass.
