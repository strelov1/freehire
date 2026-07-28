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

## Mail Inbox + Agent Surface (`gmail.go`, `inbox.go`, `inbox_linking.go`, `inbox_agent.go`)

Pipeline and cross-package invariants live in [docs/agents/mail-stack.md](../../docs/agents/mail-stack.md); this section is the HTTP surface only.

- Mounted on `mw.key`, so a user's own agent harness drives the inbox with the full-scope key it already uses for the tracker. **Exception: the Gmail OAuth connect/callback pair stays `mw.cookie`** — it redirects a browser to Google's consent screen and a keyed client cannot complete it.
- `TriageEmail` is `SetEmailClassification`'s sibling: status, link, provenance (`link_source = 'agent'`) and the classified stamp in **one** update, then `mailclassify.AdvanceStage`. Splitting it would manufacture states the worker never produces. An omitted slug means "not deciding the link", never "clear it". The stage advance is best-effort — the verdict is already durable.
- `IngestEmails` validates the whole batch before writing any of it and commits in one transaction, so a bad message at the end cannot leave earlier ones stored under a 400.
- `renderEmail` is the shared tail of every mutation that returns the message it changed (link, unlink, confirm, reject, triage), so those cannot drift from one another or from `GetEmail`.

## Error Convention

- Genuinely domain-specific status choices (e.g. `Me` returning 401 for a gone user token) stay in the handler.
- Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- Sentry reports only fall-through unexpected 500s — routine errors never reported.
