## Context

`/my/assistant` (moderators-only, PR #679) is a SvelteKit page that on mount
`POST /sessions` → spawns one agent session, opens a WebSocket relay, `attach`es,
and streams frames through the pure `reduceTurnEvent` reducer (`chat.ts`).
There is no session list, no "new", no switch, no delete. The agent backend is
`freehire-agent` (a trimmed roy fork, `agent.freehire.dev`, separate repo).

Backend facts established by exploration:

- **WS relay allow-list** (`ws.rs`): `attach, acquire_input, send, cancel_turn,
  release_input, detach`. `list`, `read_journal`, `close`, `delete_archive` are
  rejected over WS. Every op is ownership-checked via `meta.session_owner`.
- **`attach {session, from_seq}`** (`engine.rs:401`) calls
  `journal.replay_from(from)` (memory ring → disk fallback) and streams the
  replayed entries as `frame` events, then live frames — so `from_seq: 0`
  repaints full history through the same reducer.
- **HTTP** (`http.rs`): `GET /sessions` exists but is **not** owner-scoped
  (returns all users' sessions). `POST /sessions` creates. No `DELETE`.
  `session_meta.created_by` is the owner; `daemon.close(id)` archives a live
  session; there is no `delete_session_meta` yet.

## Goals / Non-Goals

**Goals:**

- List the caller's held sessions in a left sidebar; click to switch (history
  replays); "New chat"; per-session delete.
- Close the cross-user leak in `GET /sessions` (owner-scope it) and add
  `DELETE /sessions/{id}`.
- Keep the streaming/turn/lease machinery that already works; add session
  orchestration around it.
- Copy: "Assistant" → "Agent"; drop "Claude" from the subtitle.

**Non-Goals:**

- Session tags/rename/pinning, projects, harness/model pickers (roy-web has
  these; out of scope).
- Cross-device anything beyond what owner-scoped `GET /sessions` already gives.
- Deleting the on-disk archive journal (inert; swept by `orphan_sweep`). Delete
  = close + drop the meta row.
- Any change to the WS relay allow-list (all needed ops are already allowed).

## Decisions

**1. Backend-backed session list (owner-scoped `GET /sessions`), not a
client-side registry.** Chosen in brainstorming for cross-device sync and true
delete. `list_sessions` gains `Extension(AuthUser(user_id))` and filters the
assembled rows to those whose meta `created_by == user_id`. Rows without a meta
(no owner attributable) are excluded. *Alternative — localStorage registry:*
zero backend change, but per-browser and can't truly delete; rejected.

**2. `DELETE /sessions/{id}` = owner-check → close-if-live → delete meta.**
Verify `session_owner(id) == user_id` (else 404, not 403 — don't reveal
existence of others' sessions). If live (`daemon.list` contains it), `daemon.close(id)`
to archive the running process. Then `meta.delete_session_meta(id)` (new
`MetaStore` method). Dropping the meta row makes `session_owner` return `None`,
so the session vanishes from the owner-filtered list and any future `attach` is
rejected as not-owned — no new WS op needed. *Alternative — add a `delete_archive`
WS/daemon path:* more complete (removes disk journal) but larger surface; the
archive is inert, so deferred.

**3. Switching = detach + re-attach `from_seq: 0`, reusing `reduceTurnEvent`.**
On switch: tear down the current session's frame subscription and `detach`,
reset `chat = initChat()`, `attach {session, from_seq: 0}`, and subscribe. The
replayed `frame` events fold through the exact same reducer as live frames, so
history and live streaming share one code path. The optimistic-echo dedup
(`pendingEcho`) only applies to locally-sent messages, so replay is unaffected.
*Alternative — `read_journal` HTTP/WS fetch then seed:* rejected — `read_journal`
is not in the relay allow-list, and attach-replay already does exactly this.

**4. One WS connection, one active attach at a time.** Keep the single
`RoyClient` per page. Model an explicit "active session" with its own
subscription/lease; switching fully unwinds the old one (unsubscribe, detach,
release lease if held, end any in-flight turn) before attaching the new one.
This avoids interleaving frames across sessions and reuses the existing
teardown logic. *Alternative — attach many sessions at once (roy-web bg-attach):*
unnecessary for a single visible pane.

**5. Pure session-list state in a testable module.** A small
`web/src/lib/assistant/sessions.ts` holds the session-list type + label
derivation + list reducers (add/remove/select/sort newest-first), unit-tested
with vitest — mirroring how `chat.ts` and `jobFit.ts` isolate pure logic from
the Svelte component. Labels: prefer a stored `display_label`/tag, else derive
from the first user message on attach, else a timestamp.

**6. Ship backend first.** The owner-scope + DELETE land and deploy to
`agent.freehire.dev` before the frontend depends on them. The frontend degrades
safely if the list call fails (fall back to the current single-spawn behavior,
surfaced as a non-fatal error) so a deploy-order slip doesn't white-screen the
page.

## Risks / Trade-offs

- **Cross-repo, two deploys** → Backend PR + deploy first (Decision 6); frontend
  guards the list call so a lag is a degraded (single-session) page, not a break.
- **`GET /sessions` owner-scope is a BREAKING contract change** → Only consumer
  today is this page (which sends the cookie); scoping strictly reduces what's
  returned. Verified by a backend test asserting cross-user exclusion.
- **Newest-first ordering has no reliable timestamp in the list payload** →
  `list_sessions` returns `session_id, project_id, agent_name, tags, live` — no
  `created_at`. Add `created_at` (present on `session_meta`) to the row, or sort
  by the `live` flag then session id. Decide in tasks; leaning on adding
  `created_at` to the row for a correct newest-first sort.
- **Deleting the active session** → switch to the next listed session, or spawn a
  fresh one if the list is now empty; never leave the pane attached to a deleted
  id.
- **Replay of a very long journal** → attach streams the whole journal as frames;
  acceptable for chat-length transcripts. The memory-ring→disk path already
  handles eviction. No pagination for now.
- **Label from first user message requires reading the journal** → on attach the
  first replayed `user_prompt` frame yields the label; until then show a
  timestamp/placeholder. No extra fetch.

## Migration Plan

1. **Backend (`freehire-agent`)**: owner-scope `list_sessions`, add
   `DELETE /sessions/{id}` + `MetaStore::delete_session_meta` (+ `created_at`
   on the list row if used for sort). Tests. PR, merge, deploy to
   `agent.freehire.dev`.
2. **Frontend (`hire/web`)**: `sessions.ts` (pure) + api helpers (`listSessions`,
   `deleteSession`) + sidebar UI + switch/new/delete wiring + `from_seq: 0`
   attach + copy tweaks. Vitest for `sessions.ts`.
3. **`hire` DB migration `0019_user_beta_tester.sql`** (adds the `beta_tester`
   column) — apply to prod **manually before deploying** the hire backend, per
   the migrations convention. Then grant the first members out-of-band:
   ```sql
   UPDATE users SET beta_tester = true WHERE lower(email) = 'strelov1@gmail.com';
   ```
   (Repeat the `UPDATE` per email to add more testers.) No new env.
4. Rollback = revert each repo independently (the frontend guard makes the
   `freehire-agent` and `hire` deploys order-independent after step 1 ships).

## Open Questions

- Sort key for newest-first: add `created_at` to the `GET /sessions` row (clean),
  or accept session-id ordering? — Lean `created_at`; confirm during backend task.
- Delete UX: inline "×" with a confirm, or hover-reveal trash like roy-web? —
  Decide during the sidebar task; default to hover-reveal trash + lightweight
  confirm for a session with history.
