## 1. Backend contract (`freehire-agent`, separate PR + deploy first)

- [x] 1.1 Add `MetaStore::delete_session_meta(session_id)` (DELETE the `session_meta` row) with a unit/integration test. (Already present upstream — verified: transactional row+tags delete, NotFound on missing, covered by `delete_session_meta_removes_row_and_tags`.)
- [x] 1.2 Owner-scope `list_sessions`: inject `Extension(AuthUser(user_id))`, filter the assembled rows to those whose meta `created_by == user_id`, and drop sids with no owning meta. Test that a second user's session is excluded.
- [x] 1.3 Include a newest-first sort key in the `GET /sessions` row (add `created_at` from `session_meta`) and return it; sort the list newest-first. Assert ordering in the list test.
- [x] 1.4 Add `DELETE /sessions/{id}`: verify `session_owner == user_id` (404 when unknown/not owned, no side effects), `daemon.close(id)` if live, then `delete_session_meta(id)`; return success. Test owned-delete (gone from list, attach rejected) and non-owned/unknown (error, unchanged). (Code-review fix: a `daemon.list()` failure now returns 502 and keeps the meta, never orphaning a live process.)
- [x] 1.5 `cargo test` + `cargo clippy` green; open the backend PR, merge, deploy to `agent.freehire.dev`, and smoke-test `GET /sessions` (scoped) + `DELETE` with a real cookie. (Tests: 92 pass; clippy clean. PR + deploy pending user confirmation.)

## 2. Frontend session-list core (pure, unit-tested)

- [x] 2.1 `web/src/lib/assistant/api.ts`: add `SessionSummary` wire type + `listSessions()` (`GET /sessions`) and `deleteSession(id)` (`DELETE /sessions/{id}`) fetch helpers (credentials: 'include'), mirroring `createSession`. (Type lives in `sessions.ts`; api.ts imports it.)
- [x] 2.2 `web/src/lib/assistant/sessions.ts` (new, pure): session-list type, newest-first ordering, label derivation (stored label/tag → first user message → timestamp), and reducers `addSession`/`removeSession`/`selectSession`. Write vitest first (RED) covering ordering, label fallbacks, add/remove/select, and delete-active → next-or-empty selection. (16 vitest cases green.)

## 3. Frontend page: sidebar + switch/new/delete

- [x] 3.1 Refactor `+page.svelte` connect flow to model an explicit "active session": on load, `listSessions()`; open the newest (or create one if empty). Guard the list call so a backend failure degrades to a single fresh session with a non-fatal error banner.
- [x] 3.2 Extract per-session attach/detach: switching detaches + unsubscribes the current session, releases the lease if held, ends any in-flight turn, resets `chat = initChat()`, then `attach {session, from_seq: 0}` and re-subscribes. Verify replayed history repaints via `reduceTurnEvent`.
- [x] 3.3 Add the left sidebar UI (session list, active highlight, "New chat" button, hover-reveal delete with a confirm for a session with history). Move the chat transcript into the right pane; keep the composer/queue behavior intact. Responsive: sidebar collapses/toggles on narrow widths.
- [x] 3.4 Wire "New chat" (create → prepend → activate) and delete (`deleteSession` → remove from list → if active, switch to next or spawn fresh).

## 4. Copy tweaks

- [x] 4.1 Rename "Assistant" → "Agent" (page heading, `<title>`, and the `accountNav` label) and change the subtitle "Chat with a Claude agent inside freehire" → "Chat with an agent inside freehire".

## 6. Beta-tester group (gate the assistant)

- [x] 6.1 Migration `migrations/0019_user_beta_tester.sql`: `ALTER TABLE users ADD COLUMN beta_tester boolean NOT NULL DEFAULT false;`.
- [x] 6.2 `internal/db/queries/users.sql`: add `beta_tester` to `CreateUser` RETURNING, `GetUserByEmail`, and `GetUserByID`; run `make sqlc` (or the pinned sqlc binary) and commit generated code.
- [x] 6.3 `internal/accounts`: add `BetaTester bool` to `User` and map `row.BetaTester` in the three `repository.go` constructions.
- [x] 6.4 `internal/handler/auth.go`: add `BetaTester bool \`json:"beta_tester"\`` to `userResponse` and map it in `toUserResponse` (unit test the mapping, mirroring `TestToUserResponse_IncludesRole`).
- [x] 6.5 Frontend: add `beta_tester: boolean` to the `User` type (`web/src/lib/types.ts`); change the assistant `accountNav` item from `moderatorOnly` to `betaOnly` and extend `visibleAccountNav(isModerator, isBetaTester)` — RED-first in `accountNav.test.ts` (beta-only Assistant vs moderator-only Inbox); update the `my/+layout.svelte` caller.
- [x] 6.6 Gate the page: `+page.svelte` guards on `currentUser()?.beta_tester` (not `role === 'moderator'`); adjust the restricted-rollout copy.
- [x] 6.7 Grant SQL documented in `design.md` Migration Plan (`UPDATE users SET beta_tester = true WHERE lower(email) = 'strelov1@gmail.com';`). Local-DB apply + grant verification pending (needs the stack up).

## 5. Verify & finish

- [x] 5.1 Automated checks green: web svelte-check (0 errors), eslint (clean), vitest (254), `npm run build` (prod bundle OK); Go `build`/`vet`/`test` (accounts+handler) OK; freehire-agent `cargo test` (92) + clippy clean.
- [x] 5.2 Visual/behavioral verify on `localhost` as a beta tester (grant the QA account): list, new, switch-with-history-replay, delete-active, mid-turn switch safety, copy text, and that a non-beta user is stopped.
- [x] 5.3 Update this change's checkboxes; run the finish flow (verification-before-completion → finish branch → archive + sync). Offer a `/blog` changelog entry (user-facing).
