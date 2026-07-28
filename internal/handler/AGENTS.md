# internal/handler — HTTP Handlers

Fiber HTTP handlers: feature handler structs, route registration, auth surface, user job endpoints, error rendering.

## Architecture

- `API` (`handler.go`) holds only the cross-cutting dependencies: the DB pool, the sqlc
  queries, and the token issuer the auth middleware is built from. `Register` builds the
  shared services, constructs every feature handler, and calls each feature's `register`
  in an order that keeps literal routes before param routes (e.g. `/jobs/search` before
  `/jobs/:slug`, `/threads/count` before `/threads/:id`, the static `/me/tracking/*`
  before `/me/tracking/:slug`).
- Each feature area owns a `<feature>Handlers` struct with its own dependencies, a
  `new<Feature>Handlers` constructor, and a `register(api fiber.Router, mw middleware)`
  method that mounts its routes — `grep 'Handlers struct' *.go` lists them all. Files are
  named after the routes they serve, so a feature spans several files (`matchHandlers` →
  `match_analysis.go`, `match_analysis_stream.go`, `job_match.go`, …). Handlers are thin;
  auth primitives, user job operations, API key management and error rendering live in
  their own files.
- Two couplings worth knowing: `cvHandlers` holds a `*matchHandlers` (it reuses the
  blocker/credits helpers for tailoring), and `jobs_moderation.go` carries the
  moderator-authored writes behind the `moderator` gate.
- The `middleware` bundle (`handler.go`) carries the shared auth gates features mount
  behind: `optional` (attach caller, never reject), `key` (cookie or API key), `cookie`
  (cookie-only), `moderator` (role gate, stacked after `key`/`cookie`).
- Services shared across features (résumé store, profile, credits, CV store/renderer,
  match analyzer, contribution, moderation) are built once in `Register` and passed to
  each constructor; single-feature services are built inside the feature's constructor.
- Tests construct the feature struct directly with fakes/stubs (e.g.
  `&trackingHandlers{tracking: ...}`) and mount routes on a bare `fiber.App`.
- Central `handler.RenderError` (wired in `cmd/server` via `fiber.Config{ErrorHandler: handler.RenderError}`) renders JSON envelope: `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK-violation (SQLSTATE 23503)→404, everything else→500.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, bare error (e.g. `pgx.ErrNoRows`) for common cases. Don't hand-roll per-handler error JSON; don't re-map `ErrNoRows` in read handlers (just `return err`).

## Auth Handlers (`auth.go`)

- `register`/`login` set JWT cookie + return `{"data": user}`. `logout` clears it. `me` is guarded by `RequireAuthOrKey` (cookie or API key).
- Rate-limited credential endpoints (10/min, keyed on client IP).

## Account Deletion (`me_delete.go`)

- `DELETE /api/v1/me` erases the caller's account permanently — no soft-delete, no grace period, no restore. Cookie-only (`RequireAuth`), never `keyAuth`: a leaked API key must not destroy the account that issued it.
- The body confirms the caller's **own** email (case-insensitive, matching the login lookup); a mismatch is 400 and erases nothing. Success is 204 plus an expired session cookie.
- The orchestration lives in `internal/accountdelete`, not here: object keys are collected, the Gmail grant revoked (best-effort, shared with `GmailDisconnect` via `revokeGmailGrant`), objects deleted, and only then the row — so a storage failure leaves the account whole. That case surfaces as **503**, meaning "nothing was deleted, retry", and is the one status worth knowing: `ErrStorageUnavailable` must not fall through to a 500.

## User Job Handlers (`user_jobs.go`)

- `view`/`apply`/`save`/`track` interaction endpoints. Addressed by job's public `:slug` (resolved to internal id before write). All writes are idempotent upserts behind `RequireAuthOrKey`.
- Return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.

## Assistant (`assistant.go`, `assistant_*_tools.go`)

Routes (all cookie-only, behind `auth.RequireModeratorOrBeta` — inference is
billed to us, so the assistant is not open to everyone while it is free):

| Route | Does |
|---|---|
| `POST /assistant/sessions` | start a chat conversation (a tailoring one is minted by the CV bootstrap, which knows the CV and vacancy to bind) |
| `GET /assistant/sessions` | the caller's conversations, most recently active first |
| `GET /assistant/sessions/:id` | one conversation with its stored transcript, for replay |
| `DELETE /assistant/sessions/:id` | remove a conversation and its transcript |
| `POST /assistant/sessions/:id/messages` | run one turn, streamed as named SSE events |

A session the caller does not own is a 404, never a 403, so ids stay unprobeable.

The turn endpoint writes with `writeEvent`, which — unlike `writeSSE` — reports a
failed write. That is how a streamed turn learns the client is gone: the failure
cancels the loop's context, so it stops before spending another model call.

`assistant_tools.go` / `assistant_tracking_tools.go` / `assistant_cv_tools.go` /
`assistant_profile_tool.go` build the agent's tools from the same services these
handlers use, and `assistantRegistry` picks the set for a session's preset. The loop
itself lives in [internal/assistant](../assistant/AGENTS.md).

`get_profile` is built on `profileHandlers` rather than the services under it, so the
tool and `GET /me/profile` share one assembly and cannot drift. It returns the CV as
`resumeextract.Professional` — **contacts omitted**, and not merely as good manners: a
tool result is persisted in the transcript and replayed into the model's context on
every later turn, so a name that lands there stays for the conversation's life. A
caller with no profile gets a result naming `/my/profile`, never an error and never an
empty profile the model would read as "no preferences".

## Error Convention

- Genuinely domain-specific status choices (e.g. `Me` returning 401 for a gone user token) stay in the handler.
- Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- Sentry reports only fall-through unexpected 500s — routine errors never reported.
