## Why

The in-app AI assistant at `/my/assistant` spawns one throwaway session on
mount: there is no way to start a fresh conversation without losing the current
one, and no way to return to an earlier conversation. Every page load strands
whatever was said before. Moderators using the assistant for real work need to
keep several conversations and move between them — the same session model
roy-web already ships.

## What Changes

- Add a roy-web-style **left sidebar** to `/my/assistant` listing the signed-in
  user's held sessions, with a **"New chat"** button and a per-session **delete**
  control. The chat transcript moves into the right pane.
- **Switch sessions** by clicking one: detach the current attach and re-`attach`
  to the selected session with `from_seq: 0`, replaying its journal through the
  existing `reduceTurnEvent` reducer so the full history repaints.
- Back the session list with the agent backend: **`GET /sessions` scoped to the
  caller** (today it leaks every user's sessions) and a new
  **`DELETE /sessions/{id}`** (verify owner → `daemon.close` if live → delete the
  `session_meta` row). **BREAKING** (backend contract): `list_sessions` gains an
  owner filter.
- Copy tweaks on the page: rename **"Assistant" → "Agent"** (heading + title +
  the account-nav label) and drop **"Claude"** from the subtitle ("Chat with a
  Claude agent inside freehire" → "Chat with an agent inside freehire").
- Introduce a **beta-tester group** (a `beta_tester` flag on `users`, separate
  from `role` so it coexists with moderator/admin) and gate the assistant to it:
  the page and the nav item show **only to beta testers** (moderators without the
  flag lose access). Membership is granted by **manual SQL** (`UPDATE users SET
  beta_tester = true WHERE lower(email) = …`), consistent with the manual-migration
  convention; the first member is the maintainer's account.

## Capabilities

### New Capabilities
- `assistant-sessions`: multi-session management for the in-app agent chat —
  listing the caller's sessions, creating/switching (with journal replay) and
  deleting them, and the owner-scoped backend contract (`GET /sessions`,
  `DELETE /sessions/{id}`) the frontend consumes.

### Modified Capabilities
<!-- No existing spec captures the assistant chat; the session behavior is net-new. -->

## Impact

- **Frontend (`hire/web`)**: `web/src/routes/my/assistant/+page.svelte` (sidebar +
  switch/new/delete + copy), `web/src/lib/assistant/api.ts` (list/delete fetch
  helpers, session-list wire types), and the attach flow (`from_seq: 0` replay).
  A pure session-list store/reducer is unit-tested (vitest), mirroring
  `chat.ts`/`jobFit.ts`.
- **Backend (`freehire-agent`, separate repo + deploy)**:
  `crates/roy-management/src/http.rs` — scope `list_sessions` by `created_by`,
  add `DELETE /sessions/{id}`; `meta_store.rs` — add `delete_session_meta`.
  Shipped as its own PR and deployed to `agent.freehire.dev` before the frontend
  relies on the new contract.
- **Auth/privacy**: closes an existing cross-user leak in `GET /sessions`.
  Session ownership on `attach`/`delete` is already enforced via
  `session_owner`.
- **Beta-tester group (`hire`)**: a `beta_tester` boolean on `users` (migration
  `0019`), carried through sqlc (`users.sql`), `accounts.User`, and the `/auth/me`
  `userResponse` (`beta_tester` field); the frontend `User` type + `accountNav`
  gate (`betaOnly`) + the assistant page guard read it. Membership set by manual
  SQL. No new env.
